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
	// IsLegacy marks pre-engine weekly-sprint rows (join-time baseline, live
	// repricing) so the client can separate them from engine editions instead
	// of rendering them as if they were the same product. Kept in the
	// catalogue only for migration compatibility with users already entered
	// in one — see Competition.IsLegacy.
	IsLegacy bool `json:"is_legacy"`
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

// maxArenaCatalogueSize caps the catalogue response for repositories that
// can't paginate the query itself (the in-memory store — see
// ArenaCatalogue's fallback path). Postgres pushes the limit into SQL
// instead via ArenaCatalogueRepository.
const maxArenaCatalogueSize = 100

// Arena catalogue bucket values — the four sections the catalogue page
// renders as separate tabs, each independently paginated.
const (
	ArenaBucketLive      = "live"
	ArenaBucketUpcoming  = "upcoming"
	ArenaBucketMine      = "mine"
	ArenaBucketCompleted = "completed"
)

// ArenaCatalogueQuery narrows and paginates one bucket/tab of the catalogue.
type ArenaCatalogueQuery struct {
	Bucket   string // one of the ArenaBucket* constants; "" = no bucket filter
	Category string // "" = any category
	Limit    int
	Offset   int
}

// ArenaCataloguePage is one page of catalogue cards plus whether more exist
// past this page (Offset+len(Cards)).
type ArenaCataloguePage struct {
	Cards   []ArenaCompetitionSummary
	HasMore bool
}

const defaultArenaCataloguePageSize = 20

// ArenaCataloguePageResponse is the wire shape of GET /arena/competitions:
// one page of cards plus whether more exist for this bucket/category.
type ArenaCataloguePageResponse struct {
	Items   []ArenaCompetitionSummary `json:"items"`
	HasMore bool                      `json:"has_more"`
}

func normalizeArenaCatalogueQuery(q ArenaCatalogueQuery) ArenaCatalogueQuery {
	if q.Limit <= 0 || q.Limit > maxArenaCatalogueSize {
		q.Limit = defaultArenaCataloguePageSize
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	return q
}

// arenaBucketMatch mirrors the client's per-tab bucket rule so the in-memory
// fallback path (below) filters identically to the SQL path.
func arenaBucketMatch(bucket string, c ArenaCompetitionSummary) bool {
	switch bucket {
	case ArenaBucketLive:
		return c.Status == StatusActive
	case ArenaBucketCompleted:
		return c.Status == StatusCompleted
	case ArenaBucketMine:
		return c.Joined && c.Status != StatusCompleted
	case ArenaBucketUpcoming:
		return c.Status != StatusActive && c.Status != StatusCompleted && c.Status != LifecycleCancelled && !c.Joined
	default:
		return true
	}
}

// ArenaCatalogue lists one bucket/tab of the catalogue, paginated. If the
// repository implements ArenaCatalogueRepository (Postgres), the bucket,
// category filter, and pagination are pushed into SQL. Otherwise (the
// in-memory repository used in dev/tests) it falls back to loading every
// competition and filtering/slicing in Go — fine at that scale.
func (s *Service) ArenaCatalogue(ctx context.Context, userID string, query ArenaCatalogueQuery) (ArenaCataloguePage, error) {
	query = normalizeArenaCatalogueQuery(query)

	if err := s.ensureCurrentSprint(ctx); err != nil {
		return ArenaCataloguePage{}, err
	}

	if pager, ok := s.repo.(ArenaCatalogueRepository); ok {
		return pager.ArenaCataloguePage(ctx, ArenaCatalogueFilter{
			UserID:   userID,
			Bucket:   query.Bucket,
			Category: query.Category,
			Limit:    query.Limit,
			Offset:   query.Offset,
		})
	}

	comps, err := s.ListCompetitions(ctx)
	if err != nil {
		return ArenaCataloguePage{}, err
	}
	metaProvider, _ := s.repo.(DefinitionMetadataProvider)
	metaCache := map[string][3]string{}

	all := make([]ArenaCompetitionSummary, 0, len(comps))
	for _, c := range comps {
		summary, err := s.summarize(ctx, c, userID, metaProvider, metaCache)
		if err != nil {
			return ArenaCataloguePage{}, err
		}
		// Legacy weekly sprints stay reachable for migration compatibility —
		// existing participants (or anyone who already has an entry) keep
		// seeing and using theirs — but they're dropped from discovery for
		// everyone else, so a new user never sees the old join-time-baseline
		// product presented as if it were an engine competition.
		if summary.IsLegacy && !summary.Joined {
			continue
		}
		if query.Category != "" && summary.Category != query.Category {
			continue
		}
		if !arenaBucketMatch(query.Bucket, summary) {
			continue
		}
		all = append(all, summary)
	}

	start := query.Offset
	if start > len(all) {
		start = len(all)
	}
	end := start + query.Limit
	if end > len(all) {
		end = len(all)
	}
	page := all[start:end]
	out := make([]ArenaCompetitionSummary, len(page))
	copy(out, page)
	return ArenaCataloguePage{Cards: out, HasMore: end < len(all)}, nil
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
	summary.IsLegacy = c.IsLegacy()
	if summary.IsLegacy {
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
