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

func TestPostgresExploreProjectionFiltersSortsAndPaginatesInDatabase(t *testing.T) {
	ctx := context.Background()
	pool := profileTestPool(t)
	repo := NewPostgresRepository(pool)
	now := time.Now().UTC().Truncate(time.Microsecond)

	u1 := seedProfileUser(t, pool)
	u2 := seedProfileUser(t, pool)
	require.NoError(t, repo.Create(ctx, Profile{
		UserID: u1, Handle: "alpha_projection", DisplayName: "Alpha",
		StrategyTag: DefaultStrategyTag, IsPublic: true, ShowPublicWeights: true,
		CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.Create(ctx, Profile{
		UserID: u2, Handle: "beta_projection", DisplayName: "Beta",
		StrategyTag: DefaultStrategyTag, IsPublic: true, ShowPublicWeights: true,
		CreatedAt: now, UpdatedAt: now,
	}))
	rank1, rank2 := 1, 2
	rows := []ExploreProjectionRow{
		{
			UserID: u1, Timeframe: TimeframeAll, UpdatedAt: now.Add(-time.Hour),
			Card: PublicProfile{
				Handle: "alpha_projection", DisplayName: "Alpha", GlobalRank: &rank2,
				PortfolioIndex: 110, ReturnPercentage: 10,
				PublicWeights: []PublicWeight{
					{Symbol: "AAPL", AssetType: "stock", Weight: 60},
					{Symbol: "MSFT", AssetType: "stock", Weight: 40},
				},
			},
		},
		{
			UserID: u2, Timeframe: TimeframeAll, UpdatedAt: now,
			Card: PublicProfile{
				Handle: "beta_projection", DisplayName: "Beta", GlobalRank: &rank1,
				PortfolioIndex: 120, ReturnPercentage: 20,
				PublicWeights: []PublicWeight{
					{Symbol: "AAPL", AssetType: "stock", Weight: 30},
					{Symbol: "NVDA", AssetType: "stock", Weight: 70},
				},
			},
		},
	}
	require.NoError(t, repo.ReplaceExploreProjection(ctx, rows))

	snapshot, found, err := repo.LoadExploreProjection(ctx, ExploreFilter{
		Query: "projection", Symbol: "AAPL", Sort: SortRank,
		Timeframe: TimeframeAll, Limit: 1, Offset: 0,
	}, nil, 1)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, 2, snapshot.Total)
	require.Len(t, snapshot.Page, 1)
	assert.Equal(t, "beta_projection", snapshot.Page[0].Card.Handle)
	assert.Len(t, snapshot.Candidates, 1)
	require.NotEmpty(t, snapshot.Trending)
	assert.Equal(t, "AAPL", snapshot.Trending[0].Symbol)
	assert.Equal(t, 2, snapshot.Trending[0].ProfileCount)

	blocked, found, err := repo.LoadExploreProjection(ctx, ExploreFilter{
		Sort: SortTop, Timeframe: TimeframeAll, Limit: 10,
	}, []string{u2}, 10)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, 1, blocked.Total)
	require.Len(t, blocked.Page, 1)
	assert.Equal(t, "alpha_projection", blocked.Page[0].Card.Handle)
	require.NotEmpty(t, blocked.Trending)
	assert.Equal(t, "AAPL", blocked.Trending[0].Symbol)
	assert.Equal(t, 1, blocked.Trending[0].ProfileCount)
	assert.Equal(t, 60.0, blocked.Trending[0].AverageWeight)

	// Privacy changes must take effect immediately even if the materialized
	// card generation has not refreshed yet.
	_, err = pool.Exec(ctx, `
		UPDATE profiles
		SET is_public = FALSE, show_public_weights = FALSE
		WHERE user_id = $1
	`, u2)
	require.NoError(t, err)
	privacyFiltered, found, err := repo.LoadExploreProjection(ctx, ExploreFilter{
		Sort: SortTop, Timeframe: TimeframeAll, Limit: 10,
	}, nil, 10)
	require.NoError(t, err)
	assert.False(t, found, "privacy update must invalidate the old generation immediately")
	assert.Empty(t, privacyFiltered.Page)
}
