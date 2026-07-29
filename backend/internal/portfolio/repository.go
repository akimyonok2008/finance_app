package portfolio

import (
	"context"
	"errors"
	"sort"
	"strings"
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
	// EnsureDefaultPortfolio initializes the user's single default portfolio
	// if absent. It must be race-safe: two concurrent commands
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
	// ListActiveInstruments returns the distinct (instrument_id, symbol) pairs
	// held in an open position, for the resolved rows only (instrument_id IS
	// NOT NULL). The automatic corporate-action pipeline uses it to fetch
	// provider events by stable identity instead of ticker string, for the
	// share of holdings that have one.
	ListActiveInstruments(ctx context.Context) ([]ActiveInstrument, error)
	ListIncomeDiscoveryInstruments(ctx context.Context, since time.Time) ([]IncomeDiscoveryInstrument, error)
	ListIncomeHistoricalHolders(ctx context.Context, instrumentID, symbol string) ([]SymbolHolder, error)
	// ListOpenPositionsBySymbol returns every user's OPEN position in a symbol.
	// The automatic corporate-action pipeline uses it to discover which
	// portfolios an issuer/exchange event affects.
	ListOpenPositionsBySymbol(ctx context.Context, symbol string) ([]*Position, error)
	// ListOpenPositionsByInstrumentID is the identity-based counterpart of
	// ListOpenPositionsBySymbol, used when the corporate-action event
	// resolved to a stable instrument identity.
	ListOpenPositionsByInstrumentID(ctx context.Context, instrumentID string) ([]*Position, error)

	// --- Legacy identity backfill (see BackfillJob) ---
	ListPositionsMissingInstrumentID(ctx context.Context, limit int) ([]*Position, error)
	ListActivitiesMissingInstrumentID(ctx context.Context, limit int) ([]Activity, error)
	SetPositionInstrumentID(ctx context.Context, id, instrumentID string) error
	SetActivityInstrumentID(ctx context.Context, id, instrumentID string) error
	// EnqueueIdentityReconciliation records a legacy row the backfill job
	// could not resolve unambiguously. Re-running the job against the same
	// unresolved row must not pile up duplicate pending entries (idempotent
	// on (table_name, record_id) while status='pending').
	EnqueueIdentityReconciliation(ctx context.Context, item ReconciliationItem) error
	ListPendingReconciliation(ctx context.Context, limit int) ([]ReconciliationItem, error)
	// ResolveReconciliation assigns an instrument identity chosen by an
	// administrator and applies it to the underlying row.
	ResolveReconciliation(ctx context.Context, id, instrumentID, resolvedBy string) error
	RejectReconciliation(ctx context.Context, id, resolvedBy string) error
	ListCashBalances(ctx context.Context, userID string) ([]CashBalance, error)
	ListActivities(ctx context.Context, userID string, limit int) ([]Activity, error)
	// GetActivityByID returns a single activity scoped to userID, avoiding a
	// full-ledger scan to find one row.
	GetActivityByID(ctx context.Context, userID, activityID string) (Activity, bool, error)
	// ListActivitiesFiltered returns a page of activities matching the given
	// activity types (nil/empty = all) and symbol (empty = all), newest first,
	// paginated at the database level. total is the count of all matching rows
	// (ignoring limit/offset).
	ListActivitiesFiltered(ctx context.Context, userID string, types []ActivityType, symbol string, limit, offset int) (items []Activity, total int, err error)
	// FindCorrectionForActivity returns the activity (if any) whose metadata
	// marks it as a correction of activityID.
	FindCorrectionForActivity(ctx context.Context, userID, activityID string) (Activity, bool, error)
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

	// SnapshottedUserIDs returns the set of user IDs that already have an
	// archive snapshot for the given UTC calendar date. It is a cheap
	// membership query (no valuation), so the daily-snapshot job can skip the
	// expensive per-user Summary() computation for users already done for the
	// day instead of recomputing it — and immediately discarding it via
	// CreateArchiveSnapshot's own idempotency check — on every tick.
	SnapshottedUserIDs(ctx context.Context, date time.Time) (map[string]bool, error)
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

	reconciliation map[string]ReconciliationItem

	faults Faults
}

// NewInMemoryRepository returns an empty in-memory store.
func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		aggregates:     make(map[string]*aggregateState),
		userPortfolio:  make(map[string]string),
		locks:          make(map[string]*sync.Mutex),
		archives:       make([]*PortfolioArchiveSnapshot, 0),
		archiveDays:    make(map[string]bool),
		claimed:        make(map[string]bool),
		audits:         make(map[string]MutationAudit),
		reconciliation: make(map[string]ReconciliationItem),
	}
}

func newPortfolioID() string { return uuid.NewString() }

func (r *InMemoryRepository) OnAccountDeleted(_ context.Context, userID string) error {
	r.mu.Lock()
	portfolioID := r.userPortfolio[userID]
	delete(r.userPortfolio, userID)
	delete(r.aggregates, portfolioID)
	delete(r.locks, portfolioID)
	archives := r.archives[:0]
	for _, snapshot := range r.archives {
		if snapshot.UserID != userID {
			archives = append(archives, snapshot)
		}
	}
	r.archives = archives
	for key := range r.archiveDays {
		if strings.HasPrefix(key, portfolioID+"|") {
			delete(r.archiveDays, key)
		}
	}
	outbox := r.outbox[:0]
	for _, event := range r.outbox {
		if event.UserID != userID {
			outbox = append(outbox, event)
		} else {
			delete(r.claimed, event.ID)
		}
	}
	r.outbox = outbox
	r.mu.Unlock()

	r.auditMu.Lock()
	for key, audit := range r.audits {
		if audit.UserID == userID {
			delete(r.audits, key)
		}
	}
	auditLog := r.auditLog[:0]
	for _, audit := range r.auditLog {
		if audit.UserID != userID {
			auditLog = append(auditLog, audit)
		}
	}
	r.auditLog = auditLog
	r.auditMu.Unlock()
	return nil
}

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

func (r *InMemoryRepository) ListOpenPositionsByInstrumentID(ctx context.Context, instrumentID string) ([]*Position, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if instrumentID == "" {
		return []*Position{}, nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Position, 0)
	for _, agg := range r.aggregates {
		for _, p := range agg.positions {
			if positionStatus(p) == PositionStatusOpen && p.InstrumentID == instrumentID {
				clone := *p
				out = append(out, &clone)
			}
		}
	}
	return out, nil
}

func (r *InMemoryRepository) ListActiveInstruments(ctx context.Context) ([]ActiveInstrument, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	seen := map[string]bool{}
	out := make([]ActiveInstrument, 0)
	for _, agg := range r.aggregates {
		for _, p := range agg.positions {
			if positionStatus(p) == PositionStatusOpen && p.InstrumentID != "" && !seen[p.InstrumentID] {
				seen[p.InstrumentID] = true
				out = append(out, ActiveInstrument{InstrumentID: p.InstrumentID, Symbol: p.Symbol})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Symbol < out[j].Symbol })
	return out, nil
}

func (r *InMemoryRepository) ListPositionsMissingInstrumentID(ctx context.Context, limit int) ([]*Position, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Position, 0)
	for _, agg := range r.aggregates {
		for _, p := range agg.positions {
			if p.InstrumentID == "" {
				out = append(out, &Position{ID: p.ID, Symbol: p.Symbol, CreatedAt: p.CreatedAt})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *InMemoryRepository) ListActivitiesMissingInstrumentID(ctx context.Context, limit int) ([]Activity, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Activity, 0)
	for _, agg := range r.aggregates {
		for _, a := range agg.activities {
			if a.InstrumentID == "" && a.Symbol != "" {
				out = append(out, Activity{ID: a.ID, Symbol: a.Symbol, OccurredAt: a.OccurredAt})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].OccurredAt.Before(out[j].OccurredAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *InMemoryRepository) SetPositionInstrumentID(ctx context.Context, id, instrumentID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, agg := range r.aggregates {
		if p, ok := agg.positions[id]; ok {
			if p.InstrumentID == "" {
				p.InstrumentID = instrumentID
			}
			return nil
		}
	}
	return ErrPositionNotFound
}

func (r *InMemoryRepository) SetActivityInstrumentID(ctx context.Context, id, instrumentID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, agg := range r.aggregates {
		for i := range agg.activities {
			if agg.activities[i].ID == id && agg.activities[i].InstrumentID == "" {
				agg.activities[i].InstrumentID = instrumentID
				return nil
			}
		}
	}
	return nil
}

func (r *InMemoryRepository) EnqueueIdentityReconciliation(ctx context.Context, item ReconciliationItem) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.reconciliation {
		if existing.Status == ReconciliationStatusPending && existing.TableName == item.TableName && existing.RecordID == item.RecordID {
			return nil // already queued, matches the Postgres ON CONFLICT DO NOTHING
		}
	}
	if item.ID == "" {
		item.ID = uuid.NewString()
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now().UTC()
	}
	item.Status = ReconciliationStatusPending
	r.reconciliation[item.ID] = item
	return nil
}

func (r *InMemoryRepository) ListPendingReconciliation(ctx context.Context, limit int) ([]ReconciliationItem, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ReconciliationItem, 0)
	for _, item := range r.reconciliation {
		if item.Status == ReconciliationStatusPending {
			out = append(out, item)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *InMemoryRepository) ResolveReconciliation(ctx context.Context, id, instrumentID, resolvedBy string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	item, ok := r.reconciliation[id]
	if !ok || item.Status != ReconciliationStatusPending {
		r.mu.Unlock()
		return ErrReconciliationItemNotFound
	}
	tableName, recordID := item.TableName, item.RecordID
	now := time.Now().UTC()
	item.Status = ReconciliationStatusResolved
	item.CandidateInstrumentID = instrumentID
	item.ResolvedAt = &now
	item.ResolvedBy = resolvedBy
	r.reconciliation[id] = item
	r.mu.Unlock()

	switch tableName {
	case "positions":
		if err := r.SetPositionInstrumentID(ctx, recordID, instrumentID); err != nil && !errors.Is(err, ErrPositionNotFound) {
			return err
		}
	case "portfolio_activities":
		return r.SetActivityInstrumentID(ctx, recordID, instrumentID)
	}
	return nil
}

func (r *InMemoryRepository) RejectReconciliation(ctx context.Context, id, resolvedBy string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.reconciliation[id]
	if !ok || item.Status != ReconciliationStatusPending {
		return ErrReconciliationItemNotFound
	}
	now := time.Now().UTC()
	item.Status = ReconciliationStatusRejected
	item.ResolvedAt = &now
	item.ResolvedBy = resolvedBy
	r.reconciliation[id] = item
	return nil
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

func (r *InMemoryRepository) GetActivityByID(ctx context.Context, userID, activityID string) (Activity, bool, error) {
	if err := ctx.Err(); err != nil {
		return Activity{}, false, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.userPortfolio[userID]
	if !ok {
		return Activity{}, false, nil
	}
	for _, a := range r.aggregates[id].activities {
		if a.ID == activityID {
			return cloneActivity(a), true, nil
		}
	}
	return Activity{}, false, nil
}

func (r *InMemoryRepository) FindCorrectionForActivity(ctx context.Context, userID, activityID string) (Activity, bool, error) {
	if err := ctx.Err(); err != nil {
		return Activity{}, false, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.userPortfolio[userID]
	if !ok {
		return Activity{}, false, nil
	}
	for _, a := range r.aggregates[id].activities {
		if correctionOf, _ := a.Metadata["correction_of_activity_id"].(string); correctionOf == activityID {
			return cloneActivity(a), true, nil
		}
	}
	return Activity{}, false, nil
}

func (r *InMemoryRepository) ListActivitiesFiltered(ctx context.Context, userID string, types []ActivityType, symbol string, limit, offset int) ([]Activity, int, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.userPortfolio[userID]
	if !ok {
		return []Activity{}, 0, nil
	}
	typeSet := make(map[ActivityType]bool, len(types))
	for _, t := range types {
		typeSet[t] = true
	}
	items := r.aggregates[id].activities
	matched := make([]Activity, 0, len(items))
	for i := len(items) - 1; i >= 0; i-- {
		a := items[i]
		if symbol != "" && normalizeSymbol(a.Symbol) != symbol {
			continue
		}
		if len(typeSet) > 0 && !typeSet[a.Type] {
			continue
		}
		matched = append(matched, cloneActivity(a))
	}
	total := len(matched)
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	end := offset + limit
	if limit <= 0 || end > total {
		end = total
	}
	return matched[offset:end], total, nil
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

// SnapshottedUserIDs mirrors the Postgres implementation's cheap
// membership-only lookup: no valuation, just a scan of already-recorded
// snapshot dates.
func (r *InMemoryRepository) SnapshottedUserIDs(ctx context.Context, date time.Time) (map[string]bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	day := date.UTC().Format("2006-01-02")
	out := map[string]bool{}
	for _, s := range r.archives {
		if s.CapturedAt.UTC().Format("2006-01-02") == day {
			out[s.UserID] = true
		}
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
