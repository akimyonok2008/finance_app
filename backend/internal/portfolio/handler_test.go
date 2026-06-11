package portfolio_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/fx"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
	"github.com/ardakimyonok/finance_app/internal/prices"
)

type testEnv struct {
	router http.Handler
	tm     *auth.TokenManager
}

func newTestEnv() *testEnv {
	tm := auth.NewTokenManager("test-secret", time.Hour)
	provider := prices.NewMockPriceProvider()
	svc := portfolio.NewService(portfolio.NewInMemoryRepository(), provider, fx.NewMockFXProvider())
	ph := portfolio.NewHandler(svc)
	priceH := prices.NewHandler(provider)

	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuth(tm))
		r.Get("/portfolio", ph.GetPortfolio)
		r.Get("/portfolio/summary", ph.Summary)
		r.Post("/portfolio/positions", ph.AddPosition)
		r.Get("/portfolio/positions", ph.ListPositions)
		r.Put("/portfolio/positions/{positionId}", ph.UpdatePosition)
		r.Delete("/portfolio/positions/{positionId}", ph.DeletePosition)
		r.Get("/prices/{symbol}", priceH.GetPrice)
	})
	return &testEnv{router: r, tm: tm}
}

func (e *testEnv) token(t *testing.T, userID string) string {
	t.Helper()
	tok, err := e.tm.Generate(userID, userID+"@example.com")
	require.NoError(t, err)
	return tok
}

func (e *testEnv) do(t *testing.T, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	return rec
}

const aaplBody = `{"symbol":"AAPL","asset_type":"stock","quantity":10,"average_buy_price":180,"currency":"USD"}`

func TestGetPortfolio_RequiresAuth(t *testing.T) {
	e := newTestEnv()
	rec := e.do(t, http.MethodGet, "/portfolio", "", "")
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestGetPortfolio_ReturnsDefaultPortfolio(t *testing.T) {
	e := newTestEnv()
	rec := e.do(t, http.MethodGet, "/portfolio", "", e.token(t, "user-1"))

	assert.Equal(t, http.StatusOK, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "Default Portfolio", body["name"])
	assert.Equal(t, "user-1", body["user_id"])
	assert.NotEmpty(t, body["id"])
}

func TestAddPosition_RequiresAuth(t *testing.T) {
	e := newTestEnv()
	rec := e.do(t, http.MethodPost, "/portfolio/positions", aaplBody, "")
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAddPosition_CreatesValidPosition(t *testing.T) {
	e := newTestEnv()
	rec := e.do(t, http.MethodPost, "/portfolio/positions", aaplBody, e.token(t, "user-1"))

	assert.Equal(t, http.StatusCreated, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "AAPL", body["symbol"])
	assert.NotEmpty(t, body["id"])
}

func TestAddPosition_RejectsInvalidPayload(t *testing.T) {
	e := newTestEnv()
	bad := `{"symbol":"AAPL","asset_type":"bond","quantity":10,"average_buy_price":180,"currency":"USD"}`
	rec := e.do(t, http.MethodPost, "/portfolio/positions", bad, e.token(t, "user-1"))

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assertError(t, rec.Body.Bytes())
}

func TestAddPosition_RejectsMalformedJSON(t *testing.T) {
	e := newTestEnv()
	rec := e.do(t, http.MethodPost, "/portfolio/positions", `{bad`, e.token(t, "user-1"))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestListPositions_RequiresAuth(t *testing.T) {
	e := newTestEnv()
	rec := e.do(t, http.MethodGet, "/portfolio/positions", "", "")
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestListPositions_ReturnsUserPositions(t *testing.T) {
	e := newTestEnv()
	tok := e.token(t, "user-1")
	e.do(t, http.MethodPost, "/portfolio/positions", aaplBody, tok)

	rec := e.do(t, http.MethodGet, "/portfolio/positions", "", tok)
	assert.Equal(t, http.StatusOK, rec.Code)
	var list []map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	require.Len(t, list, 1)
	assert.Equal(t, "AAPL", list[0]["symbol"])
}

func TestUpdatePosition_UpdatesOwn(t *testing.T) {
	e := newTestEnv()
	tok := e.token(t, "user-1")
	created := e.do(t, http.MethodPost, "/portfolio/positions", aaplBody, tok)
	id := decodeID(t, created)

	upd := `{"symbol":"AAPL","asset_type":"stock","quantity":12,"average_buy_price":175,"currency":"USD"}`
	rec := e.do(t, http.MethodPut, "/portfolio/positions/"+id, upd, tok)

	assert.Equal(t, http.StatusOK, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, 12.0, body["quantity"])
	assert.Equal(t, 175.0, body["average_buy_price"])
}

func TestUpdatePosition_RejectsOtherUsersPosition(t *testing.T) {
	e := newTestEnv()
	created := e.do(t, http.MethodPost, "/portfolio/positions", aaplBody, e.token(t, "user-1"))
	id := decodeID(t, created)

	rec := e.do(t, http.MethodPut, "/portfolio/positions/"+id, aaplBody, e.token(t, "user-2"))
	assert.Equal(t, http.StatusNotFound, rec.Code, "another user's position must be invisible (404)")
}

func TestDeletePosition_DeletesOwn(t *testing.T) {
	e := newTestEnv()
	tok := e.token(t, "user-1")
	created := e.do(t, http.MethodPost, "/portfolio/positions", aaplBody, tok)
	id := decodeID(t, created)

	rec := e.do(t, http.MethodDelete, "/portfolio/positions/"+id, "", tok)
	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Deleting again is a 404.
	rec = e.do(t, http.MethodDelete, "/portfolio/positions/"+id, "", tok)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestDeletePosition_RejectsOtherUsers(t *testing.T) {
	e := newTestEnv()
	created := e.do(t, http.MethodPost, "/portfolio/positions", aaplBody, e.token(t, "user-1"))
	id := decodeID(t, created)

	rec := e.do(t, http.MethodDelete, "/portfolio/positions/"+id, "", e.token(t, "user-2"))
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestSummary_ReturnsCalculatedSummary(t *testing.T) {
	e := newTestEnv()
	tok := e.token(t, "user-1")
	e.do(t, http.MethodPost, "/portfolio/positions", aaplBody, tok)

	rec := e.do(t, http.MethodGet, "/portfolio/summary", "", tok)
	assert.Equal(t, http.StatusOK, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, 1800.0, body["total_cost_basis"])
	assert.Equal(t, 1950.0, body["current_value"])
	assert.Equal(t, 150.0, body["gain_loss"])
	assert.InDelta(t, 108.33, body["portfolio_index"], 0.01)
}

func TestGetPrice_ReturnsPrice(t *testing.T) {
	e := newTestEnv()
	rec := e.do(t, http.MethodGet, "/prices/AAPL", "", e.token(t, "user-1"))

	assert.Equal(t, http.StatusOK, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "AAPL", body["symbol"])
	assert.Equal(t, 195.0, body["price"])
	assert.Equal(t, "mock", body["source"])
}

func TestGetPrice_UnknownSymbolHandledCleanly(t *testing.T) {
	e := newTestEnv()
	// A well-formed but unsupported symbol → 404 (the mock provider doesn't know it).
	rec := e.do(t, http.MethodGet, "/prices/UNKNOWN", "", e.token(t, "user-1"))

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assertError(t, rec.Body.Bytes())
}

func TestGetPrice_MalformedSymbolIsBadRequest(t *testing.T) {
	e := newTestEnv()
	rec := e.do(t, http.MethodGet, "/prices/A_B", "", e.token(t, "user-1"))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestGetPrice_RequiresAuth(t *testing.T) {
	e := newTestEnv()
	rec := e.do(t, http.MethodGet, "/prices/AAPL", "", "")
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func decodeID(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	id, _ := body["id"].(string)
	require.NotEmpty(t, id)
	return id
}

func assertError(t *testing.T, body []byte) {
	t.Helper()
	var e map[string]any
	require.NoError(t, json.Unmarshal(body, &e))
	assert.NotEmpty(t, e["error"])
}
