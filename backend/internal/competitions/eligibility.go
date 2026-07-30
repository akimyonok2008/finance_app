package competitions

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ardakimyonok/finance_app/internal/competitions/rules"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
)

// SnapshotProvider is the narrow, read-only portfolio boundary the engine
// consumes (implemented by *portfolio.Service). The competition package never
// reconstructs portfolio state from repositories and never mutates the real
// portfolio.
type SnapshotProvider interface {
	CaptureCompetitionSnapshot(ctx context.Context, userID string, req portfolio.CompetitionSnapshotRequest) (portfolio.CompetitionPortfolioSnapshot, error)
}

// UniverseResolver resolves named instrument universes to instrument-ID sets.
// nil (or an unknown name) yields no membership, and universe-based rules
// then fail closed with an explicit reason — never an open pass.
type UniverseResolver interface {
	ResolveUniverses(ctx context.Context, names []string) (map[string]map[string]bool, error)
}

// SetSnapshotProvider attaches the portfolio-domain snapshot boundary,
// enabling eligibility previews (and, later phases, joins).
func (s *Service) SetSnapshotProvider(p SnapshotProvider) { s.snapshots = p }

// SetUniverseResolver attaches the named-universe resolver. Optional.
func (s *Service) SetUniverseResolver(r UniverseResolver) { s.universes = r }

// EligibilityRuleResult is the privacy-safe public projection of one rule
// outcome: percentages, counts and reason codes only.
type EligibilityRuleResult struct {
	Code     string `json:"code"`
	Label    string `json:"label"`
	Required string `json:"required"`
	Actual   string `json:"actual"`
	Passed   bool   `json:"passed"`
	Reason   string `json:"reason,omitempty"`
}

// EligibilityPreviewResponse answers "could I enter this competition right
// now?". It carries NO absolute portfolio values — evidence is percentage,
// count and day-count strings produced by the rules engine.
type EligibilityPreviewResponse struct {
	CompetitionID string                  `json:"competition_id"`
	Eligible      bool                    `json:"eligible"`
	EvaluatedAt   time.Time               `json:"evaluated_at"`
	Rules         []EligibilityRuleResult `json:"rules"`
}

// legacyRulesSnapshot is the rule document applied to legacy sprint rows that
// predate rules stamping (rows the pre-engine EnsureCurrentSprint created
// after migration 0045 ran). Identical to the migrated legacy definition v1.
var legacyRulesSnapshot = json.RawMessage(`{
	"schema_version": 1,
	"eligibility": {"schema_version":1,"all":[{"code":"non_empty_portfolio","label":"Portfolio has at least one position","metric":"position_count","operator":"gte","value":"1"}]},
	"scoring": {"schema_version":1,"scope":"full_portfolio","include_cash":false,"legacy_join_time_baseline":true}
}`)

// effectiveRules parses the edition's stamped rules snapshot (falling back to
// the canonical legacy document for unstamped legacy rows) into validated
// engine documents.
func effectiveRules(c *Competition) (rules.Eligibility, rules.Scoring, error) {
	raw := c.RulesSnapshotJSON
	if len(raw) == 0 {
		if !c.IsLegacy() {
			return rules.Eligibility{}, rules.Scoring{}, fmt.Errorf("edition %s has no rules snapshot", c.ID)
		}
		raw = legacyRulesSnapshot
	}
	var snapshot RulesSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return rules.Eligibility{}, rules.Scoring{}, fmt.Errorf("edition %s rules snapshot: %w", c.ID, err)
	}
	return rules.ValidateDefinitionVersionPayloads(snapshot.Eligibility, snapshot.Scoring)
}

// EligibilityPreview evaluates the competition's eligibility rules against
// the caller's CURRENT portfolio composition. Server-computed only: nothing
// here trusts client-supplied numbers, and the preview is advisory — the join
// flow re-evaluates from scratch.
func (s *Service) EligibilityPreview(ctx context.Context, competitionID, userID string) (*EligibilityPreviewResponse, error) {
	if s.snapshots == nil {
		return nil, ErrEligibilityUnavailable
	}
	comp, err := s.loadCompetition(ctx, competitionID)
	if err != nil {
		return nil, err
	}
	eligibility, _, err := effectiveRules(comp)
	if err != nil {
		return nil, err
	}

	snap, err := s.snapshots.CaptureCompetitionSnapshot(ctx, userID, portfolio.CompetitionSnapshotRequest{})
	if err != nil {
		return nil, err
	}
	facts, err := s.factsFromSnapshot(ctx, snap, eligibility)
	if err != nil {
		return nil, err
	}

	result := rules.Evaluate(eligibility, facts, s.clock.Now())
	resp := &EligibilityPreviewResponse{
		CompetitionID: competitionID,
		Eligible:      result.Eligible,
		EvaluatedAt:   result.EvaluatedAt,
		Rules:         make([]EligibilityRuleResult, 0, len(result.Rules)),
	}
	for _, ev := range result.Rules {
		resp.Rules = append(resp.Rules, EligibilityRuleResult{
			Code: ev.Code, Label: ev.Label, Required: ev.Required,
			Actual: ev.Actual, Passed: ev.Passed, Reason: ev.Reason,
		})
	}
	return resp, nil
}

// factsFromSnapshot converts the portfolio-domain snapshot into the pure
// facts model the rules engine evaluates, resolving any named universes the
// document references.
func (s *Service) factsFromSnapshot(ctx context.Context, snap portfolio.CompetitionPortfolioSnapshot, e rules.Eligibility) (rules.PortfolioFacts, error) {
	facts := rules.PortfolioFacts{
		CashValueBase:  snap.CashValueBase,
		TotalValueBase: snap.TotalValueBase,
	}
	if !snap.PortfolioCreatedAt.IsZero() {
		facts.PortfolioAgeDays = int(s.clock.Now().UTC().Sub(snap.PortfolioCreatedAt) / (24 * time.Hour))
	}
	for _, p := range snap.Positions {
		facts.Positions = append(facts.Positions, rules.PositionFacts{
			InstrumentID:   p.InstrumentID,
			AssetType:      p.AssetType,
			VenueMIC:       p.VenueMIC,
			IssuerCountry:  p.IssuerCountry,
			ListingCountry: p.ListingCountry,
			Currency:       p.Currency,
			ValueBase:      p.ValueBase,
		})
	}

	names := universeNames(e)
	if len(names) > 0 && s.universes != nil {
		membership, err := s.universes.ResolveUniverses(ctx, names)
		if err != nil {
			return rules.PortfolioFacts{}, err
		}
		facts.UniverseMembership = membership
	}
	return facts, nil
}

func universeNames(e rules.Eligibility) []string {
	seen := map[string]bool{}
	var names []string
	for _, group := range [][]rules.Rule{e.All, e.Any} {
		for _, r := range group {
			if r.Filter != nil && r.Filter.Universe != "" && !seen[r.Filter.Universe] {
				seen[r.Filter.Universe] = true
				names = append(names, r.Filter.Universe)
			}
		}
	}
	return names
}
