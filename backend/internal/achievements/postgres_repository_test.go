package achievements

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

func TestPostgresAchievementRepository_SeedsAndLists(t *testing.T) {
	ctx := context.Background()
	repo, err := NewPostgresAchievementRepository(ctx, testPool(t))
	require.NoError(t, err)

	list, err := repo.ListAchievements(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(list), 5)

	byKey := map[string]bool{}
	for _, a := range list {
		byKey[a.Key] = true
	}
	for _, k := range []string{KeyFirstPortfolio, KeyFirstSprint, KeyGreenPortfolio, KeyIndex110, KeyTop10Sprint} {
		assert.Truef(t, byKey[k], "catalogue must contain %s", k)
	}
}

func TestPostgresAchievementRepository_SeedingIsIdempotent(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	_, err := NewPostgresAchievementRepository(ctx, pool)
	require.NoError(t, err)
	repo, err := NewPostgresAchievementRepository(ctx, pool) // second construction
	require.NoError(t, err)

	list, err := repo.ListAchievements(ctx)
	require.NoError(t, err)
	keys := map[string]int{}
	for _, a := range list {
		keys[a.Key]++
	}
	for k, n := range keys {
		assert.Equalf(t, 1, n, "achievement %s must not be duplicated by re-seeding", k)
	}
}

func TestPostgresAchievementRepository_UnlockIdempotentAndListable(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	repo, err := NewPostgresAchievementRepository(ctx, pool)
	require.NoError(t, err)
	userID := seedPGUser(t, pool)

	ach, err := repo.GetAchievementByKey(ctx, KeyFirstPortfolio)
	require.NoError(t, err)

	has, err := repo.HasAchievement(ctx, userID, ach.ID)
	require.NoError(t, err)
	assert.False(t, has)

	require.NoError(t, repo.UnlockAchievement(ctx, userID, ach.ID))
	require.NoError(t, repo.UnlockAchievement(ctx, userID, ach.ID)) // idempotent

	has, err = repo.HasAchievement(ctx, userID, ach.ID)
	require.NoError(t, err)
	assert.True(t, has)

	list, err := repo.ListUserAchievements(ctx, userID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, ach.ID, list[0].AchievementID)
}

func TestPostgresAchievementRepository_GetByMissingKey(t *testing.T) {
	ctx := context.Background()
	repo, err := NewPostgresAchievementRepository(ctx, testPool(t))
	require.NoError(t, err)
	_, err = repo.GetAchievementByKey(ctx, "nope_"+uuid.NewString())
	assert.ErrorIs(t, err, ErrAchievementNotFound)
}
