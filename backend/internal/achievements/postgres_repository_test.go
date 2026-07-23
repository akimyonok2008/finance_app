package achievements

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/benchmark"
	"github.com/ardakimyonok/finance_app/internal/db"
)

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

func seedPGUser(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	id := uuid.NewString()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO users (id, email, password_hash, display_name) VALUES ($1, $2, 'h', 'Test')`,
		id, id+"@example.com")
	require.NoError(t, err)
	return id
}

func TestPostgresAchievementRepository_AwardAndList(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	repo, err := NewPostgresAchievementRepository(ctx, pool)
	require.NoError(t, err)

	userID := seedPGUser(t, pool)
	award := AwardedAchievement{
		UserID:     userID,
		BadgeKey:   "bogle_badge_90d",
		UnlockedAt: time.Now().UTC().Truncate(time.Second),
		Evidence: benchmark.AchievementEvidence{
			Period:             benchmark.Period90D,
			StartDate:          "2026-04-23",
			EndDate:            "2026-07-22",
			PortfolioReturnPct: 7,
			BenchmarkReturnPct: 4.5,
			EdgePoints:         2.5,
			BenchmarkRecipeID:  "VOO",
		},
	}
	require.NoError(t, repo.Award(ctx, award))
	// Idempotent second award keeps the first record.
	require.NoError(t, repo.Award(ctx, award))

	awarded, err := repo.ListAwarded(ctx, userID)
	require.NoError(t, err)
	require.Len(t, awarded, 1)

	got, ok := awarded["bogle_badge_90d"]
	require.True(t, ok)
	assert.Equal(t, 2.5, got.Evidence.EdgePoints)
	assert.Equal(t, "VOO", got.Evidence.BenchmarkRecipeID)
}
