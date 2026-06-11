package competitions

import (
	"context"
	"sync"
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

// InMemoryCompetitionRepository is a goroutine-safe, process-local store.
type InMemoryCompetitionRepository struct {
	mu           sync.RWMutex
	competitions map[string]Competition
	compOrder    []string
	// entries keyed by competitionID -> userID -> entry
	entries map[string]map[string]CompetitionEntry
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
