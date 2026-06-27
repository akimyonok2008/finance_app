package profile

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
)

func TestExploreHandlerRequiresAuth(t *testing.T) {
	handler := NewHandler(exploreTestService(t))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/profiles/explore", nil)
	handler.Explore(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestExploreRouteEndToEnd(t *testing.T) {
	tokens := auth.NewTokenManager("test-secret", time.Hour)
	authSvc := auth.NewService(auth.NewInMemoryUserRepository(), tokens)
	_, token, err := authSvc.Register(auth.RegisterInput{
		Email: "explorer@example.com", Password: "StrongPassword123", DisplayName: "Explorer",
	})
	require.NoError(t, err)

	repo := NewInMemoryRepository()
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, repo.Create(ctx, Profile{
		UserID: "u1", Handle: "alpha_wolf", DisplayName: "AlphaWolf", StrategyTag: DefaultStrategyTag,
		IsPublic: true, ShowPublicWeights: true, CreatedAt: now, UpdatedAt: now,
	}))
	summaries := testSummaries{
		"u1": {UserID: "u1", PortfolioID: "p1", CurrentValue: 100, GainLossPercentage: 24.6, PortfolioIndex: 124.6,
			Positions: []portfolio.PositionSummary{
				{PositionID: "x1", Symbol: "NVDA", AssetType: "stock", CurrentValueBase: 60, CurrentPriceCurrency: "USD"},
				{PositionID: "x2", Symbol: "AAPL", AssetType: "stock", CurrentValueBase: 40, CurrentPriceCurrency: "USD"},
			}},
	}
	svc := NewService(repo, authUserProvider{authSvc}, summaries)

	router := chi.NewRouter()
	router.Use(auth.RequireAuthWithUser(tokens, authSvc))
	router.Get("/profiles/explore", NewHandler(svc).Explore)

	do := func(path, bearer string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	// Unauthenticated.
	assert.Equal(t, http.StatusUnauthorized, do("/profiles/explore", "").Code)

	// Invalid params -> 400.
	assert.Equal(t, http.StatusBadRequest, do("/profiles/explore?sort=wealthiest", token).Code)
	assert.Equal(t, http.StatusBadRequest, do("/profiles/explore?symbol=bad%20symbol!", token).Code)
	assert.Equal(t, http.StatusBadRequest, do("/profiles/explore?limit=0", token).Code)

	// Happy path -> 200, sections present, no forbidden keys, no PII.
	ok := do("/profiles/explore?sort=top", token)
	require.Equal(t, http.StatusOK, ok.Code)
	body := ok.Body.String()
	for _, section := range []string{`"featured"`, `"top_performers"`, `"trending_holdings"`, `"pagination"`} {
		assert.Contains(t, body, section)
	}
	assert.Contains(t, body, "alpha_wolf")
	for _, forbidden := range []string{
		`"quantity"`, `"current_value"`, `"cost_basis"`, `"gain_loss"`,
		`"user_id"`, `"portfolio_id"`, `"position_id"`, `"email"`,
	} {
		assert.NotContains(t, body, forbidden)
	}
	assert.NotContains(t, body, "explorer@example.com")
}
