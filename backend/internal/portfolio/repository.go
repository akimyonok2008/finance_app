package portfolio

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Repository is the READ/query boundary for portfolios and positions. Every
// method takes the caller's context so cancellation, deadlines and tracing
// propagate into the database — no method fabricates a context.Background().
//
// Note the deliberate asymmetry: position MUTATIONS are not here. They live
// behind AggregateStore.WithLockedPortfolio (see store.go), so positions and
// ranked-performance state can only change together inside one transaction.
// Removing the standalone mutation methods removes the bypass.
type Repository interface {
	// EnsureDefaultPortfolio returns the user's single default portfolio,
	// creating it if absent. It must be race-safe: two concurrent first requests
	// yield the SAME portfolio (enforced by UNIQUE (user_id) in Postgres and the
	// user index in memory).
	EnsureDefaultPortfolio(ctx context.Context, userID string) (*Portfolio, error)
	GetPortfolioByUser(ctx context.Context, userID string) (*Portfolio, error)
	// SetAutoFundPurchases toggles the portfolio-level automatic-purchase-funding
	// preference. It is a preference flag, not financial state, so it does not
	// need the aggregate mutation boundary.
	SetAutoFundPurchases(ctx context.Context, userID string, enabled bool) error

	GetPosition(ctx context.Context, id string) (*Position, error)
	ListPositionsByUser(ctx context.Context, userID string) ([]*Position, error)
	ListActiveSymbols(ctx context.Context) ([]string, error)
	ListIncomeDiscoveryInstruments(ctx context.Context, since time.Time) ([]IncomeDiscoveryInstrument, error)
	ListIncomeHistoricalHolders(ctx context.Context, instrumentID, symbol string) ([]SymbolHolder, error)
	// ListOpenPositionsBySymbol returns every user's OPEN position in a symbol.
	// The automatic corporate-action pipeline uses it to discover which
	// portfolios an issuer/exchange event affects.
	ListOpenPositionsBySymbol(ctx context.Context, symbol string) ([]*Position, error)
	ListCashBalances(ctx context.Context, userID string) ([]CashBalance, error)
	ListActivities(ctx context.Context, userID string, limit int) ([]Activity, error)
	// ListActivitiesByPositionEpisode returns every activity sharing the given
	// position episode id (all partial sales, the final sale/write-off, and the
	// opening buy), scoped to userID for ownership. It is the canonical source
	// for building a complete closed-position summary — a closed episode must
	// never be reconstructed from only the final sale.
	ListActivitiesByPositionEpisode(ctx context.Context, userID, episodeID string) ([]Activity, error)

	// CreateArchiveSnapshot is idempotent per (portfolio, UTC date): it returns
	// inserted=false when a snapshot for that day already exists, so concurrent
	// workers can never create duplicates.
	CreateArchiveSnapshot(ctx context.Context, s *PortfolioArchiveSnapshot) (inserted bool, err error)
	ListArchiveSnapshots(ctx context.Context, userID string, from, to string) ([]*PortfolioArchiveSnapshot, error)
}

// InMemoryRepository is the process-local store used for zero-infrastructure
// development and tests. It mirrors the PostgreSQL semantics that matter:
// per-portfolio aggregate locking, atomic position+ranked-state commit, version
// checks, daily-snapshot uniqueness, idempotency keys and outbox behaviour.
type InMemoryRepository struct {
	mu sync.RWMutex

	aggregates    map[string]*aggregateState // keyed by portfolio id
	userPortfolio map[string]string          // userID -> portfolio id
	locks         map[string]*sync.Mutex     // per-portfolio aggregate lock
	archives      []*PortfolioArchiveSnapshot
	archiveDays   map[string]bool // "portfolioID|YYYY-MM-DD" uniqueness guard

	outbox  []OutboxEvent
	claimed map[string]bool

	auditMu  sync.RWMutex
	audits   map[string]MutationAudit // by request id (idempotency)
	auditLog []MutationAudit

	faults Faults
}

// NewInMemoryRepository returns an empty in-memory store.
func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		aggregates:    make(map[string]*aggregateState),
		userPortfolio: make(map[string]string),
		locks:         make(map[string]*sync.Mutex),
		archives:      make([]*PortfolioArchiveSnapshot, 0),
		archiveDays:   make(map[string]bool),
		claimed:       make(map[string]bool),
		audits:        make(map[string]MutationAudit),
	}
}

func newPortfolioID() string { return uuid.NewString() }

// EnsureDefaultPortfolio creates the user's default portfolio on first access.
// The user index is consulted and written under one lock, so concurrent first
// requests converge on a single portfolio.
func (r *InMemoryRepository) EnsureDefaultPortfolio(ctx context.Context, userID string) (*Portfolio, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	agg, _, err := r.aggregateFor(userID)
	if err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return clonePortfolio(agg.portfolio), nil
}

// GetPortfolioByUser returns the user's portfolio, or ErrPortfolioNotFound.
func (r *InMemoryRepository) GetPortfolioByUser(ctx context.Context, userID string) (*Portfolio, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.userPortfolio[userID]
	if !ok {
		return nil, ErrPortfolioNotFound
	}
	return clonePortfolio(r.aggregates[id].portfolio), nil
}

// SetAutoFundPurchases toggles the automatic-purchase-funding preference.
func (r *InMemoryRepository) SetAutoFundPurchases(ctx context.Context, userID string, enabled bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	id, ok := r.userPortfolio[userID]
	if !ok {
		return ErrPortfolioNotFound
	}
	r.aggregates[id].portfolio.AutoFundPurchases = enabled
	return nil
}

// GetPosition returns a copy of the position by id, or ErrPositionNotFound.
func (r *InMemoryRepository) GetPosition(ctx context.Context, id string) (*Position, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, agg := range r.aggregates {
		if p, ok := agg.positions[id]; ok {
			copied := *p
			return &copied, nil
		}
	}
	return nil, ErrPositionNotFound
}

// ListPositionsByUser returns copies of the user's positions in insertion order.
func (r *InMemoryRepository) ListPositionsByUser(ctx context.Context, userID string) ([]*Position, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.userPortfolio[userID]
	if !ok {
		return []*Position{}, nil
	}
	return sortedPositions(r.aggregates[id]), nil
}

func (r *InMemoryRepository) ListActiveSymbols(ctx context.Context) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	seen := map[string]bool{}
	out := make([]string, 0)
	for _, agg := range r.aggregates {
		for _, p := range agg.positions {
			if positionStatus(p) == PositionStatusOpen && !seen[p.Symbol] {
				seen[p.Symbol] = true
				out = append(out, p.Symbol)
			}
		}
	}
	return out, nil
}

func (r *InMemoryRepository) ListIncomeDiscoveryInstruments(ctx context.Context, since time.Time) ([]IncomeDiscoveryInstrument, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	byKey := map[string]IncomeDiscoveryInstrument{}
	for _, agg := range r.aggregates {
		for _, p := range agg.positions {
			if positionStatus(p) != PositionStatusOpen && (p.ClosedAt == nil || p.ClosedAt.Before(since)) {
				continue
			}
			key := p.InstrumentID + "|alias:" + normalizeSymbol(p.Symbol)
			if p.InstrumentID == "" {
				key = "symbol:" + normalizeSymbol(p.Symbol)
			}
			byKey[key] = IncomeDiscoveryInstrument{
				InstrumentID: p.InstrumentID, Symbol: p.Symbol, AssetType: p.AssetType,
			}
		}
		for _, a := range agg.activities {
			if a.OccurredAt.Before(since) || a.Symbol == "" {
				continue
			}
			key := a.InstrumentID + "|alias:" + normalizeSymbol(a.Symbol)
			if a.InstrumentID == "" {
				key = "symbol:" + normalizeSymbol(a.Symbol)
			}
			byKey[key] = IncomeDiscoveryInstrument{
				InstrumentID: a.InstrumentID, Symbol: a.Symbol, AssetType: a.AssetType,
			}
		}
	}
	out := make([]IncomeDiscoveryInstrument, 0, len(byKey))
	for _, item := range byKey {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Symbol < out[j].Symbol })
	return out, nil
}

func (r *InMemoryRepository) ListIncomeHistoricalHolders(ctx context.Context, instrumentID, symbol string) ([]SymbolHolder, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	byPortfolio := map[string]SymbolHolder{}
	for _, agg := range r.aggregates {
		for _, p := range agg.positions {
			matches := instrumentID != "" && p.InstrumentID == instrumentID
			if !matches && instrumentID == "" {
				matches = normalizeSymbol(p.Symbol) == normalizeSymbol(symbol)
			}
			if !matches {
				continue
			}
			h, ok := byPortfolio[p.PortfolioID]
			if !ok || p.CreatedAt.Before(h.AcquiredAt) {
				byPortfolio[p.PortfolioID] = SymbolHolder{
					UserID: p.UserID, PortfolioID: p.PortfolioID, AssetType: p.AssetType, AcquiredAt: p.CreatedAt,
				}
			}
		}
	}
	out := make([]SymbolHolder, 0, len(byPortfolio))
	for _, h := range byPortfolio {
		out = append(out, h)
	}
	return out, nil
}

func (r *InMemoryRepository) ListOpenPositionsBySymbol(ctx context.Context, symbol string) ([]*Position, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Position, 0)
	for _, agg := range r.aggregates {
		for _, p := range agg.positions {
			if positionStatus(p) == PositionStatusOpen && p.Symbol == symbol {
				clone := *p
				out = append(out, &clone)
			}
		}
	}
	return out, nil
}

func (r *InMemoryRepository) ListCashBalances(ctx context.Context, userID string) ([]CashBalance, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.userPortfolio[userID]
	if !ok {
		return []CashBalance{}, nil
	}
	agg := r.aggregates[id]
	out := make([]CashBalance, 0, len(agg.cash))
	for _, balance := range agg.cash {
		out = append(out, *balance)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Currency < out[j].Currency })
	return out, nil
}

func (r *InMemoryRepository) ListActivities(ctx context.Context, userID string, limit int) ([]Activity, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.userPortfolio[userID]
	if !ok {
		return []Activity{}, nil
	}
	items := r.aggregates[id].activities
	if limit <= 0 || limit > len(items) {
		limit = len(items)
	}
	out := make([]Activity, 0, limit)
	for i := len(items) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, cloneActivity(items[i]))
	}
	return out, nil
}

// ListActivitiesByPositionEpisode returns every activity for the given episode
// (in chronological order), scoped to the owning user. It uses the in-memory
// per-portfolio activity slice directly rather than scanning every user's
// ledger.
func (r *InMemoryRepository) ListActivitiesByPositionEpisode(ctx context.Context, userID, episodeID string) ([]Activity, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.userPortfolio[userID]
	if !ok {
		return []Activity{}, nil
	}
	items := r.aggregates[id].activities
	out := make([]Activity, 0)
	for _, a := range items {
		if a.PositionEpisodeID == episodeID {
			out = append(out, cloneActivity(a))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].OccurredAt.Equal(out[j].OccurredAt) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].OccurredAt.Before(out[j].OccurredAt)
	})
	return out, nil
}

// CreateArchiveSnapshot enforces one snapshot per portfolio per UTC day,
// mirroring the Postgres unique index. inserted=false means today's snapshot
// already existed.
func (r *InMemoryRepository) CreateArchiveSnapshot(ctx context.Context, s *PortfolioArchiveSnapshot) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := s.PortfolioID + "|" + s.CapturedAt.UTC().Format("2006-01-02")
	if r.archiveDays[key] {
		return false, nil
	}
	r.archiveDays[key] = true
	r.archives = append(r.archives, copyArchiveSnapshot(s))
	return true, nil
}

func (r *InMemoryRepository) ListArchiveSnapshots(ctx context.Context, userID string, from, to string) ([]*PortfolioArchiveSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
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

func copyArchiveSnapshot(s *PortfolioArchiveSnapshot) *PortfolioArchiveSnapshot {
	if s == nil {
		return nil
	}
	copied := *s
	copied.Positions = append([]PositionSummary(nil), s.Positions...)
	copied.ClosedPositions = append([]ClosedPositionSummary(nil), s.ClosedPositions...)
	copied.CashBalances = append([]CashBalanceView(nil), s.CashBalances...)
	return &copied
}

func timeFromRFC3339(v string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}, err
	}
	return t, nil
}
