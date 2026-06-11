package leaderboard

import "context"

// CachedLeaderboardScore is one member of a cached ranking. It carries only the
// user id and the performance score — never portfolio values. Display metadata
// (name, avatar) is joined from the user repository at read time.
type CachedLeaderboardScore struct {
	UserID string
	Score  float64
}

// LeaderboardCache is a fast ranking store (Redis sorted sets in production).
// It is an optimization only: services must fall back to live calculation when
// the cache is empty or unavailable.
type LeaderboardCache interface {
	UpsertGlobalScore(ctx context.Context, userID string, score float64) error
	GetGlobalTop(ctx context.Context, limit int) ([]CachedLeaderboardScore, error)

	UpsertCompetitionScore(ctx context.Context, competitionID, userID string, score float64) error
	GetCompetitionTop(ctx context.Context, competitionID string, limit int) ([]CachedLeaderboardScore, error)

	// Rank getters return 0 (not an error) when the user is not in the set.
	GetGlobalRank(ctx context.Context, userID string) (int, error)
	GetCompetitionRank(ctx context.Context, competitionID, userID string) (int, error)
}

// NoopLeaderboardCache satisfies LeaderboardCache but stores nothing; used in
// tests and when Redis is disabled so callers always take the live path.
type NoopLeaderboardCache struct{}

func (NoopLeaderboardCache) UpsertGlobalScore(context.Context, string, float64) error { return nil }
func (NoopLeaderboardCache) GetGlobalTop(context.Context, int) ([]CachedLeaderboardScore, error) {
	return nil, nil
}
func (NoopLeaderboardCache) UpsertCompetitionScore(context.Context, string, string, float64) error {
	return nil
}
func (NoopLeaderboardCache) GetCompetitionTop(context.Context, string, int) ([]CachedLeaderboardScore, error) {
	return nil, nil
}
func (NoopLeaderboardCache) GetGlobalRank(context.Context, string) (int, error) { return 0, nil }
func (NoopLeaderboardCache) GetCompetitionRank(context.Context, string, string) (int, error) {
	return 0, nil
}
