package leaderboard

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/money"
)

// fakeRankingStore is an in-memory RankingStore double: it lets tests prove
// that Build/BuildTimeframe/GetUserRank/UserStanding prefer the projection
// (by giving it values that deliberately disagree with the live fakeRanked
// data) and correctly fall back when it's stale or missing a row.
type fakeRankingStore struct {
	users map[string]auth.User
	rows  map[Timeframe]map[string]fakeRankingRow
	clock func() time.Time
}

type fakeRankingRow struct {
	idx        money.IndexValue
	retPct     money.Ratio
	computedAt time.Time
}

func newFakeRankingStore(users []auth.User) *fakeRankingStore {
	byID := make(map[string]auth.User, len(users))
	for _, u := range users {
		byID[u.ID] = u
	}
	return &fakeRankingStore{
		users: byID,
		rows:  make(map[Timeframe]map[string]fakeRankingRow),
		clock: func() time.Time { return time.Now().UTC() },
	}
}

func (f *fakeRankingStore) Upsert(_ context.Context, tf Timeframe, userID string, idx money.IndexValue, retPct money.Ratio, _ time.Time) error {
	if f.rows[tf] == nil {
		f.rows[tf] = map[string]fakeRankingRow{}
	}
	f.rows[tf][userID] = fakeRankingRow{idx: idx, retPct: retPct, computedAt: f.clock()}
	return nil
}

func (f *fakeRankingStore) Delete(_ context.Context, tf Timeframe, userID string) error {
	delete(f.rows[tf], userID)
	return nil
}

func (f *fakeRankingStore) rowsFor(tf Timeframe) []rankedRow {
	out := make([]rankedRow, 0, len(f.rows[tf]))
	for userID, r := range f.rows[tf] {
		u := f.users[userID]
		e := rankedEntry(0, u.DisplayName, u.AvatarKey, r.retPct, r.idx)
		out = append(out, rankedRow{userID: userID, entry: e, returnPct: r.retPct, rankedIndex: r.idx})
	}
	sortRankedRows(out)
	return out
}

func (f *fakeRankingStore) TopPage(_ context.Context, tf Timeframe, limit int) ([]rankedRow, error) {
	rows := f.rowsFor(tf)
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return rows, nil
}

func (f *fakeRankingStore) RankOf(_ context.Context, tf Timeframe, userID string) (int, money.IndexValue, money.Ratio, bool, error) {
	for _, r := range f.rowsFor(tf) {
		if r.userID == userID {
			return r.entry.Rank, r.rankedIndex, r.returnPct, true, nil
		}
	}
	return 0, money.ZeroIndexValue(), money.ZeroRatio(), false, nil
}

func (f *fakeRankingStore) ValueAtRank(_ context.Context, tf Timeframe, rank int) (money.Ratio, bool, error) {
	rows := f.rowsFor(tf)
	if rank < 1 || rank > len(rows) {
		return money.ZeroRatio(), false, nil
	}
	return rows[rank-1].returnPct, true, nil
}

func (f *fakeRankingStore) Count(_ context.Context, tf Timeframe) (int, error) {
	return len(f.rows[tf]), nil
}

func (f *fakeRankingStore) OldestComputedAt(_ context.Context, tf Timeframe) (time.Time, bool, error) {
	var oldest time.Time
	found := false
	for _, r := range f.rows[tf] {
		if !found || r.computedAt.Before(oldest) {
			oldest, found = r.computedAt, true
		}
	}
	return oldest, found, nil
}

func TestBuild_PrefersFreshRankingProjectionOverLiveValues(t *testing.T) {
	users := []auth.User{user("u1", "Alpha"), user("u2", "Beta")}
	// Live values would rank Beta first; the projection deliberately disagrees
	// so the test proves the projection (not live) is what's actually used.
	sums := fakeRanked{byUser: map[string]RankedPerformance{
		"u1": summary("8", "108"), "u2": summary("12", "112"),
	}}
	svc := NewService(fakeUsers{users: users}, sums)
	ranking := newFakeRankingStore(users)
	svc.SetRankingStore(ranking)

	ctx := context.Background()
	require.NoError(t, ranking.Upsert(ctx, TimeframeAll, "u1", testIndex("120"), testRatio("20"), time.Time{}))
	require.NoError(t, ranking.Upsert(ctx, TimeframeAll, "u2", testIndex("105"), testRatio("5"), time.Time{}))

	board, err := svc.Build(ctx)
	require.NoError(t, err)
	require.Len(t, board, 2)
	assert.Equal(t, "Alpha", board[0].DisplayName, "the projection ranks Alpha first; a live fallback would rank Beta first")
	assert.Equal(t, 20.0, board[0].RankedReturnPercentage.Float64())
}

func TestBuild_FallsBackToLiveWhenRankingStale(t *testing.T) {
	users := []auth.User{user("u1", "Alpha"), user("u2", "Beta")}
	sums := fakeRanked{byUser: map[string]RankedPerformance{
		"u1": summary("8", "108"), "u2": summary("12", "112"),
	}}
	svc := NewService(fakeUsers{users: users}, sums)
	ranking := newFakeRankingStore(users)
	epoch := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ranking.clock = func() time.Time { return epoch }
	svc.SetRankingStore(ranking)
	// "Now" is far past the projection's freshness window.
	svc.now = func() time.Time { return epoch.Add(2 * time.Hour) }

	ctx := context.Background()
	require.NoError(t, ranking.Upsert(ctx, TimeframeAll, "u1", testIndex("999"), testRatio("899"), time.Time{}))
	require.NoError(t, ranking.Upsert(ctx, TimeframeAll, "u2", testIndex("999"), testRatio("899"), time.Time{}))

	board, err := svc.Build(ctx)
	require.NoError(t, err)
	require.Len(t, board, 2)
	assert.Equal(t, "Beta", board[0].DisplayName, "a stale projection must not be trusted")
	assert.Equal(t, 12.0, board[0].RankedReturnPercentage.Float64())
}

func TestGetUserRank_UsesRankingWhenFresh(t *testing.T) {
	users := []auth.User{user("u1", "Alpha"), user("u2", "Beta"), user("u3", "Gamma")}
	sums := fakeRanked{byUser: map[string]RankedPerformance{
		"u1": summary("8", "108"), "u2": summary("12", "112"), "u3": summary("-3", "97"),
	}}
	svc := NewService(fakeUsers{users: users}, sums)
	ranking := newFakeRankingStore(users)
	svc.SetRankingStore(ranking)
	ctx := context.Background()
	for _, u := range users {
		rp := sums.byUser[u.ID]
		require.NoError(t, ranking.Upsert(ctx, TimeframeAll, u.ID, rp.RankedIndex, rp.RankedReturnPercentage, time.Time{}))
	}

	rank, err := svc.GetUserRank(ctx, "u1")
	require.NoError(t, err)
	assert.Equal(t, 2, rank)
}

func TestGetUserRank_FallsBackToLiveWhenMissingFromFreshRanking(t *testing.T) {
	// A fresh-but-incomplete projection (e.g. a brand-new user not yet reached
	// by a refresh cycle) must not be reported as "unranked" — it must fall
	// back to the live computation rather than guessing.
	users := []auth.User{user("u1", "Alpha"), user("u2", "Beta")}
	sums := fakeRanked{byUser: map[string]RankedPerformance{
		"u1": summary("8", "108"), "u2": summary("12", "112"),
	}}
	svc := NewService(fakeUsers{users: users}, sums)
	ranking := newFakeRankingStore(users)
	svc.SetRankingStore(ranking)
	ctx := context.Background()
	// Only u2 has been refreshed into the projection; u1 has not yet.
	require.NoError(t, ranking.Upsert(ctx, TimeframeAll, "u2", testIndex("112"), testRatio("12"), time.Time{}))

	rank, err := svc.GetUserRank(ctx, "u1")
	require.NoError(t, err)
	assert.Equal(t, 2, rank, "must fall back to the live full-scan rank rather than reporting unranked")
}

// TestUserRankings_PrefersRankingProjectionWhenFresh locks in the fix for the
// one read path the original ranking-projection change missed: UserRankings
// (the full per-user join Explore uses) previously always called rankRows
// (one live valuation per user) regardless of a fresh projection.
func TestUserRankings_PrefersRankingProjectionWhenFresh(t *testing.T) {
	users := []auth.User{user("u1", "Alpha"), user("u2", "Beta")}
	// Live values would rank Beta first; the projection deliberately disagrees
	// so the test proves the projection (not live) is what's actually used.
	sums := fakeRanked{byUser: map[string]RankedPerformance{
		"u1": summary("8", "108"), "u2": summary("12", "112"),
	}}
	svc := NewService(fakeUsers{users: users}, sums)
	ranking := newFakeRankingStore(users)
	svc.SetRankingStore(ranking)
	ctx := context.Background()
	require.NoError(t, ranking.Upsert(ctx, TimeframeAll, "u1", testIndex("120"), testRatio("20"), time.Time{}))
	require.NoError(t, ranking.Upsert(ctx, TimeframeAll, "u2", testIndex("105"), testRatio("5"), time.Time{}))

	rows, err := svc.UserRankings(ctx, TimeframeAll)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, "u1", rows[0].UserID, "the projection ranks u1 first; a live fallback would rank u2 first")
	assert.Equal(t, 1, rows[0].Rank)
	assert.Equal(t, 20.0, rows[0].RankedReturnPercentage.Float64())
}

func TestUserRankings_FallsBackToLiveWhenRankingStale(t *testing.T) {
	users := []auth.User{user("u1", "Alpha"), user("u2", "Beta")}
	sums := fakeRanked{byUser: map[string]RankedPerformance{
		"u1": summary("8", "108"), "u2": summary("12", "112"),
	}}
	svc := NewService(fakeUsers{users: users}, sums)
	ranking := newFakeRankingStore(users)
	epoch := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ranking.clock = func() time.Time { return epoch }
	svc.SetRankingStore(ranking)
	svc.now = func() time.Time { return epoch.Add(2 * time.Hour) }
	ctx := context.Background()
	require.NoError(t, ranking.Upsert(ctx, TimeframeAll, "u1", testIndex("999"), testRatio("899"), time.Time{}))
	require.NoError(t, ranking.Upsert(ctx, TimeframeAll, "u2", testIndex("999"), testRatio("899"), time.Time{}))

	rows, err := svc.UserRankings(ctx, TimeframeAll)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, "u2", rows[0].UserID, "a stale projection must not be trusted")
}

func TestUserStanding_UsesRankingWhenFresh(t *testing.T) {
	users := []auth.User{user("u1", "Alpha"), user("u2", "Beta"), user("u3", "Gamma")}
	sums := fakeRanked{byUser: map[string]RankedPerformance{
		"u1": summary("8", "108"), "u2": summary("12", "112"), "u3": summary("-3", "97"),
	}}
	svc := NewService(fakeUsers{users: users}, sums)
	ranking := newFakeRankingStore(users)
	svc.SetRankingStore(ranking)
	ctx := context.Background()
	for _, u := range users {
		rp := sums.byUser[u.ID]
		require.NoError(t, ranking.Upsert(ctx, TimeframeAll, u.ID, rp.RankedIndex, rp.RankedReturnPercentage, time.Time{}))
	}

	st, err := svc.UserStanding(ctx, "u1", TimeframeAll)
	require.NoError(t, err)
	assert.True(t, st.Ranked)
	assert.Equal(t, 2, st.Rank)
	assert.Equal(t, 3, st.ParticipantCount)
	require.NotNil(t, st.NextMilestone)
	assert.Equal(t, "#1", st.NextMilestone.Label)
	assert.InDelta(t, 4.0, st.NextMilestone.ReturnGapPercentage.Float64(), 0.01)
}
