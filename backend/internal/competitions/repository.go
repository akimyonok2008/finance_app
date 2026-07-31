package competitions

import (
	"context"
	"sync"
	"time"
)

// CompetitionRepository is the persistence boundary for competitions and their
// entries. The service depends only on this interface, so the in-memory store
// can later be replaced by PostgreSQL/Redis without touching business logic.
type CompetitionRepository interface {
	ListCompetitions(ctx context.Context) ([]Competition, error)
	GetCompetition(ctx context.Context, competitionID string) (*Competition, error)
	CreateCompetition(ctx context.Context, competition Competition) error
	CreateEntry(ctx context.Context, entry CompetitionEntry) error
	GetEntry(ctx context.Context, competitionID, userID string) (*CompetitionEntry, error)
	ListEntries(ctx context.Context, competitionID string) ([]CompetitionEntry, error)
}

// EditionRepository is the engine-side persistence boundary for competition
// editions: rows created with a stored lifecycle and an immutable rules
// snapshot, as opposed to the legacy CreateCompetition path. Implemented by
// PostgresCompetitionRepository (same table, additive columns).
type EditionRepository interface {
	// CreateEdition inserts a new edition (normally in LifecycleDraft). The
	// rules snapshot must already be stamped — an edition without one cannot
	// be interpreted. Fails with ErrEditionExists on a duplicate id.
	CreateEdition(ctx context.Context, edition Competition) error
	// TransitionLifecycle applies a validated lifecycle transition with an
	// optimistic guard: the UPDATE only lands if the edition is still in
	// `from`. ErrLifecycleConflict means someone else transitioned it first;
	// ErrInvalidLifecycleTransition means from→to is not in the state
	// machine. Timestamp columns (published_at/finalized_at/cancelled_at)
	// are stamped by the transition that reaches them.
	TransitionLifecycle(ctx context.Context, competitionID, from, to string, now time.Time) error
}

// ArenaCatalogueFilter is the SQL-pushdown filter for one page of the Arena
// catalogue: bucket and category narrow the WHERE clause, UserID resolves
// the caller's joined/entry_status per row, and Limit/Offset paginate.
type ArenaCatalogueFilter struct {
	UserID   string
	Bucket   string
	Category string
	Limit    int
	Offset   int
}

// ArenaCatalogueRepository is an optional repository capability: a
// repository that can filter, join, and paginate the Arena catalogue
// entirely in SQL instead of loading every competition row and
// enriching/filtering each one in Go. Implemented by
// PostgresCompetitionRepository; the in-memory repository omits it and
// Service.ArenaCatalogue falls back to the Go-side path.
type ArenaCatalogueRepository interface {
	ArenaCataloguePage(ctx context.Context, filter ArenaCatalogueFilter) (ArenaCataloguePage, error)
}

// InMemoryCompetitionRepository is a goroutine-safe, process-local store.
type InMemoryCompetitionRepository struct {
	mu           sync.RWMutex
	competitions map[string]Competition
	compOrder    []string
	// entries keyed by competitionID -> userID -> entry
	entries map[string]map[string]CompetitionEntry
	// rankingStates and displayNames back the RankingRepository implementation
	// (see ranking.go). displayNames lets tests seed the tie-break lookup that
	// Postgres does via a join to the users table.
	rankingStates map[string]*memoryRankingState
	displayNames  map[string]string
	resultStore   *memoryResults
	// finalizationStates backs the FinalizationRepository implementation
	// (see finalize.go), keyed by competitionID.
	finalizationStates map[string]*memoryFinalizationState
	// observations backs ObservationRepository (see observations.go), keyed
	// by "competitionID|boundary".
	observations map[string]*memoryObservationSet
}

// NewInMemoryCompetitionRepository returns an empty repository.
func NewInMemoryCompetitionRepository() *InMemoryCompetitionRepository {
	return &InMemoryCompetitionRepository{
		competitions: make(map[string]Competition),
		entries:      make(map[string]map[string]CompetitionEntry),
	}
}

func (r *InMemoryCompetitionRepository) ListCompetitions(_ context.Context) ([]Competition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Competition, 0, len(r.compOrder))
	for _, id := range r.compOrder {
		out = append(out, r.competitions[id])
	}
	return out, nil
}

func (r *InMemoryCompetitionRepository) GetCompetition(_ context.Context, competitionID string) (*Competition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.competitions[competitionID]
	if !ok {
		return nil, ErrCompetitionNotFound
	}
	return &c, nil
}

func (r *InMemoryCompetitionRepository) CreateCompetition(_ context.Context, competition Competition) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.competitions[competition.ID]; !exists {
		r.compOrder = append(r.compOrder, competition.ID)
	}
	r.competitions[competition.ID] = competition
	return nil
}

func (r *InMemoryCompetitionRepository) CreateEntry(_ context.Context, entry CompetitionEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.entries[entry.CompetitionID] == nil {
		r.entries[entry.CompetitionID] = make(map[string]CompetitionEntry)
	}
	r.entries[entry.CompetitionID][entry.UserID] = entry
	return nil
}

func (r *InMemoryCompetitionRepository) GetEntry(_ context.Context, competitionID, userID string) (*CompetitionEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	byUser, ok := r.entries[competitionID]
	if !ok {
		return nil, ErrEntryNotFound
	}
	e, ok := byUser[userID]
	if !ok {
		return nil, ErrEntryNotFound
	}
	return &e, nil
}

func (r *InMemoryCompetitionRepository) ListEntries(_ context.Context, competitionID string) ([]CompetitionEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	byUser := r.entries[competitionID]
	out := make([]CompetitionEntry, 0, len(byUser))
	for _, e := range byUser {
		out = append(out, e)
	}
	return out, nil
}

func (r *InMemoryCompetitionRepository) OnAccountDeleted(_ context.Context, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for competitionID, entries := range r.entries {
		delete(entries, userID)
		if len(entries) == 0 {
			delete(r.entries, competitionID)
		}
	}
	return nil
}
