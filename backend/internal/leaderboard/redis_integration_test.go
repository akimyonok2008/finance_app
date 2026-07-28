package leaderboard

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/money"
)

// These tests use a real disposable Redis database. They are skipped unless
// REDIS_URL_TEST is set (use a dedicated DB number, never a shared app DB).
func realRedisCache(t *testing.T) (*RedisLeaderboardCache, *redis.Client) {
	t.Helper()
	rawURL := os.Getenv("REDIS_URL_TEST")
	if rawURL == "" {
		t.Skip("REDIS_URL_TEST not set; skipping Redis integration test")
	}
	options, err := redis.ParseURL(rawURL)
	require.NoError(t, err)
	client := redis.NewClient(options)
	ctx := context.Background()
	require.NoError(t, client.Ping(ctx).Err())
	require.NoError(t, client.FlushDB(ctx).Err())
	t.Cleanup(func() {
		_ = client.FlushDB(context.Background()).Err()
		_ = client.Close()
	})
	return NewRedisLeaderboardCache(client), client
}

func TestRedisIntegration_GlobalMembershipAndConcurrentUpdates(t *testing.T) {
	cache, _ := realRedisCache(t)
	ctx := context.Background()
	require.NoError(t, cache.UpsertGlobalScore(ctx, "u1", testRatio("5")))
	require.NoError(t, cache.UpsertGlobalScore(ctx, "u2", testRatio("10")))
	require.NoError(t, cache.RemoveGlobalScore(ctx, "missing"))
	require.NoError(t, cache.RemoveGlobalScore(ctx, "u2"))

	top, err := cache.GetGlobalTop(ctx, 0)
	require.NoError(t, err)
	require.Len(t, top, 1)
	assert.Equal(t, "u1", top[0].UserID)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(score float64) {
			defer wg.Done()
			assert.NoError(t, cache.UpsertGlobalScore(ctx, "u1", money.RatioFromFloat64(score)))
		}(float64(i))
	}
	wg.Wait()
	top, err = cache.GetGlobalTop(ctx, 0)
	require.NoError(t, err)
	require.Len(t, top, 1, "concurrent ZADD operations must not duplicate a member")
	assert.GreaterOrEqual(t, top[0].Score, 0.0)
	assert.LessOrEqual(t, top[0].Score, 19.0)
}
