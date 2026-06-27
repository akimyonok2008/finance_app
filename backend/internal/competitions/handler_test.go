package competitions_test

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
	"github.com/ardakimyonok/finance_app/internal/clock"
	"github.com/ardakimyonok/finance_app/internal/competitions"
	"github.com/ardakimyonok/finance_app/internal/fx"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
	"github.com/ardakimyonok/finance_app/internal/prices"
)

var fixedTime = time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)

type stubUsers struct{ m map[string]*auth.User }

func (s stubUsers) GetUserByID(_ context.Context, id string) (*auth.User, error) {
	if u, ok := s.m[id]; ok {
		return u, nil
	}
	return nil, assert.AnError
}

type stubPositions struct {
	m map[string][]portfolio.Position
}

func (s stubPositions) ListPositions(_ context.Context, userID string) ([]portfolio.Position, error) {
	return s.m[userID], nil
}

func compID() string { return competitions.WeeklySprintID(fixedTime) }

func newEnv(t *testing.T) (http.Handler, *auth.TokenManager) {
	t.Helper()
	tm := auth.NewTokenManager("test-secret", time.Hour)
	repo := competitions.NewInMemoryCompetitionRepository()
	users := stubUsers{m: map[string]*auth.User{"u1": {ID: "u1", DisplayName: "AlphaWolf_91", AvatarKey: "fox", Email: "a@e.com"}}}
	positions := stubPositions{m: map[string][]portfolio.Position{"u1": {{Symbol: "AAPL", AssetType: "stock", Quantity: 10, AverageBuyPrice: 180, Currency: "USD"}}}}
	svc := competitions.NewService(repo, users, positions, prices.NewMockPriceProvider(), fx.NewMockFXProvider(), &clock.FixedClock{Time: fixedTime})
	h := competitions.NewHandler(svc, nil)

	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuth(tm))
		r.Get("/competitions", h.ListCompetitions)
		r.Post("/competitions/{competitionId}/join", h.JoinCompetition)
		r.Get("/competitions/{competitionId}/me", h.GetMyCompetitionStatus)
		r.Get("/competitions/{competitionId}/leaderboard", h.GetCompetitionLeaderboard)
	})
	return r, tm
}

func do(t *testing.T, router http.Handler, method, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func tokenFor(t *testing.T, tm *auth.TokenManager, uid string) string {
	t.Helper()
	tok, err := tm.Generate(uid, uid+"@e.com")
	require.NoError(t, err)
	return tok
}

func TestList_RequiresAuth(t *testing.T) {
	r, _ := newEnv(t)
	assert.Equal(t, http.StatusUnauthorized, do(t, r, http.MethodGet, "/competitions", "").Code)
}

func TestList_ReturnsCompetitions(t *testing.T) {
	r, tm := newEnv(t)
	rec := do(t, r, http.MethodGet, "/competitions", tokenFor(t, tm, "u1"))
	assert.Equal(t, http.StatusOK, rec.Code)
	var comps []competitions.CompetitionResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &comps))
	require.Len(t, comps, 1)
	assert.Equal(t, compID(), comps[0].ID)
}

func TestJoin_RequiresAuth(t *testing.T) {
	r, _ := newEnv(t)
	assert.Equal(t, http.StatusUnauthorized, do(t, r, http.MethodPost, "/competitions/"+compID()+"/join", "").Code)
}

func TestJoin_Succeeds(t *testing.T) {
	r, tm := newEnv(t)
	rec := do(t, r, http.MethodPost, "/competitions/"+compID()+"/join", tokenFor(t, tm, "u1"))
	assert.Equal(t, http.StatusOK, rec.Code)
	var resp competitions.JoinCompetitionResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.True(t, resp.Joined)
	assert.Equal(t, 100.0, resp.StartingIndex)
}

func TestJoin_NonExistingCompetition(t *testing.T) {
	r, tm := newEnv(t)
	rec := do(t, r, http.MethodPost, "/competitions/weekly_1999_01/join", tokenFor(t, tm, "u1"))
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestMe_RequiresAuth(t *testing.T) {
	r, _ := newEnv(t)
	assert.Equal(t, http.StatusUnauthorized, do(t, r, http.MethodGet, "/competitions/"+compID()+"/me", "").Code)
}

func TestMe_ReturnsStatus(t *testing.T) {
	r, tm := newEnv(t)
	tok := tokenFor(t, tm, "u1")
	do(t, r, http.MethodPost, "/competitions/"+compID()+"/join", tok)
	rec := do(t, r, http.MethodGet, "/competitions/"+compID()+"/me", tok)
	assert.Equal(t, http.StatusOK, rec.Code)
	var st competitions.MyCompetitionStatusResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &st))
	assert.True(t, st.Joined)
}

func TestLeaderboard_RequiresAuth(t *testing.T) {
	r, _ := newEnv(t)
	assert.Equal(t, http.StatusUnauthorized, do(t, r, http.MethodGet, "/competitions/"+compID()+"/leaderboard", "").Code)
}

func TestLeaderboard_ReturnsRankedAndHidesSnapshots(t *testing.T) {
	r, tm := newEnv(t)
	tok := tokenFor(t, tm, "u1")
	do(t, r, http.MethodPost, "/competitions/"+compID()+"/join", tok)

	rec := do(t, r, http.MethodGet, "/competitions/"+compID()+"/leaderboard", tok)
	assert.Equal(t, http.StatusOK, rec.Code)
	var board []competitions.SprintLeaderboardEntry
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &board))
	require.Len(t, board, 1)
	assert.Equal(t, "AlphaWolf_91", board[0].DisplayName)

	body := rec.Body.String()
	for _, k := range []string{"symbol", "quantity", "starting_value", "starting_price", "current_value", "user_id", "email"} {
		assert.NotContainsf(t, body, `"`+k+`":`, "must not expose %q", k)
	}
	assert.NotContains(t, body, "AAPL")
	assert.NotContains(t, body, "a@e.com")
}
