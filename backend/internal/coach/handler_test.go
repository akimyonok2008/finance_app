package coach

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
)

func newHandlerEnv(t *testing.T) (http.Handler, *auth.TokenManager) {
	t.Helper()
	tm := auth.NewTokenManager("test-secret", time.Hour)
	users := []auth.User{user("u1", "Alpha"), user("u2", "Bravo")}
	sums := map[string]*portfolio.PortfolioSummary{
		"u1": summaryWith("u1", []testPosition{
			{"AAPL", "stock", "USD", 700, 0.08},
			{"NVDA", "stock", "USD", 300, 0.12},
		}),
		"u2": summaryWith("u2", []testPosition{{"SPY", "etf", "USD", 1000, 0.2}}),
		// u3 has a token but no portfolio (empty-portfolio path).
		"u3": summaryWith("u3", nil),
	}
	svc := newCoachService(users, sums)
	h := NewHandler(svc)

	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuth(tm))
		r.Post("/portfolio/coach", h.Coach)
	})
	return r, tm
}

func postCoach(t *testing.T, router http.Handler, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/portfolio/coach", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestHandler_RequiresAuth(t *testing.T) {
	router, _ := newHandlerEnv(t)
	rec := postCoach(t, router, "", `{"mode":"analyze_portfolio"}`)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestHandler_InvalidTokenIs401(t *testing.T) {
	router, _ := newHandlerEnv(t)
	rec := postCoach(t, router, "not.a.token", `{"mode":"analyze_portfolio"}`)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestHandler_UnsupportedModeIs400(t *testing.T) {
	router, tm := newHandlerEnv(t)
	token, err := tm.Generate("u1", "u1@example.com")
	require.NoError(t, err)
	rec := postCoach(t, router, token, `{"mode":"buy_everything"}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "unsupported coach mode")
}

func TestHandler_EmptyPortfolioIs400(t *testing.T) {
	router, tm := newHandlerEnv(t)
	token, err := tm.Generate("u3", "u3@example.com") // u3 has no positions
	require.NoError(t, err)
	rec := postCoach(t, router, token, `{"mode":"analyze_portfolio"}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "portfolio has no positions to analyze")
}

func TestHandler_SuccessReturnsStructuredAnalysis(t *testing.T) {
	router, tm := newHandlerEnv(t)
	token, err := tm.Generate("u1", "u1@example.com")
	require.NoError(t, err)
	rec := postCoach(t, router, token, `{"mode":"compare_top10"}`)
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, `"mode":"compare_top10"`)
	assert.Contains(t, body, Disclaimer)
	assert.Contains(t, body, `"top10_comparison":`)
	// Privacy: no other user's identity/values in the HTTP response.
	assertNoForbiddenKeys(t, "handler response", body)
	assert.NotContains(t, body, "u2@example.com")
}

func TestHandler_MalformedBodyIs400(t *testing.T) {
	router, tm := newHandlerEnv(t)
	token, err := tm.Generate("u1", "u1@example.com")
	require.NoError(t, err)
	rec := postCoach(t, router, token, `{not json`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
