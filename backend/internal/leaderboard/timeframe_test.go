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
		"u1": summary("20", "120"), // current index 120
		"u2": summary("5", "105"),  // current index 105
	}}
	svc := NewService(users, sums)
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	store := NewInMemorySnapshotStore()
	svc.SetSnapshotStore(store)
	weekAgo := now.Add(-7 * 24 * time.Hour)
	require.NoError(t, store.Record(context.Background(), "u1", testIndex("110"), weekAgo))
	require.NoError(t, store.Record(context.Background(), "u2", testIndex("104"), weekAgo))

	board, err := svc.BuildTimeframe(context.Background(), Timeframe1W)
	require.NoError(t, err)
	require.Len(t, board, 2)
	// u1 1W = (120/110-1)*100 = 9.09; u2 = (105/104-1)*100 = 0.96 → u1 leads.
	assert.Equal(t, "Alpha", board[0].DisplayName)
	assert.InDelta(t, 9.09, board[0].RankedReturnPercentage.Float64(), 0.05)
	assert.InDelta(t, 109.09, board[0].RankedIndex.Float64(), 0.05)
}

// TestBuildTimeframe_ExcludesUserWhenSnapshotGapExceedsMaxAge: a base snapshot
// that IS at-or-before the window's cutoff but sits well before it (a missed
// snapshot run, a paused-then-resumed account) must not be used — otherwise a
// board labeled "1W" would silently measure a much longer real span for that
// user.
func TestBuildTimeframe_ExcludesUserWhenSnapshotGapExceedsMaxAge(t *testing.T) {
	users := fakeUsers{users: []auth.User{user("u1", "Alpha")}}
	sums := fakeRanked{byUser: map[string]RankedPerformance{"u1": summary("20", "120")}}
	svc := NewService(users, sums)
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	store := NewInMemorySnapshotStore()
	svc.SetSnapshotStore(store)

	// Timeframe1W cutoff = now - 7d. Record a snapshot 2 days further back
	// than that — a 48h gap, beyond the 36h default tolerance.
	tooOld := now.Add(-7 * 24 * time.Hour).Add(-2 * 24 * time.Hour)
	require.NoError(t, store.Record(context.Background(), "u1", testIndex("110"), tooOld))

	board, err := svc.BuildTimeframe(context.Background(), Timeframe1W)
	require.NoError(t, err)
	assert.Empty(t, board, "a snapshot gap beyond the max-age tolerance must exclude the user, not silently stretch the window")
}

// TestBuildTimeframe_IncludesUserWithinMaxAgeTolerance is the boundary case:
// a snapshot slightly before cutoff (within tolerance) must still count.
func TestBuildTimeframe_IncludesUserWithinMaxAgeTolerance(t *testing.T) {
	users := fakeUsers{users: []auth.User{user("u1", "Alpha")}}
	sums := fakeRanked{byUser: map[string]RankedPerformance{"u1": summary("20", "120")}}
	svc := NewService(users, sums)
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	store := NewInMemorySnapshotStore()
	svc.SetSnapshotStore(store)

	// 1 hour before cutoff — well within the 36h default tolerance.
	closeEnough := now.Add(-7 * 24 * time.Hour).Add(-time.Hour)
	require.NoError(t, store.Record(context.Background(), "u1", testIndex("110"), closeEnough))

	board, err := svc.BuildTimeframe(context.Background(), Timeframe1W)
	require.NoError(t, err)
	require.Len(t, board, 1)
}

// TestSetMaxSnapshotAge_OverridesDefault confirms the tolerance is actually
// configurable rather than hardcoded.
func TestSetMaxSnapshotAge_OverridesDefault(t *testing.T) {
	users := fakeUsers{users: []auth.User{user("u1", "Alpha")}}
	sums := fakeRanked{byUser: map[string]RankedPerformance{"u1": summary("20", "120")}}
	svc := NewService(users, sums)
	svc.SetMaxSnapshotAge(48 * time.Hour) // wider than the 36h default
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	store := NewInMemorySnapshotStore()
	svc.SetSnapshotStore(store)

	// A 40h gap: excluded by the 36h default, but allowed by the wider 48h
	// tolerance configured above.
	gap := now.Add(-7 * 24 * time.Hour).Add(-40 * time.Hour)
	require.NoError(t, store.Record(context.Background(), "u1", testIndex("110"), gap))

	board, err := svc.BuildTimeframe(context.Background(), Timeframe1W)
	require.NoError(t, err)
	require.Len(t, board, 1, "a wider configured tolerance must be honored")
}

func TestBuildTimeframe_ExcludesUsersWithoutOldEnoughSnapshot(t *testing.T) {
	users := fakeUsers{users: []auth.User{user("u1", "Alpha")}}
	sums := fakeRanked{byUser: map[string]RankedPerformance{"u1": summary("20", "120")}}
	svc := NewService(users, sums)
	svc.SetSnapshotStore(NewInMemorySnapshotStore()) // empty history

	board, err := svc.BuildTimeframe(context.Background(), Timeframe1M)
	require.NoError(t, err)
	assert.Empty(t, board)
}

func TestBuild_ExcludesPausedUsers(t *testing.T) {
	users := fakeUsers{users: []auth.User{user("u1", "Alpha"), user("u2", "Beta")}}
	sums := fakeRanked{byUser: map[string]RankedPerformance{
		"u1": {RankedIndex: testIndex("130"), RankedReturnPercentage: testRatio("30")},
		"u2": {RankedIndex: testIndex("150"), RankedReturnPercentage: testRatio("50"), Paused: true}, // empty portfolio
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
		"u1": {RankedIndex: testIndex("120"), RankedReturnPercentage: testRatio("20"), TrackingStartedAt: epoch},
	}}
	svc := NewService(users, sums)
	svc.now = func() time.Time { return now }

	store := NewInMemorySnapshotStore()
	svc.SetSnapshotStore(store)
	// Only a legacy snapshot from before the epoch exists (a week ago).
	require.NoError(t, store.Record(context.Background(), "u1", testIndex("300"), now.Add(-7*24*time.Hour)))

	board, err := svc.BuildTimeframe(context.Background(), Timeframe1W)
	require.NoError(t, err)
	assert.Empty(t, board, "pre-epoch legacy snapshot must be ignored -> not enough ranked history")
}

func TestBuild_EnrichesPublicProfilesOnly(t *testing.T) {
	users := fakeUsers{users: []auth.User{user("u1", "Alpha"), user("u2", "Beta")}}
	sums := fakeRanked{byUser: map[string]RankedPerformance{
		"u1": summary("10", "110"),
		"u2": summary("5", "105"),
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
	sums := fakeRanked{byUser: map[string]RankedPerformance{"u1": summary("10", "110")}}
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
		"u1": summary("8", "108"),
		"u2": summary("12", "112"),
		"u3": summary("-3", "97"),
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
	assert.InDelta(t, 4.0, st.NextMilestone.ReturnGapPercentage.Float64(), 0.01)
	assert.InDelta(t, 8.0, st.RankedReturnPercentage.Float64(), 0.01)

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
	sums := fakeRanked{byUser: map[string]RankedPerformance{"u1": summary("8", "108")}}
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
		RankedIndex: testIndex("112"), RankedReturnPercentage: testRatio("12"), Paused: true,
	}
	svc := NewService(users, fakeRanked{byUser: map[string]RankedPerformance{"u1": paused}})
	cache := newTestCache(t)
	require.NoError(t, cache.UpsertGlobalScore(context.Background(), "u1", testRatio("12")))
	svc.SetCache(cache)

	st, err := svc.UserStanding(context.Background(), "u1", TimeframeAll)
	require.NoError(t, err)
	assert.False(t, st.Ranked)
	assert.True(t, st.Paused)
	assert.Equal(t, 112.0, st.RankedIndex.Float64())
	assert.Equal(t, 12.0, st.RankedReturnPercentage.Float64())
	assert.Contains(t, st.Reason, "paused")
}

func TestGetUserRank_PausedStateEvictsStaleCachedRank(t *testing.T) {
	users := fakeUsers{users: []auth.User{user("u1", "Alpha")}}
	paused := RankedPerformance{
		RankedIndex: testIndex("112"), RankedReturnPercentage: testRatio("12"), Paused: true,
	}
	svc := NewService(users, fakeRanked{byUser: map[string]RankedPerformance{"u1": paused}})
	cache := newTestCache(t)
	require.NoError(t, cache.UpsertGlobalScore(context.Background(), "u1", testRatio("12")))
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
	sums := fakeRanked{byUser: map[string]RankedPerformance{"u1": summary("15", "115")}}
	svc := NewService(users, sums)
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	store := NewInMemorySnapshotStore()
	svc.SetSnapshotStore(store)

	skipped, err := svc.RefreshCache(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, skipped)

	idx, _, found, err := store.IndexAtOrBefore(context.Background(), "u1", now, time.Time{})
	require.NoError(t, err)
	assert.False(t, found, "canonical ranked-snapshot worker owns history writes")
	assert.Equal(t, 0, idx.Cmp(testIndex("0")))
}
