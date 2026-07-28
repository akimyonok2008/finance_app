package portfolio

import (
	"context"
	"sync"
	"time"

	"github.com/ardakimyonok/finance_app/internal/performance"
)

// Faults lets tests inject failures at each stage of the mutation transaction to
// prove that no partial core state survives a rollback. A nil hook never fails.
type Faults struct {
	CreatePosition      error
	UpdatePosition      error
	DeletePosition      error
	ReplacePositions    error
	InsertRankedState   error
	UpdateRankedState   error
	AppendOutbox        error
	RecordAudit         error
	SetPortfolioVersion error
	Commit              error
	PutCash             error
	RecordActivity      error
}

// aggregateState is the committed in-memory state of one portfolio aggregate.
type aggregateState struct {
	portfolio  *Portfolio
	positions  map[string]*Position
	order      []string
	ranked     *performance.State
	cash       map[string]*CashBalance
	activities []Activity
}

// memoryTx stages every write and applies them to the committed state only on
// success, mirroring PostgreSQL transaction semantics: an error anywhere leaves
// the aggregate exactly as it was.
type memoryTx struct {
	store *InMemoryRepository
	base  *aggregateState

	portfolio  *Portfolio
	positions  map[string]*Position
	order      []string
	loaded     []*Position
	cash       map[string]*CashBalance
	activities []Activity

	ranked      *performance.State
	rankedDirty bool
	newVersion  *int64
	outbox      []OutboxEvent
	audits      []MutationAudit
}

func (t *memoryTx) Portfolio() *Portfolio { return t.portfolio }

func (t *memoryTx) Positions() []*Position { return t.loaded }
func (t *memoryTx) CashBalances() []CashBalance {
	out := make([]CashBalance, 0, len(t.cash))
	for _, balance := range t.cash {
		out = append(out, *balance)
	}
	return out
}

func (t *memoryTx) PutCashBalance(_ context.Context, balance CashBalance) error {
	if err := t.store.faults.PutCash; err != nil {
		return err
	}
	// Decimal amounts cannot be NaN/Inf; only the sign needs checking (exact,
	// no epsilon). Full memory-store decimal conversion is a later sub-scope.
	if balance.Amount.Sign() < 0 {
		return ErrInsufficientCash
	}
	stored := balance
	t.cash[balance.Currency] = &stored
	return nil
}

func (t *memoryTx) RecordActivity(_ context.Context, activity Activity) error {
	if err := t.store.faults.RecordActivity; err != nil {
		return err
	}
	for _, existing := range t.activities {
		if activity.RequestID != "" && existing.RequestID == activity.RequestID {
			return ErrDuplicateActivity
		}
	}
	t.activities = append(t.activities, cloneActivity(activity))
	return nil
}

func (t *memoryTx) LedgerActivities(_ context.Context) ([]Activity, error) {
	out := make([]Activity, 0, len(t.activities))
	for _, activity := range t.activities {
		out = append(out, cloneActivity(activity))
	}
	return out, nil
}

func (t *memoryTx) FindActivityByRequestID(_ context.Context, requestID string) (Activity, bool, error) {
	for _, activity := range t.activities {
		if activity.RequestID == requestID {
			return cloneActivity(activity), true, nil
		}
	}
	return Activity{}, false, nil
}

func (t *memoryTx) CreatePosition(_ context.Context, p *Position) error {
	if err := t.store.faults.CreatePosition; err != nil {
		return err
	}
	stored := *p
	if stored.Status == "" {
		stored.Status = PositionStatusOpen
	}
	t.positions[stored.ID] = &stored
	t.order = append(t.order, stored.ID)
	return nil
}

func (t *memoryTx) UpdatePosition(_ context.Context, p *Position) error {
	if err := t.store.faults.UpdatePosition; err != nil {
		return err
	}
	if _, ok := t.positions[p.ID]; !ok {
		return ErrPositionNotFound
	}
	stored := *p
	t.positions[stored.ID] = &stored
	return nil
}

func (t *memoryTx) DeletePosition(_ context.Context, id string) error {
	if err := t.store.faults.DeletePosition; err != nil {
		return err
	}
	if _, ok := t.positions[id]; !ok {
		return ErrPositionNotFound
	}
	delete(t.positions, id)
	for i, oid := range t.order {
		if oid == id {
			t.order = append(t.order[:i:i], t.order[i+1:]...)
			break
		}
	}
	return nil
}

func (t *memoryTx) ReplaceOpenPositions(_ context.Context, newPositions []*Position) error {
	if err := t.store.faults.ReplacePositions; err != nil {
		return err
	}
	kept := make([]string, 0, len(t.order))
	for _, id := range t.order {
		p, ok := t.positions[id]
		if ok && positionStatus(p) == PositionStatusOpen {
			delete(t.positions, id) // closed history is preserved; open is replaced
			continue
		}
		kept = append(kept, id)
	}
	t.order = kept
	for _, p := range newPositions {
		stored := *p
		if stored.Status == "" {
			stored.Status = PositionStatusOpen
		}
		t.positions[stored.ID] = &stored
		t.order = append(t.order, stored.ID)
	}
	return nil
}

func (t *memoryTx) RankedState(_ context.Context) (performance.State, bool, error) {
	if t.ranked == nil {
		return performance.State{}, false, nil
	}
	return performance.CloneState(*t.ranked), true, nil
}

func (t *memoryTx) PutRankedState(_ context.Context, state performance.State, isNew bool, expectedVersion int64) error {
	if isNew {
		if err := t.store.faults.InsertRankedState; err != nil {
			return err
		}
		if t.ranked != nil {
			return performance.ErrVersionConflict
		}
	} else {
		if err := t.store.faults.UpdateRankedState; err != nil {
			return err
		}
		if t.ranked == nil {
			return performance.ErrStateNotFound
		}
		if t.ranked.Version != expectedVersion {
			return performance.ErrVersionConflict
		}
	}
	if err := performance.ValidateState(state); err != nil {
		return err
	}
	stored := performance.CloneState(state)
	t.ranked = &stored
	t.rankedDirty = true
	return nil
}

func (t *memoryTx) SetPortfolioVersion(_ context.Context, version int64) error {
	if err := t.store.faults.SetPortfolioVersion; err != nil {
		return err
	}
	t.newVersion = &version
	return nil
}

func (t *memoryTx) AppendOutbox(_ context.Context, ev OutboxEvent) error {
	if err := t.store.faults.AppendOutbox; err != nil {
		return err
	}
	t.outbox = append(t.outbox, ev)
	return nil
}

func (t *memoryTx) RecordAudit(_ context.Context, a MutationAudit) error {
	if err := t.store.faults.RecordAudit; err != nil {
		return err
	}
	t.audits = append(t.audits, a)
	return nil
}

func (t *memoryTx) FindAuditByRequestID(_ context.Context, requestID string) (MutationAudit, bool, error) {
	t.store.auditMu.RLock()
	defer t.store.auditMu.RUnlock()
	a, ok := t.store.audits[t.portfolio.ID+"|"+requestID]
	if !ok {
		return MutationAudit{}, false, nil
	}
	return a, true, nil
}

// commit applies the staged writes to committed state. Callers hold the
// per-portfolio lock, so this is the atomic publication point.
func (t *memoryTx) commit() error {
	if err := t.store.faults.Commit; err != nil {
		return err
	}
	t.store.mu.Lock()
	defer t.store.mu.Unlock()

	t.base.positions = t.positions
	t.base.order = t.order
	t.base.cash = t.cash
	t.base.activities = t.activities
	if t.rankedDirty {
		t.base.ranked = t.ranked
	}
	if t.newVersion != nil {
		t.portfolio.Version = *t.newVersion
		stored := *t.portfolio
		t.base.portfolio = &stored
	}
	t.store.outbox = append(t.store.outbox, t.outbox...)
	for _, a := range t.audits {
		if a.RequestID != "" {
			t.store.auditMu.Lock()
			t.store.audits[a.PortfolioID+"|"+a.RequestID] = a
			t.store.auditMu.Unlock()
		}
		t.store.auditLog = append(t.store.auditLog, a)
	}
	return nil
}

// --- AggregateStore on the in-memory repository -------------------------------

// WithLockedPortfolio mirrors the Postgres boundary: it resolves (creating if
// needed, race-safely) the user's single default portfolio, takes that
// portfolio's lock so mutations to it serialize while other portfolios proceed
// concurrently, stages all writes, and publishes them atomically on success.
func (r *InMemoryRepository) WithLockedPortfolio(ctx context.Context, userID string, fn func(context.Context, AggregateTx) error) error {
	if err := ctx.Err(); err != nil {
		return err // honour cancellation before taking the lock
	}
	agg, lock, err := r.aggregateFor(userID)
	if err != nil {
		return err
	}
	lock.Lock()
	defer lock.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}

	r.mu.RLock()
	tx := &memoryTx{
		store:      r,
		base:       agg,
		portfolio:  clonePortfolio(agg.portfolio),
		positions:  make(map[string]*Position, len(agg.positions)),
		cash:       make(map[string]*CashBalance, len(agg.cash)),
		activities: append([]Activity(nil), agg.activities...),
		order:      append([]string(nil), agg.order...),
	}
	for id, p := range agg.positions {
		copied := *p
		tx.positions[id] = &copied
	}
	for currency, balance := range agg.cash {
		copied := *balance
		tx.cash[currency] = &copied
	}
	for _, id := range tx.order {
		if p, ok := tx.positions[id]; ok {
			copied := *p
			tx.loaded = append(tx.loaded, &copied)
		}
	}
	if agg.ranked != nil {
		st := performance.CloneState(*agg.ranked)
		tx.ranked = &st
	}
	r.mu.RUnlock()

	if err := fn(ctx, tx); err != nil {
		return err // staged writes discarded: rollback
	}
	return tx.commit()
}

// aggregateFor resolves the user's default portfolio, creating it if absent.
// Creation happens under the store lock with a user index, so two concurrent
// first requests can never produce two default portfolios (the in-memory mirror
// of the UNIQUE (user_id) constraint).
func (r *InMemoryRepository) aggregateFor(userID string) (*aggregateState, *sync.Mutex, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id, ok := r.userPortfolio[userID]
	if !ok {
		now := time.Now().UTC()
		pf := &Portfolio{
			ID: newPortfolioID(), UserID: userID, Name: DefaultPortfolioName,
			Currency: "USD", Version: 1, AutoFundPurchases: true,
			CreatedAt: now, UpdatedAt: now,
		}
		r.aggregates[pf.ID] = &aggregateState{
			portfolio: pf,
			positions: map[string]*Position{},
			cash:      map[string]*CashBalance{},
		}
		r.userPortfolio[userID] = pf.ID
		r.locks[pf.ID] = &sync.Mutex{}
		id = pf.ID
	}
	if _, ok := r.locks[id]; !ok {
		r.locks[id] = &sync.Mutex{}
	}
	return r.aggregates[id], r.locks[id], nil
}

func cloneActivity(activity Activity) Activity {
	copied := activity
	if activity.Metadata != nil {
		copied.Metadata = make(map[string]any, len(activity.Metadata))
		for key, value := range activity.Metadata {
			copied.Metadata[key] = value
		}
	}
	return copied
}

// --- outbox ------------------------------------------------------------------

// ClaimOutboxEvents claims unprocessed events for this worker. The claim is done
// under the store lock and marks events in-flight, mirroring Postgres
// FOR UPDATE SKIP LOCKED: no two workers can claim the same event.
func (r *InMemoryRepository) ClaimOutboxEvents(_ context.Context, limit int) ([]OutboxEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]OutboxEvent, 0, limit)
	for i := range r.outbox {
		if len(out) >= limit {
			break
		}
		ev := &r.outbox[i]
		if ev.ProcessedAt != nil || r.claimed[ev.ID] {
			continue
		}
		r.claimed[ev.ID] = true
		ev.AttemptCount++
		out = append(out, *ev)
	}
	return out, nil
}

func (r *InMemoryRepository) MarkOutboxProcessed(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	for i := range r.outbox {
		if r.outbox[i].ID == id {
			r.outbox[i].ProcessedAt = &now
			delete(r.claimed, id)
			return nil
		}
	}
	return nil
}

func (r *InMemoryRepository) MarkOutboxFailed(_ context.Context, id, cause string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.outbox {
		if r.outbox[i].ID == id {
			r.outbox[i].LastError = cause
			delete(r.claimed, id) // released for retry
			return nil
		}
	}
	return nil
}

func (r *InMemoryRepository) OutboxBacklog(ctx context.Context) (int64, time.Duration, error) {
	if err := ctx.Err(); err != nil {
		return 0, 0, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var pending int64
	var oldest time.Time
	for _, event := range r.outbox {
		if event.ProcessedAt != nil {
			continue
		}
		pending++
		if oldest.IsZero() || event.CreatedAt.Before(oldest) {
			oldest = event.CreatedAt
		}
	}
	if oldest.IsZero() {
		return pending, 0, nil
	}
	return pending, time.Since(oldest), nil
}

// --- ranked-state read side ---------------------------------------------------

// GetByPortfolio implements performance.StateReader against committed state.
func (r *InMemoryRepository) GetByPortfolio(_ context.Context, portfolioID string) (*performance.State, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	agg, ok := r.aggregates[portfolioID]
	if !ok || agg.ranked == nil {
		return nil, performance.ErrStateNotFound
	}
	st := performance.CloneState(*agg.ranked)
	return &st, nil
}

// --- test helpers -------------------------------------------------------------

// SetFaults installs fault-injection hooks (tests only).
func (r *InMemoryRepository) SetFaults(f Faults) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.faults = f
}

// OutboxEvents returns a copy of every emitted event (tests only).
func (r *InMemoryRepository) OutboxEvents() []OutboxEvent {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]OutboxEvent(nil), r.outbox...)
}

// AuditLog returns a copy of the mutation audit trail (tests only).
func (r *InMemoryRepository) AuditLog() []MutationAudit {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]MutationAudit(nil), r.auditLog...)
}

func clonePortfolio(p *Portfolio) *Portfolio {
	if p == nil {
		return nil
	}
	copied := *p
	return &copied
}

// sortedPositions returns copies of the aggregate's positions in insertion order.
func sortedPositions(agg *aggregateState) []*Position {
	out := make([]*Position, 0, len(agg.positions))
	for _, id := range agg.order {
		if p, ok := agg.positions[id]; ok {
			copied := *p
			out = append(out, &copied)
		}
	}
	return out
}
