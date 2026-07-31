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
	"github.com/ardakimyonok/finance_app/internal/money"
	"github.com/ardakimyonok/finance_app/internal/performance"
	"github.com/ardakimyonok/finance_app/internal/performancehistory"
)

// stubPerformance yields a two-point index series for a configured return.
type stubPerformance struct {
	returnPct map[string]float64
}

func (s stubPerformance) Window(_ context.Context, userID string, start, end time.Time) (performancehistory.Window, error) {
	ret := s.returnPct[userID]
	epoch := start.Add(-24 * time.Hour)
	mk := func(id string, at time.Time, idx float64) performancehistory.Snapshot {
		return performancehistory.Snapshot{
			ID: id, UserID: userID, PortfolioID: "pf1", CapturedAt: at,
			RankedIndex: money.IndexValueFromFloat64(idx), RankingStatus: performance.StatusActive,
			TrackingStartedAt: epoch, DataQualityStatus: "complete",
		}
	}
	startSnap := mk("s-start", start, 100)
	endSnap := mk("s-end", end, 100*(1+ret/100))
	return performancehistory.Window{
		StartSnapshot: startSnap, EndSnapshot: endSnap,
		Points:          []performancehistory.Snapshot{startSnap, endSnap},
		HistoryCoverage: 1, ActiveCoverage: 1, TrustedCoverage: 1,
	}, nil
}

func (stubPerformance) ProtectEvidence(_ context.Context, _ ...string) error { return nil }
func (stubPerformance) EligibilityThreshold() float64                        { return 0.9 }
func (stubPerformance) SnapshotFrequency() string                            { return "24h0m0s" }

// verifiedHandlerPrices labels the deterministic mock path as verified,
// adjusted/total-return data so the handler tests can exercise genuine verified
// awards end-to-end.
type verifiedHandlerPrices struct {
	inner *benchmark.MockHistoricalPriceProvider
}

func (v verifiedHandlerPrices) GetAdjustedCloseSeries(ctx context.Context, symbol string, start, end time.Time) ([]benchmark.PricePoint, error) {
	return v.inner.GetAdjustedCloseSeries(ctx, symbol, start, end)
}

func (v verifiedHandlerPrices) GetSeries(ctx context.Context, symbol string, start, end time.Time, _ benchmark.SeriesRequirement) (benchmark.BenchmarkPriceSeries, error) {
	pts, err := v.inner.GetAdjustedCloseSeries(ctx, symbol, start, end)
	if err != nil {
		return benchmark.BenchmarkPriceSeries{}, err
	}
	now := time.Now().UTC()
	return benchmark.BenchmarkPriceSeries{
		Symbol: symbol, Points: pts,
		Metadata: benchmark.BenchmarkDataMetadata{
			Provider: "test_feed", ProviderMode: "real",
			PriceType:         benchmark.PriceTypeAdjustedClose,
			IncludesDividends: true, IncludesSplits: true,
			IsAdjusted: true, IsTotalReturn: true, CorpActionsKnown: true,
			Quality:     benchmark.DataQualityVerified,
			RetrievedAt: now, SourceAsOf: now,
			Currency: "USD",
		},
	}, nil
}

func newEnv(t *testing.T) (http.Handler, *auth.TokenManager, *achievements.Service) {
	t.Helper()
	tm := auth.NewTokenManager("test-secret", time.Hour)
	engine := benchmark.NewBenchmarkConstructionService(
		verifiedHandlerPrices{benchmark.NewMockHistoricalPriceProvider(benchmark.DefaultMockReturns())},
		benchmark.Recipes,
		nil,
	)
	rules := benchmark.NewRulesEngine(benchmark.DefaultEvaluators())
	perf := stubPerformance{returnPct: map[string]float64{"u1": 25}}
	svc := achievements.NewService(achievements.NewInMemoryAchievementRepository(), perf, engine, rules)
	svc.SetAwardPolicy(benchmark.AwardModeVerifiedOnly, benchmark.EnvironmentProduction)
	h := achievements.NewHandler(svc)

	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuth(tm))
		r.Get("/achievements", h.ListAchievements)
		r.Get("/achievements/returns", h.ListReturns)
		r.Post("/achievements/evaluate", h.Evaluate)
	})
	return r, tm, svc
}

func getPath(t *testing.T, router http.Handler, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
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

func TestAchievementReturns_UsesSelectedTimeframeAndPrivacySafePercentages(t *testing.T) {
	r, tm, _ := newEnv(t)
	tok, _ := tm.Generate("u1", "u1@e.com")

	rec := getPath(t, r, "/achievements/returns?timeframe=3M", tok)
	require.Equal(t, http.StatusOK, rec.Code)

	var result achievements.AchievementReturnsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	assert.Equal(t, "3M", result.Timeframe)
	require.Len(t, result.Rows, len(benchmark.Badges))
	assert.True(t, result.Rows[0].Available)
	assert.NotNil(t, result.Rows[0].PortfolioReturnPercentage)
	assert.NotNil(t, result.Rows[0].BenchmarkReturnPercentage)
	assert.NotNil(t, result.Rows[0].EdgePoints)

	body := rec.Body.String()
	for _, key := range []string{
		"user_id", "portfolio_id", "position_id", "current_value", "quantity",
		"average_buy_price", "starting_value", "symbol",
	} {
		assert.NotContainsf(t, body, `"`+key+`":`, "must not expose %q", key)
	}
}

func TestAchievementReturns_RequiresAuthentication(t *testing.T) {
	r, _, _ := newEnv(t)
	assert.Equal(t, http.StatusUnauthorized, getPath(t, r, "/achievements/returns?timeframe=1M", "").Code)
}
