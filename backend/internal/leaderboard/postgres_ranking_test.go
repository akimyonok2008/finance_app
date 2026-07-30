package leaderboard

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/db"
)

func rankingTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL_TEST")
	if url == "" {
		t.Skip("DATABASE_URL_TEST not set; skipping Postgres integration test")
	}
	pool, err := db.ConnectPostgres(context.Background(), url)
	require.NoError(t, err)
	require.NoError(t, db.RunMigrations(context.Background(), pool))
	// The suite shares one database across tests (no per-test transaction
	// isolation); start every test from an empty projection, with no
	// generation ever activated, so Count/TopPage/ActiveGenerationAge
	// assertions aren't polluted by rows or promotions other tests left
	// behind.
	_, err = pool.Exec(context.Background(), `TRUNCATE leaderboard_rankings`)
	require.NoError(t, err)
	_, err = pool.Exec(context.Background(), `
		UPDATE leaderboard_ranking_state
		SET active_generation = 0, building_generation = 1, cycle_started_at = now(),
		    activated_at = NULL, last_cycle_seconds = NULL
		WHERE id
	`)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

func seedRankingUser(t *testing.T, pool *pgxpool.Pool, displayName string) string {
	t.Helper()
	id := uuid.NewString()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO users (id, email, password_hash, display_name) VALUES ($1, $2, 'h', $3)`,
		id, id+"@example.com", displayName)
	require.NoError(t, err)
	return id
}

func TestPostgresRankingStore_TopPageRanksAndTieBreaks(t *testing.T) {
	pool := rankingTestPool(t)
	store := NewPostgresRankingStore(pool)
	ctx := context.Background()
	epoch := time.Now().UTC()

	alpha := seedRankingUser(t, pool, "AlphaBull")
	beta := seedRankingUser(t, pool, "BetaWolf")
	gamma := seedRankingUser(t, pool, "CryptoTiger")

	// alpha and beta tie on return% — must break by display_name ascending.
	require.NoError(t, store.Upsert(ctx, TimeframeAll, beta, testIndex("110"), testRatio("10"), epoch))
	require.NoError(t, store.Upsert(ctx, TimeframeAll, alpha, testIndex("110"), testRatio("10"), epoch))
	require.NoError(t, store.Upsert(ctx, TimeframeAll, gamma, testIndex("108"), testRatio("8"), epoch))
	require.NoError(t, store.CompleteCycle(ctx))

	rows, err := store.TopPage(ctx, TimeframeAll, 100)
	require.NoError(t, err)
	require.Len(t, rows, 3)
	assert.Equal(t, "AlphaBull", rows[0].entry.DisplayName)
	assert.Equal(t, 1, rows[0].entry.Rank)
	assert.Equal(t, "BetaWolf", rows[1].entry.DisplayName)
	assert.Equal(t, 2, rows[1].entry.Rank)
	assert.Equal(t, "CryptoTiger", rows[2].entry.DisplayName)
	assert.Equal(t, 3, rows[2].entry.Rank)

	n, err := store.Count(ctx, TimeframeAll)
	require.NoError(t, err)
	assert.Equal(t, 3, n)

	rank, idx, retPct, found, err := store.RankOf(ctx, TimeframeAll, gamma)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, 3, rank)
	assert.Equal(t, 108.0, idx.Float64())
	assert.Equal(t, 8.0, retPct.Float64())

	valAtTwo, found, err := store.ValueAtRank(ctx, TimeframeAll, 2)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, 10.0, valAtTwo.Float64())

	_, _, _, found, err = store.RankOf(ctx, TimeframeAll, uuid.NewString())
	require.NoError(t, err)
	assert.False(t, found, "an unknown user must report found=false, not an error")
}

// TestPostgresRankingStore_DeleteRemovesRow proves Delete removes a user from
// the NEXT generation to be promoted, not the one currently being read —
// exclusion, like inclusion, only takes effect once a full cycle completes.
func TestPostgresRankingStore_DeleteRemovesRow(t *testing.T) {
	pool := rankingTestPool(t)
	store := NewPostgresRankingStore(pool)
	ctx := context.Background()
	user := seedRankingUser(t, pool, "Solo")

	require.NoError(t, store.Upsert(ctx, TimeframeAll, user, testIndex("120"), testRatio("20"), time.Now().UTC()))
	require.NoError(t, store.CompleteCycle(ctx))
	n, err := store.Count(ctx, TimeframeAll)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	require.NoError(t, store.Delete(ctx, TimeframeAll, user))
	require.NoError(t, store.CompleteCycle(ctx))
	n, err = store.Count(ctx, TimeframeAll)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
}

func TestPostgresRankingStore_ActiveGenerationAgeReportsFreshness(t *testing.T) {
	pool := rankingTestPool(t)
	store := NewPostgresRankingStore(pool)
	ctx := context.Background()

	_, found, err := store.ActiveGenerationAge(ctx)
	require.NoError(t, err)
	assert.False(t, found, "before any cycle completes, freshness must report not-found, not a zero time")

	user := seedRankingUser(t, pool, "Fresh")
	require.NoError(t, store.Upsert(ctx, TimeframeAll, user, testIndex("100"), testRatio("0"), time.Now().UTC()))
	// Upsert alone must not activate anything — it only writes the building
	// generation, which stays invisible until CompleteCycle promotes it.
	_, found, err = store.ActiveGenerationAge(ctx)
	require.NoError(t, err)
	assert.False(t, found, "an unpromoted (building) generation must not count as fresh")

	require.NoError(t, store.CompleteCycle(ctx))
	at, found, err := store.ActiveGenerationAge(ctx)
	require.NoError(t, err)
	require.True(t, found)
	assert.WithinDuration(t, time.Now().UTC(), at, 5*time.Second)
}

func TestPostgresRankingStore_LastCycleDurationRecordedOnPromotion(t *testing.T) {
	pool := rankingTestPool(t)
	store := NewPostgresRankingStore(pool)
	ctx := context.Background()

	_, known, err := store.LastCycleDuration(ctx)
	require.NoError(t, err)
	assert.False(t, known, "no cycle has completed yet")

	// Backdate the running cycle's start so the promoted duration is
	// measurable and deterministic (roughly 10 minutes).
	_, err = pool.Exec(ctx, `
		UPDATE leaderboard_ranking_state SET cycle_started_at = now() - interval '10 minutes' WHERE id
	`)
	require.NoError(t, err)
	user := seedRankingUser(t, pool, "Cycler")
	require.NoError(t, store.Upsert(ctx, TimeframeAll, user, testIndex("100"), testRatio("0"), time.Now().UTC()))
	require.NoError(t, store.CompleteCycle(ctx))

	d, known, err := store.LastCycleDuration(ctx)
	require.NoError(t, err)
	require.True(t, known, "promotion must record the build-cycle duration")
	assert.InDelta(t, (10 * time.Minute).Seconds(), d.Seconds(), 30,
		"recorded duration must reflect how long the generation took to build")
}

func TestPostgresRankingStore_UpsertOverwritesPreviousValue(t *testing.T) {
	pool := rankingTestPool(t)
	store := NewPostgresRankingStore(pool)
	ctx := context.Background()
	user := seedRankingUser(t, pool, "Reval")

	epoch := time.Now().UTC()
	require.NoError(t, store.Upsert(ctx, TimeframeAll, user, testIndex("100"), testRatio("0"), epoch))
	require.NoError(t, store.Upsert(ctx, TimeframeAll, user, testIndex("115"), testRatio("15"), epoch))
	require.NoError(t, store.CompleteCycle(ctx))

	rank, idx, retPct, found, err := store.RankOf(ctx, TimeframeAll, user)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, 1, rank)
	assert.Equal(t, 115.0, idx.Float64())
	assert.Equal(t, 15.0, retPct.Float64())
}

func TestPostgresRankingStore_DeletedUserCascadesOut(t *testing.T) {
	pool := rankingTestPool(t)
	store := NewPostgresRankingStore(pool)
	ctx := context.Background()
	user := seedRankingUser(t, pool, "Gone")

	require.NoError(t, store.Upsert(ctx, TimeframeAll, user, testIndex("100"), testRatio("0"), time.Now().UTC()))
	require.NoError(t, store.CompleteCycle(ctx))
	_, err := pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, user)
	require.NoError(t, err)

	n, err := store.Count(ctx, TimeframeAll)
	require.NoError(t, err)
	assert.Equal(t, 0, n, "a deleted user's ranking row must be gone via ON DELETE CASCADE, not linger as a ghost")
}
