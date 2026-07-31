package competitions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/ardakimyonok/finance_app/internal/competitions/rules"
	"github.com/ardakimyonok/finance_app/internal/money"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
)

// JoinEditionResponse is the privacy-safe confirmation for an engine join. It
// carries the entry lifecycle state and the eligibility evidence the entry
// was admitted under — never absolute portfolio values.
type JoinEditionResponse struct {
	CompetitionID string                  `json:"competition_id"`
	Joined        bool                    `json:"joined"`
	EntryStatus   string                  `json:"entry_status"`
	StartingIndex money.IndexValue        `json:"starting_index"`
	Eligibility   []EligibilityRuleResult `json:"eligibility"`
}

// Join is the single join entry point. Legacy weekly-sprint rows keep the
// pre-engine behavior (join while active, baseline at join, key optional for
// compatibility). Engine editions get the fair-baseline flow: joins only
// inside the registration window, an Idempotency-Key requirement, server-side
// eligibility re-evaluation, a frozen scoring composition, and a common
// baseline established later at starts_at by the baseline worker.
func (s *Service) Join(ctx context.Context, competitionID, userID, idempotencyKey string) (*JoinEditionResponse, error) {
	comp, err := s.loadCompetition(ctx, competitionID)
	if err != nil {
		return nil, err
	}
	if comp.IsLegacy() {
		legacy, err := s.JoinCompetition(ctx, competitionID, userID)
		if err != nil {
			return nil, err
		}
		return &JoinEditionResponse{
			CompetitionID: legacy.CompetitionID, Joined: legacy.Joined,
			EntryStatus: EntryLegacyActive, StartingIndex: legacy.StartingIndex,
		}, nil
	}
	if idempotencyKey == "" {
		return nil, ErrIdempotencyKeyRequired
	}
	return s.joinEdition(ctx, comp, userID, idempotencyKey)
}

// joinFingerprint binds an Idempotency-Key to the exact join it was used for,
// so a key collision against a different (competition, user) pair is
// detected instead of silently replaying the wrong entry.
func joinFingerprint(competitionID, userID string) string {
	sum := sha256.Sum256([]byte(competitionID + "|" + userID))
	return hex.EncodeToString(sum[:])
}

func (s *Service) joinEdition(ctx context.Context, comp *Competition, userID, idempotencyKey string) (*JoinEditionResponse, error) {
	if s.snapshots == nil {
		return nil, ErrEligibilityUnavailable
	}
	now := s.clock.Now().UTC()
	if !joinWindowOpen(comp, now) {
		return nil, ErrJoinWindowClosed
	}

	engineRepo, ok := s.repo.(EngineEntryRepository)
	if !ok {
		return nil, ErrEligibilityUnavailable
	}

	fingerprint := joinFingerprint(comp.ID, userID)

	// Durable idempotency: this key was already bound to a join. Replay the
	// stored entry's response rather than re-running eligibility/snapshot
	// work. A key bound to a DIFFERENT (competition, user) fingerprint is a
	// conflict — reusing a key across two different joins is a client bug,
	// not a retry, and must not silently return the wrong competition's entry.
	if byKey, found, err := engineRepo.FindEntryByIdempotencyKey(ctx, userID, idempotencyKey); err != nil {
		return nil, err
	} else if found {
		if byKey.RequestFingerprint != fingerprint {
			return nil, ErrIdempotencyConflict
		}
		return joinResponseFromEntry(comp.ID, byKey), nil
	}

	// Idempotent replay: an existing entry (created under any key, including
	// legacy rows with none stored) is returned as-is. The unique
	// (competition_id, user_id) constraint is the hard backstop below, so a
	// concurrent duplicate join degrades to this same read.
	if existing, err := s.repo.GetEntry(ctx, comp.ID, userID); err == nil {
		return joinResponseFromEntry(comp.ID, existing), nil
	}

	eligibility, scoring, err := effectiveRules(comp)
	if err != nil {
		return nil, err
	}

	// Joins freeze the composition the whole competition will score, so the
	// capture demands fresh quotes and a stable portfolio version.
	snap, err := s.snapshots.CaptureCompetitionSnapshot(ctx, userID, portfolio.CompetitionSnapshotRequest{RequireFreshQuotes: true})
	if err != nil {
		return nil, err
	}
	facts, err := s.factsFromSnapshot(ctx, snap, eligibility)
	if err != nil {
		return nil, err
	}
	result := rules.Evaluate(eligibility, facts, now)
	evidenceJSON, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("competitions: marshal evidence: %w", err)
	}
	if !result.Eligible {
		return &JoinEditionResponse{
			CompetitionID: comp.ID, Joined: false,
			StartingIndex: money.MustIndexValue("100"),
			Eligibility:   toRuleResults(result),
		}, ErrNotEligible
	}

	entry, err := buildEngineEntry(comp, userID, snap, scoring, facts.UniverseMembership, evidenceJSON, now)
	if err != nil {
		return nil, err
	}
	entry.IdempotencyKey = idempotencyKey
	entry.RequestFingerprint = fingerprint
	if err := engineRepo.CreateEngineEntry(ctx, *entry); err != nil {
		if errors.Is(err, ErrEntryExists) {
			if existing, getErr := s.repo.GetEntry(ctx, comp.ID, userID); getErr == nil {
				return joinResponseFromEntry(comp.ID, existing), nil
			}
		}
		return nil, err
	}
	resp := joinResponseFromEntry(comp.ID, entry)
	resp.Eligibility = toRuleResults(result)
	return resp, nil
}

func joinWindowOpen(comp *Competition, now time.Time) bool {
	if comp.LifecycleStatus != LifecycleRegistrationOpen {
		return false
	}
	if comp.JoinOpensAt == nil || comp.JoinClosesAt == nil {
		return false
	}
	return !now.Before(*comp.JoinOpensAt) && now.Before(*comp.JoinClosesAt)
}

func joinResponseFromEntry(competitionID string, e *CompetitionEntry) *JoinEditionResponse {
	return &JoinEditionResponse{
		CompetitionID: competitionID, Joined: true,
		EntryStatus:   e.EntryStatus,
		StartingIndex: money.MustIndexValue("100"),
		Eligibility:   evidenceToRuleResults(e.EligibilityEvidenceJSON),
	}
}

func toRuleResults(result rules.Result) []EligibilityRuleResult {
	out := make([]EligibilityRuleResult, 0, len(result.Rules))
	for _, ev := range result.Rules {
		out = append(out, EligibilityRuleResult{
			Code: ev.Code, Label: ev.Label, Required: ev.Required,
			Actual: ev.Actual, Passed: ev.Passed, Reason: ev.Reason,
		})
	}
	return out
}

func evidenceToRuleResults(raw json.RawMessage) []EligibilityRuleResult {
	if len(raw) == 0 {
		return nil
	}
	var result rules.Result
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil
	}
	return toRuleResults(result)
}

// buildEngineEntry freezes the scoring composition: every captured position
// and cash balance is stored with its classification evidence, and
// included_in_score marks the subset the scoring configuration selects. The
// entry awaits the common baseline (EntryAdmitted/BaselinePending); the
// join-time included value is stored only as a provisional starting value
// until the baseline worker writes the official one.
func buildEngineEntry(
	comp *Competition, userID string,
	snap portfolio.CompetitionPortfolioSnapshot,
	scoring rules.Scoring,
	universes map[string]map[string]bool,
	evidenceJSON json.RawMessage,
	now time.Time,
) (*CompetitionEntry, error) {
	entryID := uuid.NewString()
	includedValue := money.ZeroAmount()

	positions := make([]CompetitionEntrySnapshotPosition, 0, len(snap.Positions))
	for _, p := range snap.Positions {
		included := scoringIncludesPosition(scoring, p, universes)
		classification, err := json.Marshal(map[string]string{
			"instrument_id": p.InstrumentID, "asset_type": p.AssetType,
			"venue_mic": p.VenueMIC, "listing_country": p.ListingCountry,
			"issuer_country": p.IssuerCountry, "currency": p.Currency,
		})
		if err != nil {
			return nil, fmt.Errorf("competitions: marshal classification: %w", err)
		}
		if included {
			includedValue = includedValue.Add(p.ValueBase)
		}
		positions = append(positions, CompetitionEntrySnapshotPosition{
			ID: uuid.NewString(), CompetitionEntryID: entryID,
			Symbol: p.Symbol, AssetType: p.AssetType, Quantity: p.Quantity,
			Currency: p.Currency, StartingPrice: p.Price, StartingPriceCurrency: p.PriceCurrency,
			StartingValueBase: p.ValueBase,
			InstrumentID:      p.InstrumentID, VenueMIC: p.VenueMIC,
			ClassificationSnapshotJSON: classification,
			IncludedInScore:            included,
		})
	}
	cash := make([]CompetitionEntrySnapshotCash, 0, len(snap.Cash))
	for _, c := range snap.Cash {
		if scoring.IncludeCash {
			includedValue = includedValue.Add(c.ValueBase)
		}
		cash = append(cash, CompetitionEntrySnapshotCash{
			ID: uuid.NewString(), CompetitionEntryID: entryID,
			Currency: c.Currency, Amount: c.Amount, ValueBase: c.ValueBase,
			IncludedInScore: scoring.IncludeCash,
		})
	}
	if includedValue.Sign() <= 0 {
		return nil, ErrNothingToScore
	}

	captured := snap.CapturedAt
	return &CompetitionEntry{
		ID: entryID, CompetitionID: comp.ID, UserID: userID,
		StartingValue: includedValue, // provisional; official value set at baseline
		StartingIndex: money.MustIndexValue("100"),
		JoinedAt:      now,
		Snapshots:     positions,
		CashSnapshots: cash,

		EntryStatus:               EntryAdmitted,
		PortfolioVersion:          snap.PortfolioVersion,
		SnapshotCapturedAt:        &captured,
		EligibilityEvidenceJSON:   evidenceJSON,
		ScoringScope:              scoring.Scope,
		EligibleStartingValueBase: includedValue,
		BaselineStatus:            BaselinePending,
	}, nil
}

func scoringIncludesPosition(sc rules.Scoring, p portfolio.CompetitionSnapshotPosition, universes map[string]map[string]bool) bool {
	if sc.Scope == ScopeFullPortfolioAlias {
		return true
	}
	facts := rules.PositionFacts{
		InstrumentID: p.InstrumentID, AssetType: p.AssetType, VenueMIC: p.VenueMIC,
		IssuerCountry: p.IssuerCountry, ListingCountry: p.ListingCountry, Currency: p.Currency,
	}
	return rules.Matches(sc.Filter, facts, universes)
}

// ScopeFullPortfolioAlias re-exports the rules constant so join code reads
// naturally without importing rules at every call site.
const ScopeFullPortfolioAlias = rules.ScopeFullPortfolio

// Withdraw removes the caller's entry before registration closes. Withdrawal
// is only possible while the entry is still admitted (pre-baseline) and the
// join window's close has not passed. A withdrawn entry is terminal: the
// unique (competition, user) constraint means no re-entry for this edition.
func (s *Service) Withdraw(ctx context.Context, competitionID, userID string) error {
	comp, err := s.loadCompetition(ctx, competitionID)
	if err != nil {
		return err
	}
	if comp.IsLegacy() {
		return ErrWithdrawalClosed // legacy sprints never supported withdrawal
	}
	now := s.clock.Now().UTC()
	if comp.JoinClosesAt == nil || !now.Before(*comp.JoinClosesAt) {
		return ErrWithdrawalClosed
	}
	entry, err := s.repo.GetEntry(ctx, competitionID, userID)
	if err != nil {
		return err
	}
	if entry.EntryStatus != EntryAdmitted {
		return ErrWithdrawalClosed
	}
	engineRepo, ok := s.repo.(EngineEntryRepository)
	if !ok {
		return ErrEligibilityUnavailable
	}
	return engineRepo.UpdateEntryStatus(ctx, entry.ID, EntryAdmitted, EntryWithdrawn, now)
}
