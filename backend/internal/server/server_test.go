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
	"github.com/ardakimyonok/finance_app/internal/clock"
	"github.com/ardakimyonok/finance_app/internal/competitions"
	"github.com/ardakimyonok/finance_app/internal/fx"
	"github.com/ardakimyonok/finance_app/internal/leaderboard"
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

type positionProvider struct{ s *portfolio.Service }

func (p positionProvider) ListPositions(_ context.Context, userID string) ([]portfolio.Position, error) {
	ptrs, err := p.s.ListPositions(userID)
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

// newFullServer builds the complete application with in-memory storage,
// exactly as main.go wires it (minus Postgres/Redis).
func newFullServer(t *testing.T, readiness []server.ReadinessCheck) http.Handler {
	t.Helper()
	tokens := auth.NewTokenManager("test-secret", time.Hour)
	authSvc := auth.NewService(auth.NewInMemoryUserRepository(), tokens)
	fxp := fx.NewMockFXProvider()
	priceProvider := prices.NewMockPriceProvider()
	portfolioSvc := portfolio.NewService(portfolio.NewInMemoryRepository(), priceProvider, fxp)
	leaderboardSvc := leaderboard.NewService(authSvc, portfolioSvc)
	competitionsSvc := competitions.NewService(
		competitions.NewInMemoryCompetitionRepository(),
		userProvider{authSvc}, positionProvider{portfolioSvc},
		priceProvider, fxp, clock.RealClock{},
	)
	achievementsSvc := achievements.NewService(
		achievements.NewInMemoryAchievementRepository(),
		positionProvider{portfolioSvc}, summaryProvider{portfolioSvc},
		rankProvider{competitionsSvc},
	)
	achievementsSvc.SetCurrentCompetitionProvider(competitionsSvc)

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

// --- health / readiness ---------------------------------------------------------

func TestHealth_AlwaysOK(t *testing.T) {
	h := newFullServer(t, nil)
	rec := doReq(t, h, http.MethodGet, "/health", "", "")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"ok"`)
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

	rec := doReq(t, h, http.MethodPost, "/portfolio/positions", `{"symbol":"AAPL","asset_type":"stock","quantity":10}`, token)
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
