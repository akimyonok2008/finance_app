package profile

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

func profileTestPool(t *testing.T) *pgxpool.Pool {
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

func seedProfileUser(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	id := uuid.NewString()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO users (id, email, password_hash, display_name) VALUES ($1, $2, 'h', 'Profile User')`,
		id, id+"@example.com")
	require.NoError(t, err)
	return id
}

func TestPostgresRepositoryCreateGetUpdateAndConflict(t *testing.T) {
	ctx := context.Background()
	pool := profileTestPool(t)
	repo := NewPostgresRepository(pool)
	now := time.Now().UTC().Truncate(time.Microsecond)

	first := Profile{
		UserID: seedProfileUser(t, pool), Handle: "profile_" + uuid.NewString()[:8],
		DisplayName: "Profile One", StrategyTag: DefaultStrategyTag, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, repo.Create(ctx, first))

	byUser, err := repo.GetByUserID(ctx, first.UserID)
	require.NoError(t, err)
	assert.Equal(t, first.Handle, byUser.Handle)

	first.Bio = "Updated bio"
	first.UpdatedAt = now.Add(time.Minute)
	require.NoError(t, repo.Update(ctx, first))
	byHandle, err := repo.GetByHandle(ctx, first.Handle)
	require.NoError(t, err)
	assert.Equal(t, "Updated bio", byHandle.Bio)

	second := first
	second.UserID = seedProfileUser(t, pool)
	assert.ErrorIs(t, repo.Create(ctx, second), ErrHandleExists)
}

func TestPostgresRepositoryMissingProfile(t *testing.T) {
	repo := NewPostgresRepository(profileTestPool(t))
	_, err := repo.GetByUserID(context.Background(), uuid.NewString())
	assert.ErrorIs(t, err, ErrNotFound)
}
