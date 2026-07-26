package portfolio

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/db"
	"github.com/ardakimyonok/finance_app/internal/fx"
	"github.com/ardakimyonok/finance_app/internal/performance"
	"github.com/ardakimyonok/finance_app/internal/prices"
)

// These are REAL PostgreSQL integration tests: they exercise row locks, version
// increments, unique constraints, transaction rollback, ON CONFLICT behaviour,
// concurrent transactions, outbox SKIP LOCKED claiming and context cancellation.
// Mocked repositories cannot prove transactional correctness, so these are the
// authority for the storage-level guarantees.
//
// They are skipped unless DATABASE_URL_TEST points at a disposable database.

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL_TEST")
	if url == "" {
		t.Skip("DATABASE_URL_TEST not set; skipping Postgres integration test")
	}
	pool, err := db.ConnectPostgres(context.Background(), url)
	require.NoError(t, err)
	require.NoError(t, db.RunMigrations(context.Background(), pool))
	t.Cleanup(pool.Close)
	return pool
}

// seedUser inserts a user row directly (positions/portfolios have FK to users).
func seedUser(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	id := uuid.NewString()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO users (id, email, password_hash, display_name) VALUES ($1, $2, 'h', 'Test')`,
		id, id+"@example.com")
	require.NoError(t, err)
	return id
}

func newPGPosition(userID, portfolioID string) *Position {
	now := time.Now().UTC()
	return &Position{
		ID: uuid.NewString(), UserID: userID, PortfolioID: portfolioID,
		Symbol: "AAPL", AssetType: "stock", Quantity: 10, AverageBuyPrice: 180,
		Currency: "USD", Status: PositionStatusOpen, CreatedAt: now, UpdatedAt: now,
	}
}

func TestPG_EnsureDefaultPortfolioIsRaceSafe(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresRepository(pool)
	userID := seedUser(t, pool)
	ctx := context.Background()

	// Concurrent first requests must converge on ONE portfolio (UNIQUE (user_id)).
	const n = 8
	ids := make([]string, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			pf, err := repo.EnsureDefaultPortfolio(ctx, userID)
			if err == nil {
				ids[i] = pf.ID
			}
		}(i)
	}
	wg.Wait()

	for i := 1; i < n; i++ {
		assert.Equal(t, ids[0], ids[i], "all concurrent creators must observe the same portfolio")
	}
	var count int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM portfolios WHERE user_id=$1`, userID).Scan(&count))
	assert.Equal(t, 1, count, "exactly one default portfolio per user")
}

func TestPG_MutationCommitsPositionAndRankedStateTogether(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresRepository(pool)
	userID := seedUser(t, pool)
	ctx := context.Background()

	pf, err := repo.EnsureDefaultPortfolio(ctx, userID)
	require.NoError(t, err)
	pos := newPGPosition(userID, pf.ID)

	err = repo.WithLockedPortfolio(ctx, userID, func(ctx context.Context, tx AggregateTx) error {
		require.NoError(t, tx.CreatePosition(ctx, pos))
		st := performance.ActivateState(pf.ID, userID, 1800, true, time.Now().UTC())
		require.NoError(t, tx.PutRankedState(ctx, st, true, 0))
		return tx.SetPortfolioVersion(ctx, pf.Version+1)
	})
	require.NoError(t, err)

	got, err := repo.GetPosition(ctx, pos.ID)
	require.NoError(t, err)
	assert.Equal(t, "AAPL", got.Symbol)

	state, err := performance.NewPostgresStateReader(pool).GetByPortfolio(ctx, pf.ID)
	require.NoError(t, err)
	assert.Equal(t, 100.0, state.CheckpointIndex)

	after, err := repo.GetPortfolioByUser(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, pf.Version+1, after.Version, "aggregate version increments once per mutation")
}

func TestPG_RollbackLeavesNoPartialState(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresRepository(pool)
	userID := seedUser(t, pool)
	ctx := context.Background()

	pf, err := repo.EnsureDefaultPortfolio(ctx, userID)
	require.NoError(t, err)
	pos := newPGPosition(userID, pf.ID)

	// Write the position AND ranked state, then fail: both must roll back.
	sentinel := assert.AnError
	err = repo.WithLockedPortfolio(ctx, userID, func(ctx context.Context, tx AggregateTx) error {
		require.NoError(t, tx.CreatePosition(ctx, pos))
		st := performance.ActivateState(pf.ID, userID, 1800, true, time.Now().UTC())
		require.NoError(t, tx.PutRankedState(ctx, st, true, 0))
		return sentinel
	})
	require.ErrorIs(t, err, sentinel)

	_, err = repo.GetPosition(ctx, pos.ID)
	assert.ErrorIs(t, err, ErrPositionNotFound, "position must not survive rollback")

	_, err = performance.NewPostgresStateReader(pool).GetByPortfolio(ctx, pf.ID)
	assert.ErrorIs(t, err, performance.ErrStateNotFound, "ranked state must not survive rollback")

	after, err := repo.GetPortfolioByUser(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, pf.Version, after.Version, "version must not advance on rollback")
}

func TestPG_ConcurrentMutationsSerializeOnRowLock(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresRepository(pool)
	userID := seedUser(t, pool)
	ctx := context.Background()

	pf, err := repo.EnsureDefaultPortfolio(ctx, userID)
	require.NoError(t, err)

	const n = 6
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = repo.WithLockedPortfolio(ctx, userID, func(ctx context.Context, tx AggregateTx) error {
				// Each writer reads the version under the lock and bumps it by one.
				// Without real serialization this loses updates.
				cur := tx.Portfolio().Version
				position := newPGPosition(userID, pf.ID)
				position.Symbol = fmt.Sprintf("LOCK-%d", i)
				if err := tx.CreatePosition(ctx, position); err != nil {
					return err
				}
				return tx.SetPortfolioVersion(ctx, cur+1)
			})
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}

	after, err := repo.GetPortfolioByUser(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, pf.Version+int64(n), after.Version,
		"every mutation must be serialized; no lost version updates")

	positions, err := repo.ListPositionsByUser(ctx, userID)
	require.NoError(t, err)
	assert.Len(t, positions, n)
}

func TestPG_CashActivityIsAtomicIdempotentAndConstrained(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresRepository(pool)
	userID := seedUser(t, pool)
	svc := NewService(repo, prices.NewMockPriceProvider(), fx.NewMockFXProvider())

	deposit, err := svc.DepositCash(context.Background(), userID, "pg-deposit", CashFlowInput{
		Currency: "USD", Amount: 1000,
	})
	require.NoError(t, err)
	assert.InDelta(t, deposit.RankedIndexBefore, deposit.RankedIndexAfter, 1e-9)

	retry, err := svc.DepositCash(context.Background(), userID, "pg-deposit", CashFlowInput{
		Currency: "USD", Amount: 1000,
	})
	require.NoError(t, err)
	assert.True(t, retry.Duplicate)

	buy, err := svc.BuyPosition(context.Background(), userID, "pg-buy", BuyInput{
		Symbol: "AAPL", AssetType: AssetTypeStock, Quantity: 5,
	})
	require.NoError(t, err)
	assert.InDelta(t, buy.RankedIndexBefore, buy.RankedIndexAfter, 1e-9)

	balances, err := repo.ListCashBalances(context.Background(), userID)
	require.NoError(t, err)
	require.Len(t, balances, 1)
	assert.InDelta(t, 25, balances[0].Amount, 1e-9)
	activities, err := repo.ListActivities(context.Background(), userID, 100)
	require.NoError(t, err)
	assert.Len(t, activities, 2)

	var activityCount, outboxCount int
	pf, err := repo.GetPortfolioByUser(context.Background(), userID)
	require.NoError(t, err)
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM portfolio_activities WHERE portfolio_id=$1`, pf.ID).Scan(&activityCount))
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM portfolio_outbox WHERE aggregate_id=$1`, pf.ID).Scan(&outboxCount))
	assert.Equal(t, 2, activityCount)
	assert.Equal(t, 2, outboxCount)

	_, err = pool.Exec(context.Background(), `
		UPDATE portfolio_cash_balances SET amount=-1
		WHERE portfolio_id=$1 AND currency='USD'`, pf.ID)
	require.Error(t, err, "database must reject negative cash")
}

func TestPG_ConcurrentBuysCannotOverspendCash(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresRepository(pool)
	userID := seedUser(t, pool)
	svc := NewService(repo, prices.NewMockPriceProvider(), fx.NewMockFXProvider())
	_, err := svc.DepositCash(context.Background(), userID, "seed-cash", CashFlowInput{
		Currency: "USD", Amount: 1000,
	})
	require.NoError(t, err)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, symbol := range []string{"MSFT", "NVDA"} {
		wg.Add(1)
		go func(index int, symbol string) {
			defer wg.Done()
			_, errs[index] = svc.BuyPosition(context.Background(), userID,
				fmt.Sprintf("concurrent-buy-%d", index), BuyInput{
					Symbol: symbol, AssetType: AssetTypeStock, Quantity: 3,
				})
		}(i, symbol)
	}
	wg.Wait()
	// Automatic purchase funding is the default, so neither concurrent buy is
	// rejected. What the aggregate lock still guarantees is that the shared cash
	// balance is applied serially and can never go negative.
	for _, err := range errs {
		require.NoError(t, err)
	}
	balances, err := repo.ListCashBalances(context.Background(), userID)
	require.NoError(t, err)
	for _, balance := range balances {
		assert.GreaterOrEqual(t, balance.Amount, 0.0)
	}

	pf, err := repo.GetPortfolioByUser(context.Background(), userID)
	require.NoError(t, err)
	var funding int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM portfolio_activities
		 WHERE portfolio_id=$1 AND metadata_json ->> 'funding_reason' = 'purchase_shortfall'`,
		pf.ID).Scan(&funding))
	assert.GreaterOrEqual(t, funding, 1, "the overspending purchase was auto-funded, not rejected")
}

func TestPG_DailySnapshotUniquenessIsDatabaseEnforced(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresRepository(pool)
	userID := seedUser(t, pool)
	ctx := context.Background()
	pf, err := repo.EnsureDefaultPortfolio(ctx, userID)
	require.NoError(t, err)

	snap := func() *PortfolioArchiveSnapshot {
		return &PortfolioArchiveSnapshot{
			ID: uuid.NewString(), UserID: userID, PortfolioID: pf.ID,
			CapturedAt: time.Now().UTC(), BaseCurrency: "USD", PortfolioIndex: 100,
		}
	}

	inserted, err := repo.CreateArchiveSnapshot(ctx, snap())
	require.NoError(t, err)
	assert.True(t, inserted)

	// Same UTC day: ON CONFLICT DO NOTHING, reported as not inserted.
	inserted, err = repo.CreateArchiveSnapshot(ctx, snap())
	require.NoError(t, err)
	assert.False(t, inserted, "second snapshot on the same UTC day must be rejected")

	var count int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM portfolio_archive_snapshots WHERE portfolio_id=$1`, pf.ID).Scan(&count))
	assert.Equal(t, 1, count)
}

func TestPG_OutboxClaimingSkipsLockedRows(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresRepository(pool)
	userID := seedUser(t, pool)
	ctx := context.Background()
	pf, err := repo.EnsureDefaultPortfolio(ctx, userID)
	require.NoError(t, err)

	require.NoError(t, repo.WithLockedPortfolio(ctx, userID, func(ctx context.Context, tx AggregateTx) error {
		return tx.AppendOutbox(ctx, OutboxEvent{
			ID: uuid.NewString(), EventType: EventPortfolioMutated,
			AggregateType: "portfolio", AggregateID: pf.ID, AggregateVersion: 1,
			UserID: userID, RankedIndex: 100, RankingStatus: "active",
			TrackingStartedAt: time.Now().UTC().Add(-time.Hour),
			ValuationAsOf:     time.Now().UTC(),
			DataQualityStatus: "complete",
			CreatedAt:         time.Now().UTC(),
		})
	}))

	// Two concurrent claimers must never receive the same event.
	var wg sync.WaitGroup
	claims := make([][]OutboxEvent, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			claims[i], _ = repo.ClaimOutboxEvents(ctx, 10)
		}(i)
	}
	wg.Wait()

	seen := map[string]int{}
	for _, batch := range claims {
		for _, ev := range batch {
			seen[ev.ID]++
		}
	}
	for id, n := range seen {
		assert.Equal(t, 1, n, "event %s claimed more than once", id)
	}
}

func TestPG_CancelledContextRollsBack(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresRepository(pool)
	userID := seedUser(t, pool)
	pf, err := repo.EnsureDefaultPortfolio(context.Background(), userID)
	require.NoError(t, err)
	pos := newPGPosition(userID, pf.ID)

	ctx, cancel := context.WithCancel(context.Background())
	err = repo.WithLockedPortfolio(ctx, userID, func(ctx context.Context, tx AggregateTx) error {
		if err := tx.CreatePosition(ctx, pos); err != nil {
			return err
		}
		cancel() // client disconnects mid-transaction
		return ctx.Err()
	})
	require.Error(t, err)

	_, err = repo.GetPosition(context.Background(), pos.ID)
	assert.ErrorIs(t, err, ErrPositionNotFound, "cancelled transaction must roll back")
}
