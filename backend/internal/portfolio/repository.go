package portfolio

import "sync"

// Repository is the persistence boundary for portfolios and positions. Business
// logic depends only on this interface, so the in-memory implementation can be
// replaced by a Postgres-backed one without changing the service or handlers.
//
// Note: the repository performs no ownership checks — that is the service's
// responsibility. GetPosition returns any position by id; the service verifies
// ownership before mutating or exposing it.
type Repository interface {
	CreatePortfolio(p *Portfolio) error
	GetPortfolioByUser(userID string) (*Portfolio, error)

	CreatePosition(p *Position) error
	GetPosition(id string) (*Position, error)
	ListPositionsByUser(userID string) ([]*Position, error)
	UpdatePosition(p *Position) error
	DeletePosition(id string) error
}

// InMemoryRepository is a goroutine-safe, process-local store for the prototype.
type InMemoryRepository struct {
	mu            sync.RWMutex
	portfolios    map[string]*Portfolio // keyed by portfolio id
	userPortfolio map[string]string     // userID -> portfolio id
	positions     map[string]*Position  // keyed by position id
	positionOrder []string              // preserves insertion order for stable listing
}

// NewInMemoryRepository returns an empty in-memory repository.
func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		portfolios:    make(map[string]*Portfolio),
		userPortfolio: make(map[string]string),
		positions:     make(map[string]*Position),
	}
}

// CreatePortfolio stores a copy of the portfolio and indexes it by user.
func (r *InMemoryRepository) CreatePortfolio(p *Portfolio) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	stored := *p
	r.portfolios[stored.ID] = &stored
	r.userPortfolio[stored.UserID] = stored.ID
	return nil
}

// GetPortfolioByUser returns a copy of the user's portfolio, or ErrPortfolioNotFound.
func (r *InMemoryRepository) GetPortfolioByUser(userID string) (*Portfolio, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.userPortfolio[userID]
	if !ok {
		return nil, ErrPortfolioNotFound
	}
	copied := *r.portfolios[id]
	return &copied, nil
}

// CreatePosition stores a copy of the position, preserving insertion order.
func (r *InMemoryRepository) CreatePosition(p *Position) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	stored := *p
	r.positions[stored.ID] = &stored
	r.positionOrder = append(r.positionOrder, stored.ID)
	return nil
}

// GetPosition returns a copy of the position by id, or ErrPositionNotFound.
func (r *InMemoryRepository) GetPosition(id string) (*Position, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.positions[id]
	if !ok {
		return nil, ErrPositionNotFound
	}
	copied := *p
	return &copied, nil
}

// ListPositionsByUser returns copies of the user's positions in insertion order.
func (r *InMemoryRepository) ListPositionsByUser(userID string) ([]*Position, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Position, 0)
	for _, id := range r.positionOrder {
		if p, ok := r.positions[id]; ok && p.UserID == userID {
			copied := *p
			out = append(out, &copied)
		}
	}
	return out, nil
}

// UpdatePosition replaces the stored position, or returns ErrPositionNotFound.
func (r *InMemoryRepository) UpdatePosition(p *Position) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.positions[p.ID]; !ok {
		return ErrPositionNotFound
	}
	stored := *p
	r.positions[stored.ID] = &stored
	return nil
}

// DeletePosition removes the position by id, or returns ErrPositionNotFound.
func (r *InMemoryRepository) DeletePosition(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.positions[id]; !ok {
		return ErrPositionNotFound
	}
	delete(r.positions, id)
	for i, oid := range r.positionOrder {
		if oid == id {
			r.positionOrder = append(r.positionOrder[:i], r.positionOrder[i+1:]...)
			break
		}
	}
	return nil
}
