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
	"github.com/ardakimyonok/finance_app/internal/portfolio"
)

type stubUsers struct{ users []auth.User }

func (s stubUsers) ListUsers(_ context.Context) ([]auth.User, error) { return s.users, nil }

type stubSummaries struct{ byUser map[string]*portfolio.PortfolioSummary }

func (s stubSummaries) Summary(_ context.Context, userID string) (*portfolio.PortfolioSummary, error) {
	return s.byUser[userID], nil
}

func newEnv() (http.Handler, *auth.TokenManager) {
	tm := auth.NewTokenManager("test-secret", time.Hour)
	users := stubUsers{users: []auth.User{
		{ID: "u1", Email: "alpha@example.com", DisplayName: "AlphaWolf_91", AvatarKey: "fox", PasswordHash: "hash1"},
		{ID: "u2", Email: "bull@example.com", DisplayName: "SilentBull_77", AvatarKey: "bull", PasswordHash: "hash2"},
	}}
	sums := stubSummaries{byUser: map[string]*portfolio.PortfolioSummary{
		"u1": {GainLossPercentage: 12.4, PortfolioIndex: 112.4},
		"u2": {GainLossPercentage: 8.1, PortfolioIndex: 108.1},
	}}
	svc := leaderboard.NewService(users, sums)
	h := leaderboard.NewHandler(svc)

	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuth(tm))
		r.Get("/leaderboard", h.GetLeaderboard)
	})
	return r, tm
}

func get(t *testing.T, router http.Handler, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/leaderboard", nil)
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
