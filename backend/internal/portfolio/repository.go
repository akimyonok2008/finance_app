package portfolio

import (
	"sync"
	"time"
)

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
	ListActiveSymbols() ([]string, error)
	UpdatePosition(p *Position) error
	DeletePosition(id string) error
	// ReplaceOpenPositions atomically deletes the user's open positions and
	// inserts the given replacements in a single unit of work, so a whole-portfolio
	// strategy copy can never leave the portfolio half-replaced.
	ReplaceOpenPositions(userID string, newPositions []*Position) error
	CreateArchiveSnapshot(s *PortfolioArchiveSnapshot) error
	ListArchiveSnapshots(userID string, from, to string) ([]*PortfolioArchiveSnapshot, error)
}

// InMemoryRepository is a goroutine-safe, process-local store for the prototype.
type InMemoryRepository struct {
	mu            sync.RWMutex
	portfolios    map[string]*Portfolio // keyed by portfolio id
	userPortfolio map[string]string     // userID -> portfolio id
	positions     map[string]*Position  // keyed by position id
	positionOrder []string              // preserves insertion order for stable listing
	archives      []*PortfolioArchiveSnapshot
}

// NewInMemoryRepository returns an empty in-memory repository.
func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		portfolios:    make(map[string]*Portfolio),
		userPortfolio: make(map[string]string),
		positions:     make(map[string]*Position),
		archives:      make([]*PortfolioArchiveSnapshot, 0),
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
	if stored.Status == "" {
		stored.Status = PositionStatusOpen
	}
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

func (r *InMemoryRepository) ListActiveSymbols() ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	seen := map[string]bool{}
	out := make([]string, 0)
	for _, p := range r.positions {
		if positionStatus(p) == PositionStatusOpen && !seen[p.Symbol] {
			seen[p.Symbol] = true
			out = append(out, p.Symbol)
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

func (r *InMemoryRepository) CreateArchiveSnapshot(s *PortfolioArchiveSnapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	stored := copyArchiveSnapshot(s)
	r.archives = append(r.archives, stored)
	return nil
}

func (r *InMemoryRepository) ListArchiveSnapshots(userID string, from, to string) ([]*PortfolioArchiveSnapshot, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	fromTime, err := timeFromRFC3339(from)
	if err != nil {
		return nil, err
	}
	toTime, err := timeFromRFC3339(to)
	if err != nil {
		return nil, err
	}
	out := make([]*PortfolioArchiveSnapshot, 0)
	for _, s := range r.archives {
		if s.UserID != userID {
			continue
		}
		if s.CapturedAt.Before(fromTime) || s.CapturedAt.After(toTime) {
			continue
		}
		out = append(out, copyArchiveSnapshot(s))
	}
	return out, nil
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

// ReplaceOpenPositions atomically swaps the user's open positions under a single
// lock so no reader ever observes a half-replaced portfolio.
func (r *InMemoryRepository) ReplaceOpenPositions(userID string, newPositions []*Position) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Delete existing open positions for the user.
	kept := make([]string, 0, len(r.positionOrder))
	for _, id := range r.positionOrder {
		p, ok := r.positions[id]
		if ok && p.UserID == userID && positionStatus(p) == PositionStatusOpen {
			delete(r.positions, id)
			continue
		}
		kept = append(kept, id)
	}
	r.positionOrder = kept
	// Insert replacements, preserving insertion order.
	for _, p := range newPositions {
		stored := *p
		if stored.Status == "" {
			stored.Status = PositionStatusOpen
		}
		r.positions[stored.ID] = &stored
		r.positionOrder = append(r.positionOrder, stored.ID)
	}
	return nil
}

func copyArchiveSnapshot(s *PortfolioArchiveSnapshot) *PortfolioArchiveSnapshot {
	if s == nil {
		return nil
	}
	copied := *s
	copied.Positions = append([]PositionSummary(nil), s.Positions...)
	copied.ClosedPositions = append([]ClosedPositionSummary(nil), s.ClosedPositions...)
	return &copied
}

func timeFromRFC3339(v string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}, err
	}
	return t, nil
}
