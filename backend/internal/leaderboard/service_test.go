package leaderboard

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/auth"
)

// --- test doubles ------------------------------------------------------------

type fakeUsers struct {
	users []auth.User
	err   error
}

func (f fakeUsers) ListUsers(_ context.Context) ([]auth.User, error) {
	return f.users, f.err
}

// fakeRanked stands in for the ranked-performance provider.
type fakeRanked struct {
	byUser map[string]RankedPerformance
	errs   map[string]error
}

func (f fakeRanked) CurrentRankedPerformance(_ context.Context, userID string) (RankedPerformance, error) {
	if err, ok := f.errs[userID]; ok {
		return RankedPerformance{}, err
	}
	rp, ok := f.byUser[userID]
	if !ok {
		return RankedPerformance{}, errors.New("no ranked performance")
	}
	return rp, nil
}

func user(id, name string) auth.User {
	return auth.User{ID: id, Email: name + "@example.com", DisplayName: name, AvatarKey: "fox", PasswordHash: "secret-hash"}
}

// summary builds a RankedPerformance from a return percentage and index (the
// helper name is retained so existing tests read unchanged).
func summary(pct, index string) RankedPerformance {
	return RankedPerformance{RankedReturnPercentage: testRatio(pct), RankedIndex: testIndex(index)}
}

// --- tests -------------------------------------------------------------------

func TestBuild_RanksByGainLossDescending(t *testing.T) {
	users := fakeUsers{users: []auth.User{user("u1", "Alpha"), user("u2", "Beta"), user("u3", "Gamma")}}
	sums := fakeRanked{byUser: map[string]RankedPerformance{
		"u1": summary("8.1", "108.1"),
		"u2": summary("12.4", "112.4"),
		"u3": summary("-3.0", "97.0"),
	}}
	svc := NewService(users, sums)

	board, err := svc.Build(context.Background())
	require.NoError(t, err)

	require.Len(t, board, 3)
	assert.Equal(t, "Beta", board[0].DisplayName)
	assert.Equal(t, "Alpha", board[1].DisplayName)
	assert.Equal(t, "Gamma", board[2].DisplayName)
}

func TestBuild_AssignsSequentialRanks(t *testing.T) {
	users := fakeUsers{users: []auth.User{user("u1", "Alpha"), user("u2", "Beta"), user("u3", "Gamma")}}
	sums := fakeRanked{byUser: map[string]RankedPerformance{
		"u1": summary("8.1", "108.1"),
		"u2": summary("12.4", "112.4"),
		"u3": summary("5.0", "105.0"),
	}}
	svc := NewService(users, sums)

	board, err := svc.Build(context.Background())
	require.NoError(t, err)

	for i, e := range board {
		assert.Equal(t, i+1, e.Rank)
	}
}

func TestBuild_EmptyUserListReturnsEmptyBoard(t *testing.T) {
	svc := NewService(fakeUsers{users: nil}, fakeRanked{})

	board, err := svc.Build(context.Background())
	require.NoError(t, err)
	assert.Empty(t, board)
}

func TestBuild_EmptyPortfolioIsZeroAndHundred(t *testing.T) {
	users := fakeUsers{users: []auth.User{user("u1", "Alpha")}}
	sums := fakeRanked{byUser: map[string]RankedPerformance{
		"u1": summary("0", "100"), // empty portfolio convention
	}}
	svc := NewService(users, sums)

	board, err := svc.Build(context.Background())
	require.NoError(t, err)
	require.Len(t, board, 1)
	assert.Equal(t, 0.0, board[0].GainLossPercentage.Float64())
	assert.Equal(t, 100.0, board[0].PortfolioIndex.Float64())
}

func TestBuild_TieBrokenByDisplayNameAscending(t *testing.T) {
	// Insertion order intentionally non-alphabetical to prove sorting is applied.
	users := fakeUsers{users: []auth.User{user("u3", "CryptoTiger"), user("u1", "AlphaBull"), user("u2", "BetaWolf")}}
	sums := fakeRanked{byUser: map[string]RankedPerformance{
		"u1": summary("10", "110"),
		"u2": summary("10", "110"),
		"u3": summary("8", "108"),
	}}
	svc := NewService(users, sums)

	board, err := svc.Build(context.Background())
	require.NoError(t, err)

	require.Len(t, board, 3)
	assert.Equal(t, "AlphaBull", board[0].DisplayName)
	assert.Equal(t, "BetaWolf", board[1].DisplayName)
	assert.Equal(t, "CryptoTiger", board[2].DisplayName)
}

func TestBuild_SkipsUsersWhoseSummaryFails(t *testing.T) {
	users := fakeUsers{users: []auth.User{user("u1", "Alpha"), user("u2", "Beta"), user("u3", "Gamma")}}
	sums := fakeRanked{
		byUser: map[string]RankedPerformance{
			"u1": summary("8.1", "108.1"),
			"u3": summary("5.0", "105.0"),
		},
		errs: map[string]error{"u2": errors.New("price provider exploded")},
	}
	svc := NewService(users, sums)

	board, err := svc.Build(context.Background())
	require.NoError(t, err)

	require.Len(t, board, 2, "the failing user must be skipped, not crash the board")
	names := []string{board[0].DisplayName, board[1].DisplayName}
	assert.ElementsMatch(t, []string{"Alpha", "Gamma"}, names)
}

func TestBuildResult_ReportsSkippedCount(t *testing.T) {
	users := fakeUsers{users: []auth.User{user("u1", "Alpha"), user("u2", "Beta"), user("u3", "Gamma")}}
	sums := fakeRanked{
		byUser: map[string]RankedPerformance{
			"u1": summary("8.1", "108.1"),
			"u3": summary("5.0", "105.0"),
		},
		errs: map[string]error{"u2": errors.New("boom")},
	}
	svc := NewService(users, sums)

	res, err := svc.BuildResult(context.Background())
	require.NoError(t, err)
	assert.Len(t, res.Entries, 2)
	assert.Equal(t, 1, res.SkippedCount)
}

// --- cache integration (Phase 3) ----------------------------------------------

func cacheUsers() fakeUsers {
	return fakeUsers{users: []auth.User{user("u1", "Alpha"), user("u2", "Beta")}}
}

func TestBuild_UsesCacheWhenPopulated(t *testing.T) {
	// Summaries deliberately disagree with the cache so we can tell which path ran.
	sums := fakeRanked{byUser: map[string]RankedPerformance{
		"u1": summary("1.0", "101"), "u2": summary("2.0", "102"),
	}}
	svc := NewService(cacheUsers(), sums)
	cache := newTestCache(t)
	svc.SetCache(cache)

	ctx := context.Background()
	require.NoError(t, cache.UpsertGlobalScore(ctx, "u1", testRatio("50.0")))
	require.NoError(t, cache.UpsertGlobalScore(ctx, "u2", testRatio("25.0")))

	board, err := svc.Build(ctx)
	require.NoError(t, err)
	require.Len(t, board, 2)
	// Cached scores (50 > 25) win, not the live summaries (2 > 1).
	assert.Equal(t, "Alpha", board[0].DisplayName)
	assert.Equal(t, 50.0, board[0].GainLossPercentage.Float64())
	assert.Equal(t, 150.0, board[0].PortfolioIndex.Float64(), "index derives from cached score")
	assert.Equal(t, 1, board[0].Rank)
}

func TestBuild_FallsBackToLiveWhenCacheEmpty(t *testing.T) {
	sums := fakeRanked{byUser: map[string]RankedPerformance{
		"u1": summary("8.1", "108.1"), "u2": summary("12.4", "112.4"),
	}}
	svc := NewService(cacheUsers(), sums)
	svc.SetCache(newTestCache(t)) // attached but empty

	board, err := svc.Build(context.Background())
	require.NoError(t, err)
	require.Len(t, board, 2)
	assert.Equal(t, "Beta", board[0].DisplayName, "live calculation must be used when cache is empty")
	assert.InDelta(t, 12.4, board[0].GainLossPercentage.Float64(), 0.001)
}

func TestRefreshCache_PopulatesScores(t *testing.T) {
	sums := fakeRanked{byUser: map[string]RankedPerformance{
		"u1": summary("8.1", "108.1"), "u2": summary("12.4", "112.4"),
	}}
	svc := NewService(cacheUsers(), sums)
	cache := newTestCache(t)
	svc.SetCache(cache)

	ctx := context.Background()
	skipped, err := svc.RefreshCache(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, skipped)

	top, err := cache.GetGlobalTop(ctx, 10)
	require.NoError(t, err)
	require.Len(t, top, 2)
	assert.Equal(t, "u2", top[0].UserID)
	assert.InDelta(t, 12.4, top[0].Score, 0.001)
}

func TestRefreshCache_ReconcilesPausedDeletedAndPreservesValuationFailures(t *testing.T) {
	users := fakeUsers{users: []auth.User{
		user("active", "Active"), user("paused", "Paused"), user("unpriced", "Unpriced"),
	}}
	paused := summary("12", "112")
	paused.Paused = true
	sums := fakeRanked{
		byUser: map[string]RankedPerformance{
			"active": summary("8", "108"),
			"paused": paused,
		},
		errs: map[string]error{"unpriced": errors.New("temporary quote failure")},
	}
	svc := NewService(users, sums)
	cache := newTestCache(t)
	svc.SetCache(cache)
	ctx := context.Background()
	require.NoError(t, cache.UpsertGlobalScore(ctx, "paused", testRatio("12")))
	require.NoError(t, cache.UpsertGlobalScore(ctx, "deleted", testRatio("40")))
	require.NoError(t, cache.UpsertGlobalScore(ctx, "unpriced", testRatio("5")))

	skipped, err := svc.RefreshCache(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, skipped)
	top, err := cache.GetGlobalTop(ctx, 0)
	require.NoError(t, err)
	require.Len(t, top, 2)
	assert.ElementsMatch(t, []string{"active", "unpriced"},
		[]string{top[0].UserID, top[1].UserID})

	// Reconciliation is idempotent.
	skipped, err = svc.RefreshCache(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, skipped)
	top, err = cache.GetGlobalTop(ctx, 0)
	require.NoError(t, err)
	assert.Len(t, top, 2)
}

func TestRefreshCache_ConcurrentWorkersConvergeIdempotently(t *testing.T) {
	users := cacheUsers()
	sums := fakeRanked{byUser: map[string]RankedPerformance{
		"u1": summary("8.1", "108.1"), "u2": summary("12.4", "112.4"),
	}}
	cache := newTestCache(t)
	services := []*Service{NewService(users, sums), NewService(users, sums)}
	for _, svc := range services {
		svc.SetCache(cache)
	}

	var wg sync.WaitGroup
	for _, svc := range services {
		wg.Add(1)
		go func(svc *Service) {
			defer wg.Done()
			_, err := svc.RefreshCache(context.Background())
			assert.NoError(t, err)
		}(svc)
	}
	wg.Wait()
	top, err := cache.GetGlobalTop(context.Background(), 0)
	require.NoError(t, err)
	require.Len(t, top, 2)
	assert.Equal(t, "u2", top[0].UserID)
}

// TestRefreshCache_BatchedRevaluesBoundedSubsetPerCall locks in the fix for
// RefreshCache doing an unbounded full-portfolio-valuation pass over every
// user on every call (a problem on a short ticker interval as user count
// grows): with SetRefreshBatchSize(1) on three users, each call must only
// call CurrentRankedPerformance for one user, and three calls must cover all
// three without ever wrongly evicting the two users not revalued that call.
func TestRefreshCache_BatchedRevaluesBoundedSubsetPerCall(t *testing.T) {
	users := fakeUsers{users: []auth.User{user("u1", "A"), user("u2", "B"), user("u3", "C")}}
	counts := &countingRanked{byUser: map[string]RankedPerformance{
		"u1": summary("1", "101"), "u2": summary("2", "102"), "u3": summary("3", "103"),
	}}
	svc := NewService(users, counts)
	cache := newTestCache(t)
	svc.SetCache(cache)
	svc.SetRefreshBatchSize(1)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		_, err := svc.RefreshCache(ctx)
		require.NoError(t, err)
	}

	// Exactly one valuation per user per full cycle: bounded work per call,
	// not O(all users) every call.
	assert.Equal(t, 1, counts.calls["u1"])
	assert.Equal(t, 1, counts.calls["u2"])
	assert.Equal(t, 1, counts.calls["u3"])

	// All three must still be present in the cache: a user not revalued on a
	// given call must never be evicted as if deleted/unrankable.
	top, err := cache.GetGlobalTop(ctx, 0)
	require.NoError(t, err)
	assert.Len(t, top, 3)
}

// countingRanked wraps fakeRanked's data with a per-user call counter so the
// batching test can assert exactly which users were (and weren't) revalued.
type countingRanked struct {
	byUser map[string]RankedPerformance
	calls  map[string]int
}

func (c *countingRanked) CurrentRankedPerformance(_ context.Context, userID string) (RankedPerformance, error) {
	if c.calls == nil {
		c.calls = map[string]int{}
	}
	c.calls[userID]++
	rp, ok := c.byUser[userID]
	if !ok {
		return RankedPerformance{}, errors.New("no ranked performance")
	}
	return rp, nil
}

func TestCachedReadRemovesUnknownUserMember(t *testing.T) {
	svc := NewService(
		fakeUsers{users: []auth.User{user("active", "Active")}},
		fakeRanked{byUser: map[string]RankedPerformance{"active": summary("8", "108")}},
	)
	cache := newTestCache(t)
	svc.SetCache(cache)
	ctx := context.Background()
	require.NoError(t, cache.UpsertGlobalScore(ctx, "ghost", testRatio("99")))
	require.NoError(t, cache.UpsertGlobalScore(ctx, "active", testRatio("8")))

	board, err := svc.Build(ctx)
	require.NoError(t, err)
	require.Len(t, board, 1)
	assert.Equal(t, "Active", board[0].DisplayName)
	top, err := cache.GetGlobalTop(ctx, 0)
	require.NoError(t, err)
	require.Len(t, top, 1)
	assert.Equal(t, "active", top[0].UserID)
}

func TestBuild_ListUsersErrorIsReturned(t *testing.T) {
	svc := NewService(fakeUsers{err: errors.New("db down")}, fakeRanked{})

	_, err := svc.Build(context.Background())
	assert.Error(t, err)
}

// --- privacy (service level) -------------------------------------------------

func TestBuild_ResponseOmitsForbiddenFields(t *testing.T) {
	users := fakeUsers{users: []auth.User{user("u1", "Alpha")}}
	sums := fakeRanked{byUser: map[string]RankedPerformance{"u1": summary("12.4", "112.4")}}
	svc := NewService(users, sums)

	board, err := svc.Build(context.Background())
	require.NoError(t, err)

	raw, err := json.Marshal(board)
	require.NoError(t, err)
	assertNoForbiddenFields(t, string(raw))

	// Sanity: it DOES contain the allowed fields.
	body := string(raw)
	assert.Contains(t, body, `"gain_loss_percentage"`)
	assert.Contains(t, body, `"portfolio_index"`)
	assert.Contains(t, body, `"display_name"`)
	assert.Contains(t, body, `"avatar_key"`)
	assert.Contains(t, body, `"rank"`)
}

// forbiddenKeys are checked as JSON keys (quoted + colon) rather than loose
// substrings. This matters because the allowed field "gain_loss_percentage"
// legitimately contains the forbidden token "gain_loss"; checking `"gain_loss":`
// distinguishes the dollar-amount field from the percentage field.
var forbiddenKeys = []string{
	"total_cost_basis", "current_value", "gain_loss", "positions", "symbol",
	"quantity", "average_buy_price", "email", "password", "password_hash",
	"portfolio_id", "user_id",
}

func assertNoForbiddenFields(t *testing.T, body string) {
	t.Helper()
	for _, k := range forbiddenKeys {
		assert.NotContainsf(t, body, `"`+k+`":`, "leaderboard response must not expose %q", k)
	}
}
