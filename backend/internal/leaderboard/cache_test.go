package leaderboard

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestCache(t *testing.T) *RedisLeaderboardCache {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewRedisLeaderboardCache(client)
}

func TestRedisCache_GlobalUpsertAndTop(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()

	require.NoError(t, c.UpsertGlobalScore(ctx, "u1", 8.1))
	require.NoError(t, c.UpsertGlobalScore(ctx, "u2", 12.4))
	require.NoError(t, c.UpsertGlobalScore(ctx, "u3", -3.0))
	// Upsert overwrites the previous score.
	require.NoError(t, c.UpsertGlobalScore(ctx, "u1", 9.9))

	top, err := c.GetGlobalTop(ctx, 10)
	require.NoError(t, err)
	require.Len(t, top, 3)
	assert.Equal(t, "u2", top[0].UserID)
	assert.InDelta(t, 12.4, top[0].Score, 1e-9)
	assert.Equal(t, "u1", top[1].UserID)
	assert.InDelta(t, 9.9, top[1].Score, 1e-9)
	assert.Equal(t, "u3", top[2].UserID)
}

func TestRedisCache_GlobalTopRespectsLimit(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()
	for _, u := range []string{"a", "b", "c", "d"} {
		require.NoError(t, c.UpsertGlobalScore(ctx, u, 1))
	}
	top, err := c.GetGlobalTop(ctx, 2)
	require.NoError(t, err)
	assert.Len(t, top, 2)
}

func TestRedisCache_EmptyGlobalTopIsEmpty(t *testing.T) {
	c := newTestCache(t)
	top, err := c.GetGlobalTop(context.Background(), 10)
	require.NoError(t, err)
	assert.Empty(t, top)
}

func TestRedisCache_CompetitionScores(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()

	require.NoError(t, c.UpsertCompetitionScore(ctx, "weekly_2026_24", "u1", 5.0))
	require.NoError(t, c.UpsertCompetitionScore(ctx, "weekly_2026_24", "u2", 8.4))
	require.NoError(t, c.UpsertCompetitionScore(ctx, "weekly_2026_25", "u3", 99)) // other sprint

	top, err := c.GetCompetitionTop(ctx, "weekly_2026_24", 10)
	require.NoError(t, err)
	require.Len(t, top, 2, "scores must be isolated per competition")
	assert.Equal(t, "u2", top[0].UserID)
}

func TestRedisCache_Ranks(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()
	require.NoError(t, c.UpsertGlobalScore(ctx, "u1", 5))
	require.NoError(t, c.UpsertGlobalScore(ctx, "u2", 10))
	require.NoError(t, c.UpsertCompetitionScore(ctx, "comp", "u1", 3))

	rank, err := c.GetGlobalRank(ctx, "u2")
	require.NoError(t, err)
	assert.Equal(t, 1, rank)
	rank, err = c.GetGlobalRank(ctx, "u1")
	require.NoError(t, err)
	assert.Equal(t, 2, rank)

	// Unknown member ranks 0 (not ranked), not an error.
	rank, err = c.GetGlobalRank(ctx, "ghost")
	require.NoError(t, err)
	assert.Equal(t, 0, rank)

	rank, err = c.GetCompetitionRank(ctx, "comp", "u1")
	require.NoError(t, err)
	assert.Equal(t, 1, rank)
}

func TestNoopCache_AlwaysEmpty(t *testing.T) {
	c := NoopLeaderboardCache{}
	ctx := context.Background()
	require.NoError(t, c.UpsertGlobalScore(ctx, "u1", 5))
	top, err := c.GetGlobalTop(ctx, 10)
	require.NoError(t, err)
	assert.Empty(t, top)
	rank, err := c.GetGlobalRank(ctx, "u1")
	require.NoError(t, err)
	assert.Equal(t, 0, rank)
}
