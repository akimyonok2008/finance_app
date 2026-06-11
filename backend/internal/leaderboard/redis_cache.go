package leaderboard

import (
	"context"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"
)

const globalKey = "leaderboard:global"

func competitionKey(competitionID string) string {
	return "leaderboard:competition:" + competitionID
}

// RedisLeaderboardCache stores rankings in Redis sorted sets. Members are user
// ids and scores are performance percentages — no portfolio values are ever
// written to Redis.
type RedisLeaderboardCache struct {
	client *redis.Client
}

// NewRedisLeaderboardCache wraps a connected Redis client.
func NewRedisLeaderboardCache(client *redis.Client) *RedisLeaderboardCache {
	return &RedisLeaderboardCache{client: client}
}

func (c *RedisLeaderboardCache) UpsertGlobalScore(ctx context.Context, userID string, score float64) error {
	return c.upsert(ctx, globalKey, userID, score)
}

func (c *RedisLeaderboardCache) UpsertCompetitionScore(ctx context.Context, competitionID, userID string, score float64) error {
	return c.upsert(ctx, competitionKey(competitionID), userID, score)
}

func (c *RedisLeaderboardCache) upsert(ctx context.Context, key, userID string, score float64) error {
	if err := c.client.ZAdd(ctx, key, redis.Z{Score: score, Member: userID}).Err(); err != nil {
		return fmt.Errorf("leaderboard cache: upsert %s: %w", key, err)
	}
	return nil
}

func (c *RedisLeaderboardCache) GetGlobalTop(ctx context.Context, limit int) ([]CachedLeaderboardScore, error) {
	return c.top(ctx, globalKey, limit)
}

func (c *RedisLeaderboardCache) GetCompetitionTop(ctx context.Context, competitionID string, limit int) ([]CachedLeaderboardScore, error) {
	return c.top(ctx, competitionKey(competitionID), limit)
}

func (c *RedisLeaderboardCache) top(ctx context.Context, key string, limit int) ([]CachedLeaderboardScore, error) {
	zs, err := c.client.ZRevRangeWithScores(ctx, key, 0, int64(limit-1)).Result()
	if err != nil {
		return nil, fmt.Errorf("leaderboard cache: top %s: %w", key, err)
	}
	out := make([]CachedLeaderboardScore, 0, len(zs))
	for _, z := range zs {
		member, _ := z.Member.(string)
		out = append(out, CachedLeaderboardScore{UserID: member, Score: z.Score})
	}
	return out, nil
}

func (c *RedisLeaderboardCache) GetGlobalRank(ctx context.Context, userID string) (int, error) {
	return c.rank(ctx, globalKey, userID)
}

func (c *RedisLeaderboardCache) GetCompetitionRank(ctx context.Context, competitionID, userID string) (int, error) {
	return c.rank(ctx, competitionKey(competitionID), userID)
}

// rank returns the 1-based descending rank, or 0 when the member is absent.
func (c *RedisLeaderboardCache) rank(ctx context.Context, key, userID string) (int, error) {
	r, err := c.client.ZRevRank(ctx, key, userID).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, nil
		}
		return 0, fmt.Errorf("leaderboard cache: rank %s: %w", key, err)
	}
	return int(r) + 1, nil
}
