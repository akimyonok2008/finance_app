package competitions

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/clock"
	"github.com/ardakimyonok/finance_app/internal/fx"
	"github.com/ardakimyonok/finance_app/internal/leaderboard"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
	"github.com/ardakimyonok/finance_app/internal/prices"
)

// fixedTime is in ISO week 24 of 2026 (Wed 2026-06-10).
var fixedTime = time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)

// newSprintTestCache spins up a miniredis-backed leaderboard cache.
func newSprintTestCache(t *testing.T) *leaderboard.RedisLeaderboardCache {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return leaderboard.NewRedisLeaderboardCache(client)
}

// --- test doubles ------------------------------------------------------------

type fakeUsers struct{ m map[string]*auth.User }

func (f fakeUsers) GetUserByID(_ context.Context, id string) (*auth.User, error) {
	u, ok := f.m[id]
	if !ok {
		return nil, assert.AnError
	}
	return u, nil
}

type fakePositions struct{ m map[string][]portfolio.Position }

func (f fakePositions) ListPositions(_ context.Context, userID string) ([]portfolio.Position, error) {
	return f.m[userID], nil
}

func pos(symbol string, qty, avg float64, cur string) portfolio.Position {
	return portfolio.Position{Symbol: symbol, AssetType: "stock", Quantity: qty, AverageBuyPrice: avg, Currency: cur}
}

type harness struct {
	svc   *Service
	repo  *InMemoryCompetitionRepository
	mp    *prices.MockPriceProvider
	clk   *clock.FixedClock
	posns *fakePositions
}

func newHarness(users map[string]*auth.User, positions map[string][]portfolio.Position) *harness {
	repo := NewInMemoryCompetitionRepository()
	mp := prices.NewMockPriceProvider()
	clk := &clock.FixedClock{Time: fixedTime}
	fp := &fakePositions{m: positions}
	svc := NewService(repo, fakeUsers{m: users}, fp, mp, fx.NewMockFXProvider(), clk)
	return &harness{svc: svc, repo: repo, mp: mp, clk: clk, posns: fp}
}

func compID() string { return WeeklySprintID(fixedTime) }

// --- dynamic sprint generation (Problem 4) -----------------------------------

func TestWeeklySprint_ISOWeekIDAndBounds(t *testing.T) {
	c := WeeklySprint(fixedTime)
	assert.Equal(t, "weekly_2026_24", c.ID)
	assert.Equal(t, time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC), c.StartsAt)  // Monday
	assert.Equal(t, time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC), c.EndsAt)   // next Monday
	assert.Equal(t, StatusActive, c.Status)
}

func TestWeeklySprint_StatusTransitions(t *testing.T) {
	c := WeeklySprint(fixedTime)
	assert.Equal(t, StatusUpcoming, deriveStatus(c.StartsAt, c.EndsAt, c.StartsAt.Add(-time.Hour)))
	assert.Equal(t, StatusActive, deriveStatus(c.StartsAt, c.EndsAt, c.StartsAt.Add(time.Hour)))
	assert.Equal(t, StatusCompleted, deriveStatus(c.StartsAt, c.EndsAt, c.EndsAt.Add(time.Hour)))
}

func TestListCompetitions_EnsuresCurrentActiveSprint(t *testing.T) {
	h := newHarness(nil, nil)
	comps, err := h.svc.ListCompetitions(context.Background())
	require.NoError(t, err)
	require.Len(t, comps, 1)
	assert.Equal(t, "weekly_2026_24", comps[0].ID)
	assert.Equal(t, StatusActive, comps[0].Status)
}

// --- join + snapshot (Problems 3) --------------------------------------------

func TestJoin_StoresSnapshot(t *testing.T) {
	h := newHarness(
		map[string]*auth.User{"u1": {ID: "u1", DisplayName: "Alpha", AvatarKey: "fox"}},
		map[string][]portfolio.Position{"u1": {pos("AAPL", 10, 180, "USD")}},
	)
	resp, err := h.svc.JoinCompetition(context.Background(), compID(), "u1")
	require.NoError(t, err)
	assert.True(t, resp.Joined)
	assert.Equal(t, 100.0, resp.StartingIndex)

	entry, err := h.repo.GetEntry(context.Background(), compID(), "u1")
	require.NoError(t, err)
	require.Len(t, entry.Snapshots, 1)
	snap := entry.Snapshots[0]
	assert.Equal(t, "AAPL", snap.Symbol)
	assert.Equal(t, 10.0, snap.Quantity)
	assert.Equal(t, 195.0, snap.StartingPrice)         // mock AAPL price
	assert.Equal(t, 1950.0, snap.StartingValueBase)    // 10 * 195 USD
	assert.Equal(t, 1950.0, entry.StartingValue)
}

func TestJoin_EmptyPortfolioRejected(t *testing.T) {
	h := newHarness(map[string]*auth.User{"u1": {ID: "u1"}}, map[string][]portfolio.Position{"u1": {}})
	_, err := h.svc.JoinCompetition(context.Background(), compID(), "u1")
	assert.ErrorIs(t, err, ErrEmptyPortfolio)
}

func TestJoin_UnpriceablePositionRejected(t *testing.T) {
	h := newHarness(
		map[string]*auth.User{"u1": {ID: "u1"}},
		map[string][]portfolio.Position{"u1": {pos("ZZZZ", 1, 10, "USD")}},
	)
	_, err := h.svc.JoinCompetition(context.Background(), compID(), "u1")
	assert.ErrorIs(t, err, ErrJoinSnapshot)
}

func TestJoin_NonExistingCompetition(t *testing.T) {
	h := newHarness(map[string]*auth.User{"u1": {ID: "u1"}}, map[string][]portfolio.Position{"u1": {pos("AAPL", 1, 100, "USD")}})
	_, err := h.svc.JoinCompetition(context.Background(), "weekly_1999_01", "u1")
	assert.ErrorIs(t, err, ErrCompetitionNotFound)
}

func TestJoin_NotActiveCompetition(t *testing.T) {
	h := newHarness(map[string]*auth.User{"u1": {ID: "u1"}}, map[string][]portfolio.Position{"u1": {pos("AAPL", 1, 100, "USD")}})
	// Ensure the week-24 sprint exists, then advance the clock past its end.
	_, err := h.svc.ListCompetitions(context.Background())
	require.NoError(t, err)
	h.clk.Time = time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC) // week 25
	_, err = h.svc.JoinCompetition(context.Background(), "weekly_2026_24", "u1")
	assert.ErrorIs(t, err, ErrCompetitionNotActive)
}

func TestJoin_Idempotent(t *testing.T) {
	h := newHarness(
		map[string]*auth.User{"u1": {ID: "u1"}},
		map[string][]portfolio.Position{"u1": {pos("AAPL", 10, 180, "USD")}},
	)
	_, err := h.svc.JoinCompetition(context.Background(), compID(), "u1")
	require.NoError(t, err)
	_, err = h.svc.JoinCompetition(context.Background(), compID(), "u1")
	require.NoError(t, err)
	entries, err := h.repo.ListEntries(context.Background(), compID())
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

// Sprint must use the snapshot, not the live portfolio.
func TestSprint_UsesSnapshotNotLivePortfolio(t *testing.T) {
	h := newHarness(
		map[string]*auth.User{"u1": {ID: "u1", DisplayName: "Alpha", AvatarKey: "fox"}},
		map[string][]portfolio.Position{"u1": {pos("AAPL", 10, 180, "USD")}},
	)
	ctx := context.Background()
	_, err := h.svc.JoinCompetition(ctx, compID(), "u1")
	require.NoError(t, err)

	// User edits live portfolio AFTER joining: add a big BTC position and delete AAPL.
	h.posns.m["u1"] = []portfolio.Position{pos("BTC-USD", 100, 1, "USD")}

	// Sprint return must still be based on the AAPL snapshot (price unchanged → 0%).
	st, err := h.svc.MyStatus(ctx, compID(), "u1")
	require.NoError(t, err)
	assert.InDelta(t, 0.0, st.SprintReturnPercentage, 0.001)
	assert.InDelta(t, 100.0, st.SprintIndex, 0.001)

	// Now the AAPL market price moves up; sprint reflects only the snapshot.
	h.mp.Set("AAPL", 214.5, "USD") // +10% vs 195
	st, err = h.svc.MyStatus(ctx, compID(), "u1")
	require.NoError(t, err)
	assert.InDelta(t, 10.0, st.SprintReturnPercentage, 0.01)
	assert.InDelta(t, 110.0, st.SprintIndex, 0.01)
}

func TestLeaderboard_RanksAndExcludesNonJoiners(t *testing.T) {
	h := newHarness(
		map[string]*auth.User{
			"u1": {ID: "u1", DisplayName: "Alpha", AvatarKey: "fox"},
			"u2": {ID: "u2", DisplayName: "Beta", AvatarKey: "bull"},
			"u3": {ID: "u3", DisplayName: "Gamma", AvatarKey: "bear"},
		},
		map[string][]portfolio.Position{
			"u1": {pos("AAPL", 10, 180, "USD")},
			"u2": {pos("AAPL", 10, 180, "USD")},
			// u3 never joins
		},
	)
	ctx := context.Background()
	_, _ = h.svc.JoinCompetition(ctx, compID(), "u1")
	_, _ = h.svc.JoinCompetition(ctx, compID(), "u2")

	// After join, repricing AAPL affects both equally → tie broken by name.
	board, err := h.svc.Leaderboard(ctx, compID())
	require.NoError(t, err)
	require.Len(t, board, 2) // u3 excluded
	assert.Equal(t, "Alpha", board[0].DisplayName)
	assert.Equal(t, 1, board[0].Rank)
	assert.Equal(t, "Beta", board[1].DisplayName)
}

func TestMyStatus_NotJoined(t *testing.T) {
	h := newHarness(map[string]*auth.User{"u1": {ID: "u1"}}, nil)
	st, err := h.svc.MyStatus(context.Background(), compID(), "u1")
	require.NoError(t, err)
	assert.False(t, st.Joined)
	assert.Equal(t, 0, st.CurrentRank)
	assert.Equal(t, 100.0, st.SprintIndex)
}

// --- cache + jobs support (Phase 3) --------------------------------------------

func TestEnsureCurrentSprint_CreatesAndLists(t *testing.T) {
	h := newHarness(nil, nil)
	require.NoError(t, h.svc.EnsureCurrentSprint(context.Background()))
	ids, err := h.svc.ListActiveCompetitionIDs(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"weekly_2026_24"}, ids)
}

func TestRefreshCache_PopulatesSprintScores(t *testing.T) {
	h := newHarness(
		map[string]*auth.User{"u1": {ID: "u1", DisplayName: "Alpha", AvatarKey: "fox"}},
		map[string][]portfolio.Position{"u1": {pos("AAPL", 10, 180, "USD")}},
	)
	cache := newSprintTestCache(t)
	h.svc.SetCache(cache)
	ctx := context.Background()
	_, err := h.svc.JoinCompetition(ctx, compID(), "u1")
	require.NoError(t, err)

	h.mp.Set("AAPL", 214.5, "USD") // +10% vs snapshot price 195
	skipped, err := h.svc.RefreshCache(ctx, compID())
	require.NoError(t, err)
	assert.Equal(t, 0, skipped)

	top, err := cache.GetCompetitionTop(ctx, compID(), 10)
	require.NoError(t, err)
	require.Len(t, top, 1)
	assert.Equal(t, "u1", top[0].UserID)
	assert.InDelta(t, 10.0, top[0].Score, 0.01)
}

func TestLeaderboard_ServesFromCacheWhenPopulated(t *testing.T) {
	h := newHarness(
		map[string]*auth.User{"u1": {ID: "u1", DisplayName: "Alpha", AvatarKey: "fox"}},
		map[string][]portfolio.Position{"u1": {pos("AAPL", 10, 180, "USD")}},
	)
	cache := newSprintTestCache(t)
	h.svc.SetCache(cache)
	ctx := context.Background()
	_, err := h.svc.JoinCompetition(ctx, compID(), "u1")
	require.NoError(t, err)

	// Seed the cache with a score that differs from live (live would be 0%).
	require.NoError(t, cache.UpsertCompetitionScore(ctx, compID(), "u1", 7.5))

	board, err := h.svc.Leaderboard(ctx, compID())
	require.NoError(t, err)
	require.Len(t, board, 1)
	assert.Equal(t, 7.5, board[0].SprintReturnPercentage, "cached score must be served")
	assert.Equal(t, 107.5, board[0].SprintIndex)
}

func TestLeaderboard_FallsBackToLiveWhenCacheEmpty(t *testing.T) {
	h := newHarness(
		map[string]*auth.User{"u1": {ID: "u1", DisplayName: "Alpha", AvatarKey: "fox"}},
		map[string][]portfolio.Position{"u1": {pos("AAPL", 10, 180, "USD")}},
	)
	h.svc.SetCache(newSprintTestCache(t)) // attached but empty
	ctx := context.Background()
	_, err := h.svc.JoinCompetition(ctx, compID(), "u1")
	require.NoError(t, err)

	board, err := h.svc.Leaderboard(ctx, compID())
	require.NoError(t, err)
	require.Len(t, board, 1, "empty cache must fall back to live snapshot calculation")
	assert.InDelta(t, 0.0, board[0].SprintReturnPercentage, 0.001)
}

// --- privacy (Problem 3) -----------------------------------------------------

func TestSprintResponses_NeverExposeSnapshotDetails(t *testing.T) {
	h := newHarness(
		map[string]*auth.User{"u1": {ID: "u1", Email: "a@e.com", DisplayName: "Alpha", AvatarKey: "fox", PasswordHash: "h"}},
		map[string][]portfolio.Position{"u1": {pos("THYAO.IS", 100, 250, "TRY")}},
	)
	ctx := context.Background()
	_, err := h.svc.JoinCompetition(ctx, compID(), "u1")
	require.NoError(t, err)

	board, err := h.svc.Leaderboard(ctx, compID())
	require.NoError(t, err)
	st, err := h.svc.MyStatus(ctx, compID(), "u1")
	require.NoError(t, err)

	for _, payload := range []any{board, st} {
		raw, err := json.Marshal(payload)
		require.NoError(t, err)
		body := string(raw)
		for _, k := range []string{
			"symbol", "quantity", "starting_price", "starting_value", "starting_value_base",
			"snapshot", "user_id", "email", "portfolio_id", "current_value", "average_buy_price", "password",
		} {
			assert.NotContainsf(t, body, `"`+k+`":`, "sprint response must not expose %q", k)
		}
		assert.NotContains(t, body, "THYAO.IS")
		assert.NotContains(t, body, "a@e.com")
	}
}
