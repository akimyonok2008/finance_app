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
	router   http.Handler
	tm       *auth.TokenManager
	provider *prices.MockPriceProvider
	svc      *portfolio.Service
}

func newTestEnv() *testEnv {
	tm := auth.NewTokenManager("test-secret", time.Hour)
	provider := prices.NewMockPriceProvider()
	svc := portfolio.NewService(portfolio.NewInMemoryRepository(), provider, fx.NewMockFXProvider())
	ph := portfolio.NewHandler(svc)

	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuth(tm))
		r.Get("/portfolio", ph.GetPortfolio)
		r.Get("/portfolio/summary", ph.Summary)
		r.Post("/portfolio/positions", ph.AddPosition)
		r.Get("/portfolio/positions", ph.ListPositions)
		r.Put("/portfolio/positions/{positionId}", ph.UpdatePosition)
		r.Delete("/portfolio/positions/{positionId}", ph.DeletePosition)
		r.Post("/portfolio/positions/{positionId}/close", ph.ClosePosition)
		r.Post("/portfolio/deposits", ph.DepositCash)
		r.Post("/portfolio/buys", ph.BuyPosition)
		r.Post("/portfolio/sells", ph.SellPosition)
		r.Post("/portfolio/fees", ph.RecordFee)
		r.Post("/portfolio/positions/{positionId}/write-off", ph.WriteOffPosition)
		r.Patch("/portfolio/settings", ph.UpdatePortfolioSettings)
	})
	return &testEnv{router: r, tm: tm, provider: provider, svc: svc}
}

func (e *testEnv) token(t *testing.T, userID string) string {
	t.Helper()
	require.NoError(t, e.svc.OnAccountCreated(t.Context(), userID))
	tok, err := e.tm.Generate(userID, userID+"@example.com")
	require.NoError(t, err)
	return tok
}

func (e *testEnv) do(t *testing.T, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	return e.doWithKey(t, method, path, body, token, "")
}

func (e *testEnv) doWithKey(t *testing.T, method, path, body, token, idempotencyKey string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	return rec
}

// No price/currency in the payload: the baseline is locked server-side at the
// current quote (mock AAPL = 195 USD).
const aaplBody = `{"symbol":"AAPL","asset_type":"stock","quantity":10}`

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

func TestPositionMutations_RequireIdempotencyKey(t *testing.T) {
	e := newTestEnv()
	token := e.token(t, "user-1")

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "add", method: http.MethodPost, path: "/portfolio/positions", body: aaplBody},
		{name: "resize", method: http.MethodPut, path: "/portfolio/positions/position-1", body: `{"quantity":12}`},
		{name: "close", method: http.MethodPost, path: "/portfolio/positions/position-1/close"},
		{name: "delete", method: http.MethodDelete, path: "/portfolio/positions/position-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := e.do(t, tt.method, tt.path, tt.body, token)

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), "Idempotency-Key header is required")
		})
	}
}

func TestAddPosition_CreatesValidPosition(t *testing.T) {
	e := newTestEnv()
	rec := e.doWithKey(t, http.MethodPost, "/portfolio/positions", aaplBody, e.token(t, "user-1"), "add-1")

	assert.Equal(t, http.StatusCreated, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "AAPL", body["symbol"])
	assert.NotEmpty(t, body["id"])
	// Baseline locked at today's quote; no average_buy_price anywhere.
	assert.Equal(t, "195", body["baseline_price"])
	assert.NotContains(t, rec.Body.String(), "average_buy_price")
}

func TestAddPosition_RejectsClientSuppliedPrice(t *testing.T) {
	e := newTestEnv()
	// average_buy_price is no longer part of the contract; strict decoding
	// rejects it so clients cannot even attempt to set a historical price.
	legacy := `{"symbol":"AAPL","asset_type":"stock","quantity":10,"average_buy_price":1}`
	rec := e.doWithKey(t, http.MethodPost, "/portfolio/positions", legacy, e.token(t, "user-1"), "add-legacy")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAddPosition_RejectsInvalidPayload(t *testing.T) {
	e := newTestEnv()
	bad := `{"symbol":"AAPL","asset_type":"bond","quantity":10}`
	rec := e.doWithKey(t, http.MethodPost, "/portfolio/positions", bad, e.token(t, "user-1"), "add-invalid")

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assertError(t, rec.Body.Bytes())
}

func TestAddPosition_RejectsMalformedJSON(t *testing.T) {
	e := newTestEnv()
	rec := e.doWithKey(t, http.MethodPost, "/portfolio/positions", `{bad`, e.token(t, "user-1"), "add-malformed")
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
	e.doWithKey(t, http.MethodPost, "/portfolio/positions", aaplBody, tok, "add-list")

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
	created := e.doWithKey(t, http.MethodPost, "/portfolio/positions", aaplBody, tok, "add-update")
	id := decodeID(t, created)

	upd := `{"quantity":12}`
	rec := e.doWithKey(t, http.MethodPut, "/portfolio/positions/"+id, upd, tok, "resize-1")

	assert.Equal(t, http.StatusOK, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "12", body["quantity"])
	// The locked baseline is untouched by the quantity edit.
	assert.Equal(t, "195", body["baseline_price"])
}

func TestUpdatePosition_RejectsOtherUsersPosition(t *testing.T) {
	e := newTestEnv()
	created := e.doWithKey(t, http.MethodPost, "/portfolio/positions", aaplBody, e.token(t, "user-1"), "add-other-update")
	id := decodeID(t, created)

	rec := e.doWithKey(t, http.MethodPut, "/portfolio/positions/"+id, `{"quantity":5}`, e.token(t, "user-2"), "resize-other")
	assert.Equal(t, http.StatusNotFound, rec.Code, "another user's position must be invisible (404)")
}

func TestDeletePosition_DeletesOwn(t *testing.T) {
	e := newTestEnv()
	tok := e.token(t, "user-1")
	created := e.doWithKey(t, http.MethodPost, "/portfolio/positions", aaplBody, tok, "add-delete")
	id := decodeID(t, created)

	rec := e.doWithKey(t, http.MethodDelete, "/portfolio/positions/"+id, "", tok, "delete-1")
	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Deleting again is a 404.
	rec = e.doWithKey(t, http.MethodDelete, "/portfolio/positions/"+id, "", tok, "delete-2")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestDeletePosition_RejectsOtherUsers(t *testing.T) {
	e := newTestEnv()
	created := e.doWithKey(t, http.MethodPost, "/portfolio/positions", aaplBody, e.token(t, "user-1"), "add-other-delete")
	id := decodeID(t, created)

	rec := e.doWithKey(t, http.MethodDelete, "/portfolio/positions/"+id, "", e.token(t, "user-2"), "delete-other")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestSummary_ReturnsCalculatedSummary(t *testing.T) {
	e := newTestEnv()
	tok := e.token(t, "user-1")
	e.doWithKey(t, http.MethodPost, "/portfolio/positions", aaplBody, tok, "add-summary")

	rec := e.do(t, http.MethodGet, "/portfolio/summary", "", tok)
	assert.Equal(t, http.StatusOK, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	// Fresh position: baseline = today's price, so the index starts at 100.
	// Money fields now serialize as decimal strings.
	assert.Equal(t, "1950", body["total_cost_basis"])
	assert.Equal(t, "1950", body["current_value"])
	assert.Equal(t, "0", body["gain_loss"])
	assert.InDelta(t, 100.0, body["portfolio_index"], 0.01)
}

// TestBuyPosition_RejectsEffectiveAtField: there is no backdating for
// buys/sells — every trade is recorded against the live quote at entry time.
// The request body has no effective_at field at all, so submitting one (past
// or future) is rejected as an unrecognized field rather than silently
// accepted or applied.
func TestBuyPosition_RejectsEffectiveAtField(t *testing.T) {
	e := newTestEnv()
	token := e.token(t, "user-1")
	past := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339)

	rec := e.doWithKey(t, http.MethodPost, "/portfolio/buys",
		`{"symbol":"AAPL","asset_type":"stock","quantity":1,"execution_price":190,"effective_at":"`+past+`"}`,
		token, "buy-past")

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assertError(t, rec.Body.Bytes())
}

// TestRecordFee_RejectsFutureEffectiveAt covers the same guard on the fee
// endpoint, which shares parseEffectiveAt with buy/sell.
func TestRecordFee_RejectsFutureEffectiveAt(t *testing.T) {
	e := newTestEnv()
	token := e.token(t, "user-1")
	depositRec := e.doWithKey(t, http.MethodPost, "/portfolio/deposits",
		`{"currency":"USD","amount":1000}`, token, "seed-cash")
	require.Equal(t, http.StatusCreated, depositRec.Code)
	future := time.Now().UTC().Add(48 * time.Hour).Format(time.RFC3339)

	rec := e.doWithKey(t, http.MethodPost, "/portfolio/fees",
		`{"type":"other_fee","currency":"USD","amount":5,"effective_at":"`+future+`"}`,
		token, "fee-future")

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assertError(t, rec.Body.Bytes())
}

func TestGetPortfolio_DefaultsToAutoFundPurchasesTrue(t *testing.T) {
	e := newTestEnv()
	token := e.token(t, "user-1")

	rec := e.do(t, http.MethodGet, "/portfolio", "", token)

	assert.Equal(t, http.StatusOK, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, true, body["auto_fund_purchases"])
}

func TestUpdatePortfolioSettings_DisablesAutoFundPurchases(t *testing.T) {
	e := newTestEnv()
	token := e.token(t, "user-1")

	rec := e.do(t, http.MethodPatch, "/portfolio/settings", `{"auto_fund_purchases":false}`, token)
	assert.Equal(t, http.StatusOK, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, false, body["auto_fund_purchases"])

	// GET /portfolio must reflect the same persisted preference afterward.
	getRec := e.do(t, http.MethodGet, "/portfolio", "", token)
	var getBody map[string]any
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getBody))
	assert.Equal(t, false, getBody["auto_fund_purchases"])
}

func TestUpdatePortfolioSettings_DisabledRejectsFundingBuy(t *testing.T) {
	e := newTestEnv()
	token := e.token(t, "user-1")
	settingsRec := e.do(t, http.MethodPatch, "/portfolio/settings", `{"auto_fund_purchases":false}`, token)
	require.Equal(t, http.StatusOK, settingsRec.Code)

	rec := e.doWithKey(t, http.MethodPost, "/portfolio/buys",
		`{"symbol":"AAPL","asset_type":"stock","quantity":1}`, token, "buy-1")

	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

func decodeID(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	id, _ := body["id"].(string)
	require.NotEmpty(t, id)
	return id
}

// decodeBuyPositionID extracts position.id from a buy mutation response.
func decodeBuyPositionID(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Position struct {
			ID string `json:"id"`
		} `json:"position"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.NotEmpty(t, body.Position.ID)
	return body.Position.ID
}

func TestWriteOffPosition_RejectsPriceableSymbol(t *testing.T) {
	e := newTestEnv()
	token := e.token(t, "user-1")
	buyRec := e.doWithKey(t, http.MethodPost, "/portfolio/buys",
		`{"symbol":"AAPL","asset_type":"stock","quantity":10}`, token, "buy-1")
	require.Equal(t, http.StatusCreated, buyRec.Code)
	positionID := decodeBuyPositionID(t, buyRec)

	rec := e.doWithKey(t, http.MethodPost, "/portfolio/positions/"+positionID+"/write-off", "", token, "wo-1")

	assert.Equal(t, http.StatusConflict, rec.Code)
	assertError(t, rec.Body.Bytes())
}

func TestWriteOffPosition_SucceedsWhenSymbolUnpriceable(t *testing.T) {
	e := newTestEnv()
	token := e.token(t, "user-1")
	buyRec := e.doWithKey(t, http.MethodPost, "/portfolio/buys",
		`{"symbol":"AAPL","asset_type":"stock","quantity":10}`, token, "buy-2")
	require.Equal(t, http.StatusCreated, buyRec.Code)
	positionID := decodeBuyPositionID(t, buyRec)

	e.provider.Unset("AAPL")

	rec := e.doWithKey(t, http.MethodPost, "/portfolio/positions/"+positionID+"/write-off", "", token, "wo-2")

	assert.Equal(t, http.StatusOK, rec.Code)
	var closed map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &closed))
	assert.Equal(t, -100.0, closed["realized_gain_loss_percentage"])
}

func TestWriteOffPosition_RequiresIdempotencyKey(t *testing.T) {
	e := newTestEnv()
	token := e.token(t, "user-1")
	buyRec := e.doWithKey(t, http.MethodPost, "/portfolio/buys",
		`{"symbol":"AAPL","asset_type":"stock","quantity":10}`, token, "buy-3")
	positionID := decodeBuyPositionID(t, buyRec)
	e.provider.Unset("AAPL")

	rec := e.do(t, http.MethodPost, "/portfolio/positions/"+positionID+"/write-off", "", token)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func assertError(t *testing.T, body []byte) {
	t.Helper()
	var e map[string]any
	require.NoError(t, json.Unmarshal(body, &e))
	assert.NotEmpty(t, e["error"])
}
