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
	"github.com/ardakimyonok/finance_app/internal/benchmark"
)

// stubPerformance yields a two-point index series for a configured return.
type stubPerformance struct {
	returnPct map[string]float64
}

func (s stubPerformance) GetPortfolioIndexSeries(_ context.Context, userID string, start, end time.Time) ([]benchmark.IndexPoint, error) {
	ret := s.returnPct[userID]
	return []benchmark.IndexPoint{
		{Date: start.UTC().Format("2006-01-02"), Index: 100},
		{Date: end.UTC().Format("2006-01-02"), Index: 100 * (1 + ret/100)},
	}, nil
}

func newEnv(t *testing.T) (http.Handler, *auth.TokenManager, *achievements.Service) {
	t.Helper()
	tm := auth.NewTokenManager("test-secret", time.Hour)
	engine := benchmark.NewBenchmarkConstructionService(
		benchmark.NewMockHistoricalPriceProvider(benchmark.DefaultMockReturns()),
		benchmark.Recipes,
		benchmark.NewSnapshotRecipeResolver(benchmark.DefaultRecipeSnapshots()),
	)
	rules := benchmark.NewRulesEngine(benchmark.DefaultEvaluators())
	perf := stubPerformance{returnPct: map[string]float64{"u1": 25}}
	svc := achievements.NewService(achievements.NewInMemoryAchievementRepository(), perf, engine, rules)
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

func TestEvaluate_RequiresAuth(t *testing.T) {
	r, _, _ := newEnv(t)
	assert.Equal(t, http.StatusUnauthorized, post(t, r, "/achievements/evaluate", "").Code)
}

func TestEvaluate_UnlocksAndReturnsList(t *testing.T) {
	r, tm, _ := newEnv(t)
	tok, _ := tm.Generate("u1", "u1@e.com")
	rec := post(t, r, "/achievements/evaluate", tok)
	assert.Equal(t, http.StatusOK, rec.Code)

	var list []achievements.AchievementResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	assert.Len(t, list, len(benchmark.Badges))

	byKey := map[string]bool{}
	for _, a := range list {
		byKey[a.Key] = a.Unlocked
	}
	// A +25% portfolio beats the easy benchmarks.
	assert.True(t, byKey["cash_plus_30d"])
	assert.True(t, byKey["first_market_edge_30d"])
}

func TestAchievements_RequiresAuth(t *testing.T) {
	r, _, _ := newEnv(t)
	assert.Equal(t, http.StatusUnauthorized, get(t, r, "").Code)
}

func TestAchievements_ReturnsFullCatalogue(t *testing.T) {
	r, tm, _ := newEnv(t)
	tok, _ := tm.Generate("u1", "u1@e.com")
	rec := get(t, r, tok)
	assert.Equal(t, http.StatusOK, rec.Code)
	var list []achievements.AchievementResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	assert.Len(t, list, len(benchmark.Badges))
}

func TestAchievements_ResponseHasNoForbiddenFields(t *testing.T) {
	r, tm, _ := newEnv(t)
	tok, _ := tm.Generate("u1", "u1@e.com")
	// Unlock some badges so evidence is present in the response too.
	_ = post(t, r, "/achievements/evaluate", tok)
	body := get(t, r, tok).Body.String()
	for _, k := range []string{
		"email", "password", "password_hash", "portfolio_id", "position_id",
		"total_cost_basis", "current_value", "gain_loss", "positions", "symbol",
		"quantity", "average_buy_price", "starting_value", "user_id",
	} {
		assert.NotContainsf(t, body, `"`+k+`":`, "must not expose %q", k)
	}
}
