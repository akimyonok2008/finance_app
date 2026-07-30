package competitions

import (
	"context"
	"errors"
	"time"
)

// ArenaCompetitionSummary is the privacy-safe catalogue row for one
// competition (legacy weekly sprint or engine edition) — the shape Arena's
// discovery screens (live/upcoming/joined/completed) render as a card.
type ArenaCompetitionSummary struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	Description      string     `json:"description,omitempty"`
	Category         string     `json:"category,omitempty"`
	IconKey          string     `json:"icon_key,omitempty"`
	Status           string     `json:"status"`
	StartsAt         time.Time  `json:"starts_at"`
	EndsAt           time.Time  `json:"ends_at"`
	JoinOpensAt      *time.Time `json:"join_opens_at,omitempty"`
	JoinClosesAt     *time.Time `json:"join_closes_at,omitempty"`
	ScoringScope     string     `json:"scoring_scope,omitempty"`
	ParticipantCount int        `json:"participant_count"`
	Joined           bool       `json:"joined"`
	EntryStatus      string     `json:"entry_status,omitempty"`
}

// DefinitionMetadataProvider is an optional capability of
// CompetitionRepository: resolves an engine edition's display metadata
// (description/category/icon) by its definition, so the catalogue never
// duplicates that metadata onto every edition row. Postgres-only — the
// in-memory repository (dev/test mode) simply omits it, and a competition
// still displays fine with just its name.
type DefinitionMetadataProvider interface {
	DefinitionMetadata(ctx context.Context, definitionID string) (description, category, iconKey string, err error)
}

// maxArenaCatalogueSize caps the catalogue response. Competitions are a small,
// operator-curated set (unlike users or rankings), so a simple cap is
// sufficient discipline — no cursor pagination is needed at this scale.
const maxArenaCatalogueSize = 100

// ArenaCatalogue lists every competition (legacy sprint and engine editions)
// as privacy-safe summary cards, enriched with the caller's own joined/entry
// status. Definition display metadata is resolved once per distinct
// definition, not once per edition.
func (s *Service) ArenaCatalogue(ctx context.Context, userID string) ([]ArenaCompetitionSummary, error) {
	comps, err := s.ListCompetitions(ctx)
	if err != nil {
		return nil, err
	}
	if len(comps) > maxArenaCatalogueSize {
		comps = comps[:maxArenaCatalogueSize]
	}
	metaProvider, _ := s.repo.(DefinitionMetadataProvider)
	metaCache := map[string][3]string{}

	out := make([]ArenaCompetitionSummary, 0, len(comps))
	for _, c := range comps {
		summary, err := s.summarize(ctx, c, userID, metaProvider, metaCache)
		if err != nil {
			return nil, err
		}
		out = append(out, summary)
	}
	return out, nil
}

// ArenaCatalogueItem returns one competition's catalogue card, or
// ErrCompetitionNotFound.
func (s *Service) ArenaCatalogueItem(ctx context.Context, competitionID, userID string) (*ArenaCompetitionSummary, error) {
	comp, err := s.loadCompetition(ctx, competitionID)
	if err != nil {
		return nil, err
	}
	metaProvider, _ := s.repo.(DefinitionMetadataProvider)
	summary, err := s.summarize(ctx, *comp, userID, metaProvider, map[string][3]string{})
	if err != nil {
		return nil, err
	}
	return &summary, nil
}

func (s *Service) summarize(ctx context.Context, c Competition, userID string, metaProvider DefinitionMetadataProvider, metaCache map[string][3]string) (ArenaCompetitionSummary, error) {
	summary := ArenaCompetitionSummary{
		ID: c.ID, Name: c.Name, StartsAt: c.StartsAt, EndsAt: c.EndsAt,
		ScoringScope: c.ScoringScope,
	}
	if c.IsLegacy() {
		summary.Status = c.Status
	} else {
		summary.Status = c.LifecycleStatus
		summary.JoinOpensAt = c.JoinOpensAt
		summary.JoinClosesAt = c.JoinClosesAt
	}

	if c.DefinitionID != "" && metaProvider != nil {
		meta, ok := metaCache[c.DefinitionID]
		if !ok {
			desc, cat, icon, err := metaProvider.DefinitionMetadata(ctx, c.DefinitionID)
			if err != nil {
				return ArenaCompetitionSummary{}, err
			}
			meta = [3]string{desc, cat, icon}
			metaCache[c.DefinitionID] = meta
		}
		summary.Description, summary.Category, summary.IconKey = meta[0], meta[1], meta[2]
	}

	entries, err := s.repo.ListEntries(ctx, c.ID)
	if err != nil {
		return ArenaCompetitionSummary{}, err
	}
	summary.ParticipantCount = len(entries)

	entry, err := s.repo.GetEntry(ctx, c.ID, userID)
	switch {
	case err == nil:
		summary.Joined = true
		summary.EntryStatus = entry.EntryStatus
		if summary.EntryStatus == "" {
			summary.EntryStatus = EntryLegacyActive
		}
	case errors.Is(err, ErrEntryNotFound):
		// not joined
	default:
		return ArenaCompetitionSummary{}, err
	}
	return summary, nil
}
