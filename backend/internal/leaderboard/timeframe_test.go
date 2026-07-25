package leaderboard

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/auth"
)

type fakeProfiles struct{ byUser map[string]ProfilePublicInfo }

func (f fakeProfiles) PublicInfo(_ context.Context, userID string) (ProfilePublicInfo, bool, error) {
	info, ok := f.byUser[userID]
	return info, ok, nil
}

func TestParseTimeframe(t *testing.T) {
	assert.Equal(t, Timeframe1W, ParseTimeframe("1W"))
	assert.Equal(t, Timeframe1Y, ParseTimeframe("1y"))
	assert.Equal(t, TimeframeAll, ParseTimeframe(""))
	assert.Equal(t, TimeframeAll, ParseTimeframe("nonsense"))
}

func TestBuildTimeframe_WindowedReturnFromSnapshots(t *testing.T) {
	users := fakeUsers{users: []auth.User{user("u1", "Alpha"), user("u2", "Beta")}}
	sums := fakeRanked{byUser: map[string]RankedPerformance{
		"u1": summary(20, 120), // current index 120
		"u2": summary(5, 105),  // current index 105
	}}
	svc := NewService(users, sums)
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	store := NewInMemorySnapshotStore()
	svc.SetSnapshotStore(store)
	weekAgo := now.Add(-7 * 24 * time.Hour)
	require.NoError(t, store.Record(context.Background(), "u1", 110, weekAgo))
	require.NoError(t, store.Record(context.Background(), "u2", 104, weekAgo))

	board, err := svc.BuildTimeframe(context.Background(), Timeframe1W)
	require.NoError(t, err)
	require.Len(t, board, 2)
	// u1 1W = (120/110-1)*100 = 9.09; u2 = (105/104-1)*100 = 0.96 → u1 leads.
	assert.Equal(t, "Alpha", board[0].DisplayName)
	assert.InDelta(t, 9.09, board[0].RankedReturnPercentage, 0.05)
	assert.InDelta(t, 109.09, board[0].RankedIndex, 0.05)
}

func TestBuildTimeframe_ExcludesUsersWithoutOldEnoughSnapshot(t *testing.T) {
	users := fakeUsers{users: []auth.User{user("u1", "Alpha")}}
	sums := fakeRanked{byUser: map[string]RankedPerformance{"u1": summary(20, 120)}}
	svc := NewService(users, sums)
	svc.SetSnapshotStore(NewInMemorySnapshotStore()) // empty history

	board, err := svc.BuildTimeframe(context.Background(), Timeframe1M)
	require.NoError(t, err)
	assert.Empty(t, board)
}

func TestBuild_ExcludesPausedUsers(t *testing.T) {
	users := fakeUsers{users: []auth.User{user("u1", "Alpha"), user("u2", "Beta")}}
	sums := fakeRanked{byUser: map[string]RankedPerformance{
		"u1": {RankedIndex: 130, RankedReturnPercentage: 30},
		"u2": {RankedIndex: 150, RankedReturnPercentage: 50, Paused: true}, // empty portfolio
	}}
	svc := NewService(users, sums)

	board, err := svc.Build(context.Background())
	require.NoError(t, err)
	require.Len(t, board, 1, "paused (empty-portfolio) user must be excluded from active ranking")
	assert.Equal(t, "Alpha", board[0].DisplayName)
}

func TestBuildTimeframe_IgnoresPreEpochSnapshots(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	epoch := now.Add(-3 * 24 * time.Hour) // ranking epoch 3 days ago
	users := fakeUsers{users: []auth.User{user("u1", "Alpha")}}
	sums := fakeRanked{byUser: map[string]RankedPerformance{
		"u1": {RankedIndex: 120, RankedReturnPercentage: 20, TrackingStartedAt: epoch},
	}}
	svc := NewService(users, sums)
	svc.now = func() time.Time { return now }

	store := NewInMemorySnapshotStore()
	svc.SetSnapshotStore(store)
	// Only a legacy snapshot from before the epoch exists (a week ago).
	require.NoError(t, store.Record(context.Background(), "u1", 300, now.Add(-7*24*time.Hour)))

	board, err := svc.BuildTimeframe(context.Background(), Timeframe1W)
	require.NoError(t, err)
	assert.Empty(t, board, "pre-epoch legacy snapshot must be ignored -> not enough ranked history")
}

func TestBuild_EnrichesPublicProfilesOnly(t *testing.T) {
	users := fakeUsers{users: []auth.User{user("u1", "Alpha"), user("u2", "Beta")}}
	sums := fakeRanked{byUser: map[string]RankedPerformance{
		"u1": summary(10, 110),
		"u2": summary(5, 105),
	}}
	svc := NewService(users, sums)
	svc.SetProfileProvider(fakeProfiles{byUser: map[string]ProfilePublicInfo{
		"u1": {Handle: "alpha", StrategyTag: "growth", IsPublic: true, ShowWeights: true,
			Weights: []PublicWeight{{Symbol: "AAPL", AssetType: "stock", WeightPercentage: 60}}},
		"u2": {Handle: "beta", StrategyTag: "value", IsPublic: false, ShowWeights: true},
	}})

	board, err := svc.Build(context.Background())
	require.NoError(t, err)

	byName := map[string]LeaderboardEntry{}
	for _, e := range board {
		byName[e.DisplayName] = e
	}
	// Public profile → enriched.
	assert.Equal(t, "alpha", byName["Alpha"].Handle)
	assert.Equal(t, "growth", byName["Alpha"].StrategyTag)
	require.Len(t, byName["Alpha"].PublicWeights, 1)
	assert.Equal(t, "AAPL", byName["Alpha"].PublicWeights[0].Symbol)
	// Private profile → stays anonymous.
	assert.Empty(t, byName["Beta"].Handle)
	assert.Empty(t, byName["Beta"].PublicWeights)
}

func TestBuild_WeightsHiddenWhenShowWeightsFalse(t *testing.T) {
	users := fakeUsers{users: []auth.User{user("u1", "Alpha")}}
	sums := fakeRanked{byUser: map[string]RankedPerformance{"u1": summary(10, 110)}}
	svc := NewService(users, sums)
	svc.SetProfileProvider(fakeProfiles{byUser: map[string]ProfilePublicInfo{
		"u1": {Handle: "alpha", StrategyTag: "growth", IsPublic: true, ShowWeights: false,
			Weights: []PublicWeight{{Symbol: "AAPL", AssetType: "stock", WeightPercentage: 60}}},
	}})

	board, err := svc.Build(context.Background())
	require.NoError(t, err)
	require.Len(t, board, 1)
	// Public but weights hidden: handle/tag shown, weights withheld.
	assert.Equal(t, "alpha", board[0].Handle)
	assert.Empty(t, board[0].PublicWeights)
}

func TestUserStanding(t *testing.T) {
	users := fakeUsers{users: []auth.User{user("u1", "Alpha"), user("u2", "Beta"), user("u3", "Gamma")}}
	sums := fakeRanked{byUser: map[string]RankedPerformance{
		"u1": summary(8, 108),
		"u2": summary(12, 112),
		"u3": summary(-3, 97),
	}}
	svc := NewService(users, sums)

	// Ranking: Beta(12) #1, Alpha(8) #2, Gamma(-3) #3.
	st, err := svc.UserStanding(context.Background(), "u1", TimeframeAll)
	require.NoError(t, err)
	assert.True(t, st.Ranked)
	assert.Equal(t, 2, st.Rank)
	assert.Equal(t, 3, st.ParticipantCount)
	assert.Equal(t, 2, *st.BestRank)
	assert.InDelta(t, 66.67, st.Percentile, 0.01)
	require.NotNil(t, st.NextMilestone)
	assert.Equal(t, "#1", st.NextMilestone.Label)
	assert.Equal(t, 1, st.NextMilestone.RankGap)
	assert.InDelta(t, 4.0, st.NextMilestone.ReturnGapPercentage, 0.01)
	assert.InDelta(t, 8.0, st.RankedReturnPercentage, 0.01)

	// Unknown user: not ranked, but total still reported.
	ghost, err := svc.UserStanding(context.Background(), "ghost", TimeframeAll)
	require.NoError(t, err)
	assert.False(t, ghost.Ranked)
	assert.Equal(t, 0, ghost.Rank)
	assert.Equal(t, 3, ghost.ParticipantCount)
	assert.Equal(t, "Create a strategy baseline to enter the leaderboard.", ghost.Reason)
}

func TestUserStanding_WindowedIneligibleWhenHistoryMissing(t *testing.T) {
	users := fakeUsers{users: []auth.User{user("u1", "Alpha")}}
	sums := fakeRanked{byUser: map[string]RankedPerformance{"u1": summary(8, 108)}}
	svc := NewService(users, sums)
	svc.SetSnapshotStore(NewInMemorySnapshotStore())

	st, err := svc.UserStanding(context.Background(), "u1", Timeframe1W)
	require.NoError(t, err)
	assert.False(t, st.Ranked)
	assert.Equal(t, 0, st.ParticipantCount)
	assert.Equal(t, "Not enough ranked history for this timeframe yet.", st.Reason)
}

func TestUserStanding_PausedOverridesAnyCachedRankAndPreservesIndex(t *testing.T) {
	users := fakeUsers{users: []auth.User{user("u1", "Alpha")}}
	paused := RankedPerformance{
		RankedIndex: 112, RankedReturnPercentage: 12, Paused: true,
	}
	svc := NewService(users, fakeRanked{byUser: map[string]RankedPerformance{"u1": paused}})
	cache := newTestCache(t)
	require.NoError(t, cache.UpsertGlobalScore(context.Background(), "u1", 12))
	svc.SetCache(cache)

	st, err := svc.UserStanding(context.Background(), "u1", TimeframeAll)
	require.NoError(t, err)
	assert.False(t, st.Ranked)
	assert.True(t, st.Paused)
	assert.Equal(t, 112.0, st.RankedIndex)
	assert.Equal(t, 12.0, st.RankedReturnPercentage)
	assert.Contains(t, st.Reason, "paused")
}

func TestGetUserRank_PausedStateEvictsStaleCachedRank(t *testing.T) {
	users := fakeUsers{users: []auth.User{user("u1", "Alpha")}}
	paused := RankedPerformance{
		RankedIndex: 112, RankedReturnPercentage: 12, Paused: true,
	}
	svc := NewService(users, fakeRanked{byUser: map[string]RankedPerformance{"u1": paused}})
	cache := newTestCache(t)
	require.NoError(t, cache.UpsertGlobalScore(context.Background(), "u1", 12))
	svc.SetCache(cache)

	rank, err := svc.GetUserRank(context.Background(), "u1")
	require.NoError(t, err)
	assert.Zero(t, rank)
	top, err := cache.GetGlobalTop(context.Background(), 0)
	require.NoError(t, err)
	assert.Empty(t, top)
}

func TestRefreshCache_DoesNotWriteIndependentSnapshotHistory(t *testing.T) {
	users := fakeUsers{users: []auth.User{user("u1", "Alpha")}}
	sums := fakeRanked{byUser: map[string]RankedPerformance{"u1": summary(15, 115)}}
	svc := NewService(users, sums)
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	store := NewInMemorySnapshotStore()
	svc.SetSnapshotStore(store)

	skipped, err := svc.RefreshCache(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, skipped)

	idx, found, err := store.IndexAtOrBefore(context.Background(), "u1", now, time.Time{})
	require.NoError(t, err)
	assert.False(t, found, "canonical ranked-snapshot worker owns history writes")
	assert.Zero(t, idx)
}
