package corpactions_test

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

	"github.com/ardakimyonok/finance_app/internal/corpactions"
	"github.com/ardakimyonok/finance_app/internal/db"
)

// These integration tests run only when DATABASE_URL_TEST points at a disposable
// database. They verify the durable store's uniqueness and transactional
// claiming, which are what stop two workers applying the same event twice.
func caTestPool(t *testing.T) *pgxpool.Pool {
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

// seedCAUserPortfolio inserts a user + portfolio so application FKs resolve.
func seedCAUserPortfolio(t *testing.T, pool *pgxpool.Pool) (userID, portfolioID string) {
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

func caEvent(id string) corpactions.CorporateAction {
	num := 4.0
	den := 1.0
	return corpactions.CorporateAction{
		ID: id, Provider: "manual_dev", ProviderEventID: id, Type: corpactions.TypeSplit,
		Source:      corpactions.InstrumentReference{Symbol: "AAPL"},
		EffectiveAt: time.Now().UTC(), Status: corpactions.StatusValidated,
		Quality: corpactions.QualityVerified, RatioNumerator: &num, RatioDenominator: &den,
		RawFingerprint: "fp-1", RetrievedAt: time.Now().UTC(),
	}
}

func TestPG_EventUniquenessAndCorrection(t *testing.T) {
	pool := caTestPool(t)
	store := corpactions.NewPostgresStore(pool)
	ctx := context.Background()
	id := "manual_dev:SPLIT-" + uuid.NewString()

	changed, err := store.UpsertEvent(ctx, caEvent(id))
	require.NoError(t, err)
	assert.False(t, changed, "first insert is not a correction")

	// Same fingerprint again: idempotent, no change.
	changed, err = store.UpsertEvent(ctx, caEvent(id))
	require.NoError(t, err)
	assert.False(t, changed)

	// Changed terms → correction detected.
	ev := caEvent(id)
	ev.RawFingerprint = "fp-2"
	num := 3.0
	ev.RatioNumerator = &num
	changed, err = store.UpsertEvent(ctx, ev)
	require.NoError(t, err)
	assert.True(t, changed, "changed fingerprint must be reported as a correction")

	got, ok, err := store.GetEvent(ctx, id)
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, got.RatioNumerator)
	assert.InDelta(t, 3, *got.RatioNumerator, 1e-9)

	// The DB enforces (provider, provider_event_id) uniqueness.
	var count int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM corporate_actions WHERE provider='manual_dev' AND provider_event_id=$1`,
		id).Scan(&count))
	assert.Equal(t, 1, count)
}

func TestPG_ApplicationClaimIsExclusive(t *testing.T) {
	pool := caTestPool(t)
	store := corpactions.NewPostgresStore(pool)
	ctx := context.Background()
	userID, portfolioID := seedCAUserPortfolio(t, pool)
	id := "manual_dev:SPLIT-" + uuid.NewString()
	_, err := store.UpsertEvent(ctx, caEvent(id))
	require.NoError(t, err)

	// Many concurrent workers claim the same (event, portfolio): exactly one wins.
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

	wins := 0
	for _, r := range results {
		if r {
			wins++
		}
	}
	assert.Equal(t, 1, wins, "only one worker may claim an application")

	// The primary key enforces one application row per (event, portfolio).
	var count int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM corporate_action_applications WHERE corporate_action_id=$1 AND portfolio_id=$2`,
		id, portfolioID).Scan(&count))
	assert.Equal(t, 1, count)
}
