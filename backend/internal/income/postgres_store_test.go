package income_test

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/db"
	"github.com/ardakimyonok/finance_app/internal/income"
	"github.com/ardakimyonok/finance_app/internal/money"
)

// These integration tests run only when DATABASE_URL_TEST points at a disposable
// database. They verify the durable store's uniqueness, correction versioning,
// transactional claiming, and decimal precision — the guarantees that stop two
// workers crediting the same income event twice.
func incTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL_TEST")
	if url == "" {
		t.Skip("DATABASE_URL_TEST not set; skipping Postgres integration test")
	}
	ctx := context.Background()
	pool, err := db.ConnectPostgres(ctx, url)
	require.NoError(t, err)
	require.NoError(t, db.RunMigrations(ctx, pool))
	t.Cleanup(pool.Close)
	return pool
}

func seedIncUserPortfolio(t *testing.T, pool *pgxpool.Pool) (userID, portfolioID string) {
	t.Helper()
	ctx := context.Background()
	userID = uuid.NewString()
	portfolioID = uuid.NewString()
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, email, password_hash, display_name, created_at) VALUES ($1,$2,'x','Test',now())`,
		userID, userID+"@e.com")
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		`INSERT INTO portfolios (id, user_id, name, currency, created_at, updated_at)
		 VALUES ($1,$2,'Default','USD',now(),now())`, portfolioID, userID)
	require.NoError(t, err)
	return userID, portfolioID
}

func incEvent(id string) income.IncomeEvent {
	return income.IncomeEvent{
		ID: id, Provider: "manual_dev", ProviderEventID: id, Type: income.TypeCashDividend,
		Instrument: income.InstrumentReference{Symbol: "AAPL"}, AmountPerUnit: money.MustPrice("0.25"), Currency: "USD",
		PaymentDate: time.Now().UTC(), Status: income.StatusScheduled, Quality: income.QualityVerified,
		RawFingerprint: "fp-1", RetrievedAt: time.Now().UTC(),
	}
}

func TestPG_IncomeEventUniquenessAndCorrection(t *testing.T) {
	pool := incTestPool(t)
	store := income.NewPostgresStore(pool)
	ctx := context.Background()
	id := "manual_dev:DIV-" + uuid.NewString()

	changed, err := store.UpsertEvent(ctx, incEvent(id))
	require.NoError(t, err)
	assert.False(t, changed)

	changed, err = store.UpsertEvent(ctx, incEvent(id))
	require.NoError(t, err)
	assert.False(t, changed, "same fingerprint is idempotent")

	ev := incEvent(id)
	ev.RawFingerprint = "fp-2"
	ev.AmountPerUnit = money.MustPrice("0.30") // provider revised the amount
	changed, err = store.UpsertEvent(ctx, ev)
	require.NoError(t, err)
	assert.True(t, changed, "changed fingerprint is a correction")

	got, ok, err := store.GetEvent(ctx, id)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, 0, money.MustPrice("0.30").Cmp(got.AmountPerUnit)) // decimal precision preserved

	var count int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM income_events WHERE provider='manual_dev' AND provider_event_id=$1`, id).Scan(&count))
	assert.Equal(t, 1, count)
}

func TestPG_IncomeApplicationClaimIsExclusive(t *testing.T) {
	pool := incTestPool(t)
	store := income.NewPostgresStore(pool)
	ctx := context.Background()
	userID, portfolioID := seedIncUserPortfolio(t, pool)
	id := "manual_dev:DIV-" + uuid.NewString()
	_, err := store.UpsertEvent(ctx, incEvent(id))
	require.NoError(t, err)

	const workers = 8
	var wg sync.WaitGroup
	results := make([]bool, workers)
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(i int) {
			defer wg.Done()
			claimed, err := store.ClaimApplication(ctx, id, portfolioID, userID)
			require.NoError(t, err)
			results[i] = claimed
		}(i)
	}
	wg.Wait()

	won := 0
	for _, r := range results {
		if r {
			won++
		}
	}
	assert.Equal(t, 1, won, "exactly one worker claims the (event, portfolio)")
}

func TestPG_IncomeApplicationStoresSeparatedAmounts(t *testing.T) {
	pool := incTestPool(t)
	store := income.NewPostgresStore(pool)
	ctx := context.Background()
	userID, portfolioID := seedIncUserPortfolio(t, pool)
	id := "manual_dev:DIV-" + uuid.NewString()
	_, err := store.UpsertEvent(ctx, incEvent(id))
	require.NoError(t, err)

	claimed, err := store.ClaimApplication(ctx, id, portfolioID, userID)
	require.NoError(t, err)
	require.True(t, claimed)

	require.NoError(t, store.CompleteApplication(ctx, income.Application{
		IncomeEventID: id, PortfolioID: portfolioID, UserID: userID, Status: income.ApplicationApplied,
		EligibleQuantity: money.MustQuantity("100"), GrossAmount: money.MustAmount("100"),
		WithholdingAmount: money.MustAmount("15"), NetAmount: money.MustAmount("85"),
		CashCurrency: "USD", Estimated: true,
	}))

	got, ok, err := store.GetApplication(ctx, id, portfolioID)
	require.NoError(t, err)
	require.True(t, ok)
	// Gross / withholding / net stored distinctly (never one ambiguous amount).
	assert.True(t, money.MustAmount("100").EqualAmount(got.GrossAmount))
	assert.True(t, money.MustAmount("15").EqualAmount(got.WithholdingAmount))
	assert.True(t, money.MustAmount("85").EqualAmount(got.NetAmount))
	assert.Equal(t, income.ApplicationApplied, got.Status)
}
