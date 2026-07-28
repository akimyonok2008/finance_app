package leaderboard_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/leaderboard"
	"github.com/ardakimyonok/finance_app/internal/money"
)

type stubUsers struct{ users []auth.User }

func (s stubUsers) ListUsers(_ context.Context) ([]auth.User, error) { return s.users, nil }

type stubSummaries struct {
	byUser map[string]leaderboard.RankedPerformance
}

func (s stubSummaries) CurrentRankedPerformance(_ context.Context, userID string) (leaderboard.RankedPerformance, error) {
	return s.byUser[userID], nil
}

func newEnv() (http.Handler, *auth.TokenManager) {
	tm := auth.NewTokenManager("test-secret", time.Hour)
	users := stubUsers{users: []auth.User{
		{ID: "u1", Email: "alpha@example.com", DisplayName: "AlphaWolf_91", AvatarKey: "fox", PasswordHash: "hash1"},
		{ID: "u2", Email: "bull@example.com", DisplayName: "SilentBull_77", AvatarKey: "bull", PasswordHash: "hash2"},
	}}
	sums := stubSummaries{byUser: map[string]leaderboard.RankedPerformance{
		"u1": {RankedReturnPercentage: money.MustRatio("12.4"), RankedIndex: money.MustIndexValue("112.4")},
		"u2": {RankedReturnPercentage: money.MustRatio("8.1"), RankedIndex: money.MustIndexValue("108.1")},
	}}
	svc := leaderboard.NewService(users, sums)
	h := leaderboard.NewHandler(svc)

	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuth(tm))
		r.Get("/leaderboard", h.GetLeaderboard)
		r.Get("/leaderboard/me", h.GetMyStanding)
	})
	return r, tm
}

func get(t *testing.T, router http.Handler, token string) *httptest.ResponseRecorder {
	return getPath(t, router, "/leaderboard", token)
}

func getPath(t *testing.T, router http.Handler, path string, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestLeaderboard_RequiresAuth(t *testing.T) {
	router, _ := newEnv()
	rec := get(t, router, "")
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestLeaderboard_InvalidTokenIs401(t *testing.T) {
	router, _ := newEnv()
	rec := get(t, router, "not.a.real.token")
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestLeaderboard_ReturnsRankedEntriesWithValidToken(t *testing.T) {
	router, tm := newEnv()
	token, err := tm.Generate("u1", "alpha@example.com")
	require.NoError(t, err)

	rec := get(t, router, token)
	assert.Equal(t, http.StatusOK, rec.Code)

	var board []leaderboard.LeaderboardEntry
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &board))
	require.Len(t, board, 2)

	assert.Equal(t, 1, board[0].Rank)
	assert.Equal(t, "AlphaWolf_91", board[0].DisplayName)
	assert.Equal(t, "fox", board[0].AvatarKey)
	assert.InDelta(t, 12.4, board[0].GainLossPercentage, 0.001)

	assert.Equal(t, 2, board[1].Rank)
	assert.Equal(t, "SilentBull_77", board[1].DisplayName)
}

func TestLeaderboard_ResponseHidesForbiddenFields(t *testing.T) {
	router, tm := newEnv()
	token, err := tm.Generate("u1", "alpha@example.com")
	require.NoError(t, err)

	rec := get(t, router, token)
	require.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	forbidden := []string{
		"total_cost_basis", "current_value", "gain_loss", "positions", "symbol",
		"quantity", "average_buy_price", "email", "password", "password_hash",
		"portfolio_id", "user_id",
	}
	for _, k := range forbidden {
		// Quoted-key form so "gain_loss" does not match "gain_loss_percentage".
		assert.NotContainsf(t, body, `"`+k+`":`, "response must not expose %q", k)
	}
	// The hashed secret value itself must never appear.
	assert.NotContains(t, body, "hash1")
	assert.NotContains(t, body, "alpha@example.com")
}

func TestLeaderboardMe_ReturnsEnhancedStanding(t *testing.T) {
	router, tm := newEnv()
	token, err := tm.Generate("u2", "bull@example.com")
	require.NoError(t, err)

	rec := getPath(t, router, "/leaderboard/me?timeframe=ALL", token)
	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Timeframe              leaderboard.Timeframe `json:"timeframe"`
		Eligible               bool                  `json:"eligible"`
		Rank                   *int                  `json:"rank"`
		BestRank               *int                  `json:"best_rank"`
		ParticipantCount       int                   `json:"participant_count"`
		TotalParticipants      int                   `json:"total_participants"`
		Percentile             float64               `json:"percentile"`
		RankedReturnPercentage float64               `json:"ranked_return_percentage"`
		RankedIndex            float64               `json:"ranked_index"`
		Reason                 string                `json:"reason"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.NotNil(t, body.Rank)
	require.NotNil(t, body.BestRank)
	assert.Equal(t, leaderboard.TimeframeAll, body.Timeframe)
	assert.True(t, body.Eligible)
	assert.Equal(t, 2, *body.Rank)
	assert.Equal(t, 2, *body.BestRank)
	assert.Equal(t, 2, body.ParticipantCount)
	assert.Equal(t, 2, body.TotalParticipants)
	assert.InDelta(t, 50.0, body.Percentile, 0.01)
	assert.InDelta(t, 8.1, body.RankedReturnPercentage, 0.001)
	assert.InDelta(t, 108.1, body.RankedIndex, 0.001)
	assert.Empty(t, body.Reason)
}
