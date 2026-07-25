package portfolio

import (
	"context"
	"time"

	"github.com/ardakimyonok/finance_app/internal/performance"
)

// --- transactional aggregate boundary ----------------------------------------
//
// Positions and ranked-performance state form ONE consistent aggregate. Every
// production mutation goes through AggregateStore.WithLockedPortfolio, which:
//
//  1. opens a database transaction,
//  2. locks the user's portfolio row (SELECT ... FOR UPDATE in Postgres, a
//     per-portfolio mutex in memory), serializing mutations to that portfolio
//     while leaving other portfolios free to proceed,
//  3. loads the positions and ranked state THROUGH that transaction, so the
//     coordinator never calculates from a stale read,
//  4. commits the position write, the ranked-state write, the portfolio version
//     bump, the audit record and the outbox event together — or rolls all of
//     them back.
//
// There is deliberately no public API that writes positions or ranked state
// outside this boundary.

// OutboxEventType enumerates the post-commit events emitted by a mutation.
type OutboxEventType string

const (
	// EventPortfolioMutated is emitted for every committed portfolio mutation and
	// carries the aggregate version, so a single event drives all derived work
	// (private archive snapshot, ranked lifecycle history, leaderboard cache,
	// and profile projections).
	EventPortfolioMutated OutboxEventType = "portfolio.mutated"
)

// RankingStatus mirrors performance.Status on the outbox payload so consumers
// (the outbox processor, cache sync) need not import the performance package.
type RankingStatus string

const (
	RankingStatusActive RankingStatus = "active"
	RankingStatusPaused RankingStatus = "paused"
)

// OutboxEvent is a durable, post-commit work item written inside the mutation
// transaction. Payload is privacy-safe: it carries the ranked index and status
// but never quantities, cost basis, portfolio value, or segment baselines.
type OutboxEvent struct {
	ID                string
	EventType         OutboxEventType
	AggregateType     string
	AggregateID       string // portfolio id
	AggregateVersion  int64
	UserID            string
	RankedIndex       float64
	RankingStatus     string
	TrackingStartedAt time.Time
	ValuationAsOf     time.Time
	DataQualityStatus string
	CreatedAt         time.Time
	ProcessedAt       *time.Time
	AttemptCount      int
	LastError         string
}

// MutationAudit is the privacy-safe mutation log. It proves the neutrality
// invariant (ranked_index_before == ranked_index_after for every mutation) and
// doubles as the idempotency record: RequestID is unique per committed mutation.
type MutationAudit struct {
	ID                       string
	RequestID                string
	PortfolioID              string
	UserID                   string
	MutationType             string
	PortfolioVersionBefore   int64
	PortfolioVersionAfter    int64
	PerformanceVersionBefore int64
	PerformanceVersionAfter  int64
	RankedIndexBefore        float64
	RankedIndexAfter         float64
	OccurredAt               time.Time
	// ResultPositionID lets a duplicate request replay the original result
	// without re-applying the mutation.
	ResultPositionID string
}

// AggregateTx is the transaction-scoped view of one locked portfolio aggregate.
// Every method takes the request context so cancellation and deadlines
// propagate into the database and roll the transaction back.
type AggregateTx interface {
	// Portfolio returns the locked aggregate row.
	Portfolio() *Portfolio
	// Positions returns every position (open and closed) loaded under the lock.
	Positions() []*Position
	CashBalances() []CashBalance

	CreatePosition(ctx context.Context, p *Position) error
	UpdatePosition(ctx context.Context, p *Position) error
	DeletePosition(ctx context.Context, id string) error
	// ReplaceOpenPositions deletes the open positions and inserts replacements,
	// preserving closed-position history.
	ReplaceOpenPositions(ctx context.Context, newPositions []*Position) error
	PutCashBalance(ctx context.Context, balance CashBalance) error
	RecordActivity(ctx context.Context, activity Activity) error
	FindActivityByRequestID(ctx context.Context, requestID string) (Activity, bool, error)

	// RankedState reads the aggregate's ranked-performance state. found=false
	// means the portfolio has no state yet and must be initialized in this
	// transaction.
	RankedState(ctx context.Context) (state performance.State, found bool, err error)
	// PutRankedState writes the ranked state. isNew inserts; otherwise it updates
	// guarded by expectedVersion (which, under the aggregate lock, can never lose
	// an update — the guard is defence in depth).
	PutRankedState(ctx context.Context, state performance.State, isNew bool, expectedVersion int64) error

	// SetPortfolioVersion persists the incremented aggregate version.
	SetPortfolioVersion(ctx context.Context, version int64) error

	AppendOutbox(ctx context.Context, ev OutboxEvent) error
	RecordAudit(ctx context.Context, a MutationAudit) error
	// FindAuditByRequestID supports idempotency: a retry with the same request id
	// returns the already-committed result instead of mutating again.
	FindAuditByRequestID(ctx context.Context, requestID string) (MutationAudit, bool, error)
}

// AggregateStore opens the mutation transaction boundary.
type AggregateStore interface {
	// WithLockedPortfolio ensures the user's default portfolio exists (race-safe),
	// locks it, loads its positions and runs fn inside a transaction. fn returning
	// nil commits; any error rolls everything back.
	WithLockedPortfolio(ctx context.Context, userID string, fn func(context.Context, AggregateTx) error) error
	// ClaimOutboxEvents atomically claims up to limit unprocessed events for this
	// worker (FOR UPDATE SKIP LOCKED in Postgres) so multiple instances never
	// process the same event concurrently.
	ClaimOutboxEvents(ctx context.Context, limit int) ([]OutboxEvent, error)
	// MarkOutboxProcessed records successful processing (idempotent).
	MarkOutboxProcessed(ctx context.Context, id string) error
	// MarkOutboxFailed records a failed attempt for retry.
	MarkOutboxFailed(ctx context.Context, id string, cause string) error
}
