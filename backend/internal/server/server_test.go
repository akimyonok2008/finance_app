package server_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/achievements"
	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/benchmark"
	"github.com/ardakimyonok/finance_app/internal/clock"
	"github.com/ardakimyonok/finance_app/internal/competitions"
	"github.com/ardakimyonok/finance_app/internal/fx"
	"github.com/ardakimyonok/finance_app/internal/leaderboard"
	"github.com/ardakimyonok/finance_app/internal/performance"
	"github.com/ardakimyonok/finance_app/internal/performancehistory"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
	"github.com/ardakimyonok/finance_app/internal/prices"
	"github.com/ardakimyonok/finance_app/internal/server"
)

// --- adapters mirroring cmd/api/main.go ----------------------------------------

type userProvider struct{ s *auth.Service }

func (u userProvider) GetUserByID(_ context.Context, id string) (*auth.User, error) {
	return u.s.UserByID(id)
}

type summaryProvider struct{ s *portfolio.Service }

func (p summaryProvider) GetSummary(ctx context.Context, userID string) (*portfolio.PortfolioSummary, error) {
	return p.s.Summary(ctx, userID)
}

type rankedPerformanceAdapter struct{ s *performance.Service }

func (a rankedPerformanceAdapter) CurrentRankedPerformance(ctx context.Context, userID string) (leaderboard.RankedPerformance, error) {
	rp, err := a.s.CurrentRankedPerformance(ctx, userID)
	if err != nil {
		return leaderboard.RankedPerformance{}, err
	}
	return leaderboard.RankedPerformance{
		RankedIndex:            rp.RankedIndex,
		RankedReturnPercentage: rp.RankedReturnPercentage,
		Paused:                 rp.Status == performance.StatusPaused,
		TrackingStartedAt:      rp.TrackingStartedAt,
	}, nil
}

type positionProvider struct{ s *portfolio.Service }

func (p positionProvider) ListPositions(ctx context.Context, userID string) ([]portfolio.Position, error) {
	ptrs, err := p.s.ListPositions(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]portfolio.Position, 0, len(ptrs))
	for _, x := range ptrs {
		out = append(out, *x)
	}
	return out, nil
}

type rankProvider struct{ s *competitions.Service }

func (r rankProvider) GetUserRank(ctx context.Context, competitionID, userID string) (int, error) {
	return r.s.GetUserRank(ctx, competitionID, userID)
}

type achievementPerformanceProvider struct{ s *portfolio.Service }

func (a achievementPerformanceProvider) GetPortfolioIndexSeries(ctx context.Context, userID string, start, end time.Time) ([]benchmark.IndexPoint, error) {
	archives, err := a.s.Archives(ctx, userID, portfolio.ArchiveTimeframe1Y)
	if err != nil {
		return nil, err
	}
	out := make([]benchmark.IndexPoint, 0, len(archives.Points))
	for _, p := range archives.Points {
		captured, err := time.Parse(time.RFC3339, p.CapturedAt)
		if err != nil || captured.Before(start) || captured.After(end) {
			continue
		}
		out = append(out, benchmark.IndexPoint{Date: captured.UTC().Format("2006-01-02"), Index: p.PortfolioIndex})
	}
	return out, nil
}

// newFullServer builds the complete application with in-memory storage,
// exactly as main.go wires it (minus Postgres/Redis).
func newFullServer(t *testing.T, readiness []server.ReadinessCheck) http.Handler {
	t.Helper()
	tokens := auth.NewTokenManager("test-secret", time.Hour)
	authSvc := auth.NewService(auth.NewInMemoryUserRepository(), tokens)
	fxp := fx.NewMockFXProvider()
	priceProvider := prices.NewMockPriceProvider()
	portfolioRepo := portfolio.NewInMemoryRepository()
	portfolioSvc := portfolio.NewService(portfolioRepo, priceProvider, fxp)
	performanceSvc := performance.NewService(portfolioRepo)
	performanceSvc.SetValuator(portfolioSvc)
	historySvc := performancehistory.NewService(
		performancehistory.NewInMemoryRepository(), performanceSvc, performancehistory.Config{})
	leaderboardSvc := leaderboard.NewService(authSvc, rankedPerformanceAdapter{performanceSvc})
	competitionsSvc := competitions.NewService(
		competitions.NewInMemoryCompetitionRepository(),
		userProvider{authSvc}, positionProvider{portfolioSvc},
		priceProvider, fxp, clock.RealClock{},
	)
	benchmarkEngine := benchmark.NewBenchmarkConstructionService(
		benchmark.NewMockHistoricalPriceProvider(benchmark.DefaultMockReturns()),
		benchmark.Recipes,
		benchmark.NewSnapshotRecipeResolver(benchmark.DefaultRecipeSnapshots()),
	)
	achievementsSvc := achievements.NewService(
		achievements.NewInMemoryAchievementRepository(),
		historySvc,
		benchmarkEngine,
		benchmark.NewRulesEngine(benchmark.DefaultEvaluators()),
	)

	return server.New(server.Deps{
		Auth:            authSvc,
		Tokens:          tokens,
		Portfolio:       portfolioSvc,
		Leaderboard:     leaderboardSvc,
		Competitions:    competitionsSvc,
		Achievements:    achievementsSvc,
		ReadinessChecks: readiness,
		Info:            map[string]string{"storage_provider": "memory", "price_provider": "mock"},
	})
}

func doReq(t *testing.T, h http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func doIdempotentReq(t *testing.T, h http.Handler, path, body, token, key string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Idempotency-Key", key)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// --- health / readiness ---------------------------------------------------------

func TestHealth_AlwaysOK(t *testing.T) {
	h := newFullServer(t, nil)
	rec := doReq(t, h, http.MethodGet, "/health", "", "")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"ok"`)
}

func TestCORSPreflight_AllowsIdempotencyKey(t *testing.T) {
	h := newFullServer(t, nil)
	req := httptest.NewRequest(http.MethodOptions, "/portfolio/buys", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "authorization,content-type,idempotency-key")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Contains(t, rec.Header().Get("Access-Control-Allow-Headers"), "Idempotency-Key")
}

func TestReady_OKWhenAllChecksPass(t *testing.T) {
	h := newFullServer(t, []server.ReadinessCheck{
		{Name: "postgres", Check: func(context.Context) error { return nil }},
		{Name: "redis", Check: func(context.Context) error { return nil }},
	})
	rec := doReq(t, h, http.MethodGet, "/ready", "", "")
	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, `"status":"ready"`)
	assert.Contains(t, body, `"postgres":"ok"`)
	assert.Contains(t, body, `"redis":"ok"`)
	assert.Contains(t, body, `"storage_provider":"memory"`)
}

func TestReady_503WhenADependencyFails(t *testing.T) {
	h := newFullServer(t, []server.ReadinessCheck{
		{Name: "postgres", Check: func(context.Context) error { return nil }},
		{Name: "redis", Check: func(context.Context) error { return errors.New("connection refused") }},
	})
	rec := doReq(t, h, http.MethodGet, "/ready", "", "")
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, `"status":"not_ready"`)
	assert.Contains(t, body, `"redis":"error"`)
}

func TestReady_OKWithNoChecks(t *testing.T) {
	h := newFullServer(t, nil)
	rec := doReq(t, h, http.MethodGet, "/ready", "", "")
	assert.Equal(t, http.StatusOK, rec.Code)
}

// --- end-to-end privacy regression (Phase 3, Goal 8) -----------------------------

// forbiddenPublicFields must never appear as JSON keys in social/gamification
// responses. ("symbol" also guards "symbols", "position_id" guards ids, etc.)
var forbiddenPublicFields = []string{
	"email", "password", "password_hash", "user_id", "portfolio_id", "position_id",
	"quantity", "average_buy_price", "baseline_price", "symbol", "positions", "total_cost_basis",
	"current_value", "gain_loss", "starting_value", "starting_value_base",
	"starting_price", "snapshot",
}

func assertNoForbiddenKeys(t *testing.T, endpoint, body string) {
	t.Helper()
	for _, k := range forbiddenPublicFields {
		assert.NotContainsf(t, body, `"`+k+`":`, "%s must not expose %q", endpoint, k)
	}
}

func TestPrivacy_PublicEndpointsExposeNoSensitiveFields(t *testing.T) {
	h := newFullServer(t, nil)

	// Register a user, add positions, join the sprint — produce real data.
	reg := doReq(t, h, http.MethodPost, "/auth/register", `{"email":"alpha@example.com","password":"StrongPassword123","display_name":"AlphaWolf_91","avatar_key":"fox"}`, "")
	require.Equal(t, http.StatusCreated, reg.Code)
	token := extractToken(t, reg.Body.String())

	rec := doIdempotentReq(t, h, "/portfolio/deposits", `{"currency":"USD","amount":5000}`, token, "privacy-deposit")
	require.Equal(t, http.StatusCreated, rec.Code)
	rec = doIdempotentReq(t, h, "/portfolio/buys", `{"symbol":"AAPL","asset_type":"stock","quantity":10}`, token, "privacy-buy")
	require.Equal(t, http.StatusCreated, rec.Code)

	comps := doReq(t, h, http.MethodGet, "/competitions", "", token)
	require.Equal(t, http.StatusOK, comps.Code)
	compID := extractFirstID(t, comps.Body.String())
	join := doReq(t, h, http.MethodPost, "/competitions/"+compID+"/join", "", token)
	require.Equal(t, http.StatusOK, join.Code)

	endpoints := []string{
		"/leaderboard",
		"/competitions/" + compID + "/leaderboard",
		"/competitions/" + compID + "/me",
		"/achievements",
	}
	for _, ep := range endpoints {
		rec := doReq(t, h, http.MethodGet, ep, "", token)
		require.Equalf(t, http.StatusOK, rec.Code, "endpoint %s", ep)
		body := rec.Body.String()
		assertNoForbiddenKeys(t, ep, body)
		assert.NotContainsf(t, body, "alpha@example.com", "%s must not leak the email", ep)
		assert.NotContainsf(t, body, "AAPL", "%s must not leak holdings", ep)
		assert.NotContainsf(t, body, "StrongPassword123", "%s must not leak the password", ep)
	}
}

func extractToken(t *testing.T, body string) string {
	t.Helper()
	const marker = `"token":"`
	i := strings.Index(body, marker)
	require.GreaterOrEqual(t, i, 0, "register response must contain a token")
	rest := body[i+len(marker):]
	return rest[:strings.Index(rest, `"`)]
}

func extractFirstID(t *testing.T, body string) string {
	t.Helper()
	const marker = `"id":"`
	i := strings.Index(body, marker)
	require.GreaterOrEqual(t, i, 0, "competitions response must contain an id")
	rest := body[i+len(marker):]
	return rest[:strings.Index(rest, `"`)]
}
