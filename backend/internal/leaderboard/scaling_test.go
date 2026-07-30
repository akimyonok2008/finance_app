package leaderboard

// Tests for the population-scaling behaviors: the adaptive projection trust
// window (CycleDurationReporter), keyset-paged batch selection
// (PagedUserProvider), and bounded-concurrency batch processing.

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/money"
)

// adaptiveFakeRankingStore adds the optional CycleDurationReporter capability
// to the shared fakeRankingStore.
type adaptiveFakeRankingStore struct {
	*fakeRankingStore
	lastCycle      time.Duration
	lastCycleKnown bool
}

func (a *adaptiveFakeRankingStore) LastCycleDuration(context.Context) (time.Duration, bool, error) {
	return a.lastCycle, a.lastCycleKnown, nil
}

func TestRankingServable_AdaptiveWindowScalesWithCycleDuration(t *testing.T) {
	users := []auth.User{user("u1", "Alpha")}
	svc := NewService(fakeUsers{users: users}, fakeRanked{byUser: map[string]RankedPerformance{
		"u1": summary("1", "101"),
	}})
	base := newFakeRankingStore(users)
	store := &adaptiveFakeRankingStore{fakeRankingStore: base}
	svc.SetRankingStore(store)

	now := time.Now().UTC()
	svc.now = func() time.Time { return now }
	ctx := context.Background()

	// Never promoted: not servable at all — this is the only state that
	// sends reads to the live path.
	servable, _, _ := svc.rankingServable(ctx)
	assert.False(t, servable, "an unpromoted projection must not be servable")

	base.hasActive = true

	// 40 minutes old with no measured cycle: servable but past the fixed
	// 30m floor, so not fresh.
	base.activatedAt = now.Add(-40 * time.Minute)
	servable, fresh, _ := svc.rankingServable(ctx)
	assert.True(t, servable)
	assert.False(t, fresh, "without a measured cycle duration, only the fixed floor applies")

	// Same age, but the last cycle measurably took 25 minutes: the window
	// stretches to 2x25m = 50m, so a 40-minute-old generation stays fresh
	// while the next one is (necessarily) still building.
	store.lastCycleKnown = true
	store.lastCycle = 25 * time.Minute
	_, fresh, _ = svc.rankingServable(ctx)
	assert.True(t, fresh, "freshness window must scale with the measured cycle duration")

	// A short cycle can't shrink the window below the floor.
	store.lastCycle = 5 * time.Minute
	_, fresh, _ = svc.rankingServable(ctx)
	assert.False(t, fresh, "floor still applies when cycles are fast")
	base.activatedAt = now.Add(-20 * time.Minute)
	_, fresh, _ = svc.rankingServable(ctx)
	assert.True(t, fresh)

	// The hard cap bounds the adaptive window; past it the generation is
	// stale (alert-worthy) but — crucially — still servable.
	base.activatedAt = now.Add(-25 * time.Hour)
	store.lastCycle = 20 * time.Hour // adaptive would be 40h without the cap
	servable, fresh, _ = svc.rankingServable(ctx)
	assert.True(t, servable, "a stale projection must remain servable")
	assert.False(t, fresh, "hard cap must override the adaptive window")
}

// TestBuild_ServesStaleProjectionOverLivePath is the degrade-don't-compute
// contract: once any generation has been promoted, reads keep serving it no
// matter how old it gets — the O(N) live path must never reactivate merely
// because the refresh pipeline fell behind.
func TestBuild_ServesStaleProjectionOverLivePath(t *testing.T) {
	users := []auth.User{user("u1", "Alpha"), user("u2", "Beta")}
	// Live values would rank Beta first; the (stale) projection disagrees,
	// proving which path actually served the read.
	sums := fakeRanked{byUser: map[string]RankedPerformance{
		"u1": summary("8", "108"), "u2": summary("12", "112"),
	}}
	svc := NewService(fakeUsers{users: users}, sums)
	ranking := newFakeRankingStore(users)
	svc.SetRankingStore(ranking)
	ctx := context.Background()

	require.NoError(t, ranking.Upsert(ctx, TimeframeAll, "u1", testIndex("120"), testRatio("20"), time.Time{}))
	require.NoError(t, ranking.Upsert(ctx, TimeframeAll, "u2", testIndex("105"), testRatio("5"), time.Time{}))
	require.NoError(t, ranking.CompleteCycle(ctx))
	// Age the generation far past even the 24h hard cap.
	ranking.activatedAt = time.Now().UTC().Add(-48 * time.Hour)

	board, err := svc.Build(ctx)
	require.NoError(t, err)
	require.Len(t, board, 2)
	assert.Equal(t, "Alpha", board[0].DisplayName,
		"the stale projection, not the live values, must serve the board")
}

// TestColdStart_LiveComputeBounded proves the cold-start guard: with no
// generation ever promoted and a population above the live-compute bound,
// reads degrade softly instead of valuing every user per request.
func TestColdStart_LiveComputeBounded(t *testing.T) {
	users := []auth.User{
		user("u1", "Alpha"), user("u2", "Beta"), user("u3", "Gamma"),
	}
	perf := map[string]RankedPerformance{
		"u1": summary("8", "108"), "u2": summary("12", "112"), "u3": summary("1", "101"),
	}
	provider := &fakePagedUsers{users: users}
	svc := NewService(provider, fakeRanked{byUser: perf})
	svc.SetRankingStore(newFakeRankingStore(users)) // present but never promoted
	svc.SetMaxLiveComputeUsers(2)                   // population of 3 exceeds the bound
	ctx := context.Background()

	board, err := svc.Build(ctx)
	require.NoError(t, err)
	assert.Empty(t, board, "cold start at scale must degrade to an empty board, not an error")

	rank, err := svc.GetUserRank(ctx, "u2")
	require.NoError(t, err)
	assert.Zero(t, rank, "cold start at scale must report unranked")

	st, err := svc.UserStanding(ctx, "u2", TimeframeAll)
	require.NoError(t, err)
	assert.False(t, st.Ranked)
	assert.NotEmpty(t, st.Reason, "standing must explain that rankings are being prepared")

	// Background rebuilds must NOT silently see an empty ranking: Explore's
	// refresh aborts and keeps its previous generation published.
	_, err = svc.UserRankings(ctx, TimeframeAll)
	assert.ErrorIs(t, err, ErrRankingUnavailable)

	// A small population is still allowed to compute live.
	svc.SetMaxLiveComputeUsers(10)
	board, err = svc.Build(ctx)
	require.NoError(t, err)
	require.Len(t, board, 3)
	assert.Equal(t, "Beta", board[0].DisplayName)
}

// fakePagedUsers implements UserProvider plus the PagedUserProvider
// capability, counting full-population listings so tests can prove the paged
// refresh path never does one mid-lap.
type fakePagedUsers struct {
	users     []auth.User
	fullLists int
	pageCalls int
}

func (f *fakePagedUsers) ListRankableUsers(_ context.Context) ([]auth.RankableUser, error) {
	f.fullLists++
	out := make([]auth.RankableUser, 0, len(f.users))
	for _, u := range f.users {
		out = append(out, auth.RankableUser{ID: u.ID, DisplayName: u.DisplayName, AvatarKey: u.AvatarKey})
	}
	return out, nil
}

func (f *fakePagedUsers) ListRankableUsersPage(_ context.Context, afterID string, limit int) ([]auth.RankableUser, error) {
	f.pageCalls++
	sorted := make([]auth.User, len(f.users))
	copy(sorted, f.users)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	out := make([]auth.RankableUser, 0, limit)
	for _, u := range sorted {
		if u.ID <= afterID {
			continue
		}
		out = append(out, auth.RankableUser{ID: u.ID, DisplayName: u.DisplayName, AvatarKey: u.AvatarKey})
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func TestRefreshCache_PagedSelectionPromotesAfterFullLap(t *testing.T) {
	users := []auth.User{
		user("u1", "Alpha"), user("u2", "Beta"), user("u3", "Gamma"),
		user("u4", "Delta"), user("u5", "Epsilon"),
	}
	perf := map[string]RankedPerformance{}
	for i, u := range users {
		perf[u.ID] = summary("1", "101")
		_ = i
	}
	provider := &fakePagedUsers{users: users}
	svc := NewService(provider, fakeRanked{byUser: perf})
	ranking := newFakeRankingStore(users)
	svc.SetRankingStore(ranking)
	svc.SetRefreshBatchSize(2)
	ctx := context.Background()

	// Lap of 5 users at batch size 2: two full pages, then a short page that
	// completes the cycle and promotes the generation.
	for call, wantPromoted := range []bool{false, false, true} {
		_, err := svc.RefreshCache(ctx)
		require.NoError(t, err)
		assert.Equal(t, wantPromoted, svc.LastRefreshPromotedGeneration(),
			"promotion after call %d", call+1)
		_, found, err := ranking.ActiveGenerationAge(ctx)
		require.NoError(t, err)
		assert.Equal(t, wantPromoted, found,
			"active generation visibility after call %d", call+1)
	}
	n, err := ranking.Count(ctx, TimeframeAll)
	require.NoError(t, err)
	assert.Equal(t, 5, n, "the promoted generation must cover every user")

	// The whole lap ran off bounded page queries: with no Redis cache
	// attached there is never a reason to list the full population.
	assert.Equal(t, 0, provider.fullLists,
		"paged refresh must not enumerate the full population")
	assert.Equal(t, 3, provider.pageCalls)

	// The next call starts a fresh lap: promotion flag drops back.
	_, err = svc.RefreshCache(ctx)
	require.NoError(t, err)
	assert.False(t, svc.LastRefreshPromotedGeneration())
}

// lockedRankingStore makes the shared fakeRankingStore safe under
// processBatch's goroutine fan-out.
type lockedRankingStore struct {
	mu sync.Mutex
	f  *fakeRankingStore
}

func (l *lockedRankingStore) Upsert(ctx context.Context, tf Timeframe, userID string, idx money.IndexValue, retPct money.Ratio, at time.Time) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.f.Upsert(ctx, tf, userID, idx, retPct, at)
}

func (l *lockedRankingStore) Delete(ctx context.Context, tf Timeframe, userID string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.f.Delete(ctx, tf, userID)
}

func (l *lockedRankingStore) TopPage(ctx context.Context, tf Timeframe, limit int) ([]rankedRow, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.f.TopPage(ctx, tf, limit)
}

func (l *lockedRankingStore) RankOf(ctx context.Context, tf Timeframe, userID string) (int, money.IndexValue, money.Ratio, bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.f.RankOf(ctx, tf, userID)
}

func (l *lockedRankingStore) ValueAtRank(ctx context.Context, tf Timeframe, rank int) (money.Ratio, bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.f.ValueAtRank(ctx, tf, rank)
}

func (l *lockedRankingStore) Count(ctx context.Context, tf Timeframe) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.f.Count(ctx, tf)
}

func (l *lockedRankingStore) ActiveGenerationAge(ctx context.Context) (time.Time, bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.f.ActiveGenerationAge(ctx)
}

func (l *lockedRankingStore) CompleteCycle(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.f.CompleteCycle(ctx)
}

func TestRefreshCache_ParallelBatchCoversEveryUser(t *testing.T) {
	var users []auth.User
	perf := map[string]RankedPerformance{}
	for i := 0; i < 30; i++ {
		u := user(string(rune('a'+i/10))+string(rune('0'+i%10)), "User")
		users = append(users, u)
		perf[u.ID] = summary("1", "101")
	}
	svc := NewService(fakeUsers{users: users}, fakeRanked{byUser: perf})
	ranking := &lockedRankingStore{f: newFakeRankingStore(users)}
	svc.SetRankingStore(ranking)
	svc.SetRefreshParallelism(8)
	ctx := context.Background()

	_, err := svc.RefreshCache(ctx)
	require.NoError(t, err)
	require.True(t, svc.LastRefreshPromotedGeneration())
	n, err := ranking.Count(ctx, TimeframeAll)
	require.NoError(t, err)
	assert.Equal(t, 30, n, "concurrent processing must still cover every user")
}
