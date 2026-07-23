package performance

import (
	"context"
	"sync"
)

// Repository is the persistence boundary for ranked-performance state. Exactly
// one state row exists per portfolio.
//
// Create inserts a brand-new state (fails if one already exists). Update applies
// an optimistic-concurrency write: it must be given the version that was read,
// and returns ErrVersionConflict if the stored version has moved on. This is the
// lost-update guard for concurrent mutations of the same portfolio.
type Repository interface {
	GetByPortfolio(ctx context.Context, portfolioID string) (*State, error)
	Create(ctx context.Context, state State) error
	Update(ctx context.Context, state State, expectedVersion int64) error
}

// InMemoryRepository is a goroutine-safe, process-local store mirroring the
// Postgres semantics (including the optimistic-version check) so the default
// zero-infrastructure configuration behaves identically.
type InMemoryRepository struct {
	mu     sync.RWMutex
	byPort map[string]State
}

// NewInMemoryRepository returns an empty in-memory ranked-state store.
func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{byPort: make(map[string]State)}
}

func (r *InMemoryRepository) GetByPortfolio(_ context.Context, portfolioID string) (*State, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	st, ok := r.byPort[portfolioID]
	if !ok {
		return nil, ErrStateNotFound
	}
	copied := st
	if st.SegmentStartValueBase != nil {
		v := *st.SegmentStartValueBase
		copied.SegmentStartValueBase = &v
	}
	return &copied, nil
}

func (r *InMemoryRepository) Create(_ context.Context, state State) error {
	if err := validate(state); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byPort[state.PortfolioID]; ok {
		return ErrVersionConflict
	}
	r.byPort[state.PortfolioID] = clone(state)
	return nil
}

func (r *InMemoryRepository) Update(_ context.Context, state State, expectedVersion int64) error {
	if err := validate(state); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	existing, ok := r.byPort[state.PortfolioID]
	if !ok {
		return ErrStateNotFound
	}
	if existing.Version != expectedVersion {
		return ErrVersionConflict
	}
	r.byPort[state.PortfolioID] = clone(state)
	return nil
}

func clone(state State) State {
	copied := state
	if state.SegmentStartValueBase != nil {
		v := *state.SegmentStartValueBase
		copied.SegmentStartValueBase = &v
	}
	if state.SegmentStartedAt != nil {
		t := *state.SegmentStartedAt
		copied.SegmentStartedAt = &t
	}
	return copied
}

// validate enforces the storage invariants shared by both repositories.
func validate(state State) error {
	if state.PortfolioID == "" || state.UserID == "" {
		return ErrInvalidState
	}
	if !isFinite(state.CheckpointIndex) || state.CheckpointIndex <= 0 {
		return ErrInvalidState
	}
	switch state.Status {
	case StatusActive:
		if state.SegmentStartValueBase == nil || *state.SegmentStartValueBase <= 0 || !isFinite(*state.SegmentStartValueBase) {
			return ErrInvalidState
		}
	case StatusPaused:
		if state.SegmentStartValueBase != nil {
			return ErrInvalidState
		}
	default:
		return ErrInvalidState
	}
	return nil
}
