package leaderboard

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
)

// --- test doubles ------------------------------------------------------------

type fakeUsers struct {
	users []auth.User
	err   error
}

func (f fakeUsers) ListUsers(_ context.Context) ([]auth.User, error) {
	return f.users, f.err
}

type fakeSummaries struct {
	byUser map[string]*portfolio.PortfolioSummary
	errs   map[string]error
}

func (f fakeSummaries) Summary(_ context.Context, userID string) (*portfolio.PortfolioSummary, error) {
	if err, ok := f.errs[userID]; ok {
		return nil, err
	}
	s, ok := f.byUser[userID]
	if !ok {
		return nil, errors.New("no summary")
	}
	return s, nil
}

func user(id, name string) auth.User {
	return auth.User{ID: id, Email: name + "@example.com", DisplayName: name, AvatarKey: "fox", PasswordHash: "secret-hash"}
}

func summary(pct, index float64) *portfolio.PortfolioSummary {
	return &portfolio.PortfolioSummary{GainLossPercentage: pct, PortfolioIndex: index}
}

// --- tests -------------------------------------------------------------------

func TestBuild_RanksByGainLossDescending(t *testing.T) {
	users := fakeUsers{users: []auth.User{user("u1", "Alpha"), user("u2", "Beta"), user("u3", "Gamma")}}
	sums := fakeSummaries{byUser: map[string]*portfolio.PortfolioSummary{
		"u1": summary(8.1, 108.1),
		"u2": summary(12.4, 112.4),
		"u3": summary(-3.0, 97.0),
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
	sums := fakeSummaries{byUser: map[string]*portfolio.PortfolioSummary{
		"u1": summary(8.1, 108.1),
		"u2": summary(12.4, 112.4),
		"u3": summary(5.0, 105.0),
	}}
	svc := NewService(users, sums)

	board, err := svc.Build(context.Background())
	require.NoError(t, err)

	for i, e := range board {
		assert.Equal(t, i+1, e.Rank)
	}
}

func TestBuild_EmptyUserListReturnsEmptyBoard(t *testing.T) {
	svc := NewService(fakeUsers{users: nil}, fakeSummaries{})

	board, err := svc.Build(context.Background())
	require.NoError(t, err)
	assert.Empty(t, board)
}

func TestBuild_EmptyPortfolioIsZeroAndHundred(t *testing.T) {
	users := fakeUsers{users: []auth.User{user("u1", "Alpha")}}
	sums := fakeSummaries{byUser: map[string]*portfolio.PortfolioSummary{
		"u1": summary(0, 100), // empty portfolio convention
	}}
	svc := NewService(users, sums)

	board, err := svc.Build(context.Background())
	require.NoError(t, err)
	require.Len(t, board, 1)
	assert.Equal(t, 0.0, board[0].GainLossPercentage)
	assert.Equal(t, 100.0, board[0].PortfolioIndex)
}

func TestBuild_TieBrokenByDisplayNameAscending(t *testing.T) {
	// Insertion order intentionally non-alphabetical to prove sorting is applied.
	users := fakeUsers{users: []auth.User{user("u3", "CryptoTiger"), user("u1", "AlphaBull"), user("u2", "BetaWolf")}}
	sums := fakeSummaries{byUser: map[string]*portfolio.PortfolioSummary{
		"u1": summary(10, 110),
		"u2": summary(10, 110),
		"u3": summary(8, 108),
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
	sums := fakeSummaries{
		byUser: map[string]*portfolio.PortfolioSummary{
			"u1": summary(8.1, 108.1),
			"u3": summary(5.0, 105.0),
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
	sums := fakeSummaries{
		byUser: map[string]*portfolio.PortfolioSummary{
			"u1": summary(8.1, 108.1),
			"u3": summary(5.0, 105.0),
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
	sums := fakeSummaries{byUser: map[string]*portfolio.PortfolioSummary{
		"u1": summary(1.0, 101), "u2": summary(2.0, 102),
	}}
	svc := NewService(cacheUsers(), sums)
	cache := newTestCache(t)
	svc.SetCache(cache)

	ctx := context.Background()
	require.NoError(t, cache.UpsertGlobalScore(ctx, "u1", 50.0))
	require.NoError(t, cache.UpsertGlobalScore(ctx, "u2", 25.0))

	board, err := svc.Build(ctx)
	require.NoError(t, err)
	require.Len(t, board, 2)
	// Cached scores (50 > 25) win, not the live summaries (2 > 1).
	assert.Equal(t, "Alpha", board[0].DisplayName)
	assert.Equal(t, 50.0, board[0].GainLossPercentage)
	assert.Equal(t, 150.0, board[0].PortfolioIndex, "index derives from cached score")
	assert.Equal(t, 1, board[0].Rank)
}

func TestBuild_FallsBackToLiveWhenCacheEmpty(t *testing.T) {
	sums := fakeSummaries{byUser: map[string]*portfolio.PortfolioSummary{
		"u1": summary(8.1, 108.1), "u2": summary(12.4, 112.4),
	}}
	svc := NewService(cacheUsers(), sums)
	svc.SetCache(newTestCache(t)) // attached but empty

	board, err := svc.Build(context.Background())
	require.NoError(t, err)
	require.Len(t, board, 2)
	assert.Equal(t, "Beta", board[0].DisplayName, "live calculation must be used when cache is empty")
	assert.InDelta(t, 12.4, board[0].GainLossPercentage, 0.001)
}

func TestRefreshCache_PopulatesScores(t *testing.T) {
	sums := fakeSummaries{byUser: map[string]*portfolio.PortfolioSummary{
		"u1": summary(8.1, 108.1), "u2": summary(12.4, 112.4),
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

func TestBuild_ListUsersErrorIsReturned(t *testing.T) {
	svc := NewService(fakeUsers{err: errors.New("db down")}, fakeSummaries{})

	_, err := svc.Build(context.Background())
	assert.Error(t, err)
}

// --- privacy (service level) -------------------------------------------------

func TestBuild_ResponseOmitsForbiddenFields(t *testing.T) {
	users := fakeUsers{users: []auth.User{user("u1", "Alpha")}}
	sums := fakeSummaries{byUser: map[string]*portfolio.PortfolioSummary{"u1": summary(12.4, 112.4)}}
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
