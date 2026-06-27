package achievements_test

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

	"github.com/ardakimyonok/finance_app/internal/achievements"
	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
)

type stubPositions struct {
	m map[string][]portfolio.Position
}

func (s stubPositions) ListPositions(_ context.Context, userID string) ([]portfolio.Position, error) {
	return s.m[userID], nil
}

type stubSummaries struct {
	m map[string]*portfolio.PortfolioSummary
}

func (s stubSummaries) GetSummary(_ context.Context, userID string) (*portfolio.PortfolioSummary, error) {
	return s.m[userID], nil
}

type stubRanks struct{}

func (stubRanks) GetUserRank(_ context.Context, _, _ string) (int, error) { return 0, nil }

func newEnv(t *testing.T) (http.Handler, *auth.TokenManager, *achievements.Service) {
	t.Helper()
	tm := auth.NewTokenManager("test-secret", time.Hour)
	pos := stubPositions{m: map[string][]portfolio.Position{"u1": {{ID: "p1"}}}}
	sums := stubSummaries{m: map[string]*portfolio.PortfolioSummary{"u1": {GainLossPercentage: 12, PortfolioIndex: 112}}}
	svc := achievements.NewService(achievements.NewInMemoryAchievementRepository(), pos, sums, stubRanks{})
	h := achievements.NewHandler(svc)

	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuth(tm))
		r.Get("/achievements", h.ListAchievements)
		r.Post("/achievements/evaluate", h.Evaluate)
	})
	return r, tm, svc
}

func post(t *testing.T, router http.Handler, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestEvaluate_RequiresAuth(t *testing.T) {
	r, _, _ := newEnv(t)
	assert.Equal(t, http.StatusUnauthorized, post(t, r, "/achievements/evaluate", "").Code)
}

func TestEvaluate_UnlocksAndReturnsList(t *testing.T) {
	r, tm, _ := newEnv(t)
	tok, _ := tm.Generate("u1", "u1@e.com")
	// u1 has a position and +12% / index 112 portfolio in the stub.
	rec := post(t, r, "/achievements/evaluate", tok)
	assert.Equal(t, http.StatusOK, rec.Code)
	var list []achievements.AchievementResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	byKey := map[string]bool{}
	for _, a := range list {
		byKey[a.Key] = a.Unlocked
	}
	assert.True(t, byKey["first_portfolio"])
	assert.True(t, byKey["green_portfolio"])
	assert.True(t, byKey["index_110"])
}

func get(t *testing.T, router http.Handler, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/achievements", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestAchievements_RequiresAuth(t *testing.T) {
	r, _, _ := newEnv(t)
	assert.Equal(t, http.StatusUnauthorized, get(t, r, "").Code)
}

func TestAchievements_ReturnsList(t *testing.T) {
	r, tm, _ := newEnv(t)
	tok, _ := tm.Generate("u1", "u1@e.com")
	rec := get(t, r, tok)
	assert.Equal(t, http.StatusOK, rec.Code)
	var list []achievements.AchievementResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	assert.Len(t, list, 5)
}

func TestAchievements_ShowsUnlockedAndLocked(t *testing.T) {
	r, tm, svc := newEnv(t)
	// u1 has a position and a +12% / index 112 portfolio: first_portfolio,
	// green_portfolio, and index_110 should unlock; first_sprint stays locked.
	require.NoError(t, svc.EvaluatePortfolioAchievements(context.Background(), "u1"))

	tok, _ := tm.Generate("u1", "u1@e.com")
	rec := get(t, r, tok)
	var list []achievements.AchievementResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))

	byKey := map[string]achievements.AchievementResponse{}
	for _, a := range list {
		byKey[a.Key] = a
	}
	assert.True(t, byKey["first_portfolio"].Unlocked)
	assert.True(t, byKey["green_portfolio"].Unlocked)
	assert.True(t, byKey["index_110"].Unlocked)
	assert.False(t, byKey["first_sprint"].Unlocked)
	require.NotNil(t, byKey["first_portfolio"].UnlockedAt)
}

func TestAchievements_ResponseHasNoForbiddenFields(t *testing.T) {
	r, tm, _ := newEnv(t)
	tok, _ := tm.Generate("u1", "u1@e.com")
	body := get(t, r, tok).Body.String()
	for _, k := range []string{
		"email", "password", "password_hash", "portfolio_id", "position_id",
		"total_cost_basis", "current_value", "gain_loss", "positions", "symbol",
		"quantity", "average_buy_price", "starting_value", "user_id",
	} {
		assert.NotContainsf(t, body, `"`+k+`":`, "must not expose %q", k)
	}
}
