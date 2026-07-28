package portfolio

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ardakimyonok/finance_app/internal/httpx"
	"github.com/ardakimyonok/finance_app/internal/money"
)

// These handlers record user-reported income, fees, and corporate actions. They
// are owner-private and change nothing about brokerage execution — the product
// records activity for tracking only.

type feeRequest struct {
	Type             string       `json:"type"`
	Currency         string       `json:"currency"`
	Amount           money.Amount `json:"amount"`
	Symbol           string       `json:"symbol"`
	LinkedActivityID string       `json:"linked_activity_id"`
	Description      string       `json:"description"`
	OccurredAt       string       `json:"effective_at"`
}

// ListIncomeEvents handles GET /portfolio/income-events. Ordinary income
// (dividends, ETF/fund distributions, bond coupons, return of capital, stock
// dividends) is detected and credited automatically by the background pipeline;
// this endpoint is read-only and owner-private. There is deliberately no
// endpoint that creates arbitrary income.
func (h *Handler) ListIncomeEvents(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	if h.incomeView == nil {
		httpx.WriteJSON(w, http.StatusOK, []IncomeEventView{})
		return
	}
	views, err := h.incomeView.ListIncomeEventViews(r.Context(), uid)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if views == nil {
		views = []IncomeEventView{}
	}
	httpx.WriteJSON(w, http.StatusOK, views)
}

// GetIncomeEvent handles GET /portfolio/income-events/{id}, owner-private.
func (h *Handler) GetIncomeEvent(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	if h.incomeView == nil {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	pf, err := h.svc.GetOrCreateDefaultPortfolio(r.Context(), uid)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	view, found, err := h.incomeView.GetIncomeEventView(r.Context(), uid, pf.ID, chi.URLParam(r, "id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if !found {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, view)
}

type incomeCorrectionRequest struct {
	Kind                    string         `json:"kind"`
	ActualNet               money.Amount   `json:"actual_net"`
	ActualWithholding       money.Amount   `json:"actual_withholding"`
	ActualFee               money.Amount   `json:"actual_fee"`
	ActualReinvestmentQty   money.Quantity `json:"actual_reinvestment_quantity"`
	ActualReinvestmentPrice money.Price    `json:"actual_reinvestment_price"`
}

// CorrectIncomeEvent handles POST /portfolio/income-events/{id}/correction. It
// allows only constrained, account-specific reconciliation of an already-
// detected event — never arbitrary income creation.
func (h *Handler) CorrectIncomeEvent(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	if h.incomeView == nil {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	var req incomeCorrectionRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	requestID, ok := requiredIdempotencyKey(w, r)
	if !ok {
		return
	}
	pf, err := h.svc.GetOrCreateDefaultPortfolio(r.Context(), uid)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	err = h.incomeView.CorrectIncomeEvent(r.Context(), uid, IncomeCorrectionInput{
		IncomeEventID:           chi.URLParam(r, "id"),
		PortfolioID:             pf.ID,
		Kind:                    req.Kind,
		RequestID:               requestID,
		ActualNet:               req.ActualNet,
		ActualWithholding:       req.ActualWithholding,
		ActualFee:               req.ActualFee,
		ActualReinvestmentQty:   req.ActualReinvestmentQty,
		ActualReinvestmentPrice: req.ActualReinvestmentPrice,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	view, found, err := h.incomeView.GetIncomeEventView(r.Context(), uid, pf.ID, chi.URLParam(r, "id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if !found {
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "corrected"})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, view)
}

type activityCorrectionRequest struct {
	ActualAmount float64 `json:"actual_amount"`
	Reason       string  `json:"reason"`
}

// CorrectActivity handles POST /portfolio/activities/{id}/correction. It
// reconciles a user-recorded deposit or withdrawal to its actual amount by
// posting a compensating activity — the original, immutable activity is never
// edited. Buy/sell activities are rejected (record an offsetting sell/buy
// instead); this endpoint never accepts automatic/system-generated activities
// since those already have their own dedicated correction paths (e.g. income).
func (h *Handler) CorrectActivity(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	var req activityCorrectionRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	requestID, ok := requiredIdempotencyKey(w, r)
	if !ok {
		return
	}
	res, err := h.svc.CorrectActivity(r.Context(), uid, requestID, ActivityCorrectionInput{
		ActivityID:   chi.URLParam(r, "id"),
		ActualAmount: money.AmountFromFloat64(req.ActualAmount),
		Reason:       req.Reason,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, mutationView(res))
}

// RecordFee handles POST /portfolio/fees.
func (h *Handler) RecordFee(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	var req feeRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	occurredAt, ok := parseEffectiveAt(w, req.OccurredAt)
	if !ok {
		return
	}
	requestID, ok := requiredIdempotencyKey(w, r)
	if !ok {
		return
	}
	res, err := h.svc.RecordFee(r.Context(), uid, requestID, FeeInput{
		Subtype:          FeeSubtype(req.Type),
		Currency:         req.Currency,
		Amount:           req.Amount,
		Symbol:           req.Symbol,
		LinkedActivityID: req.LinkedActivityID,
		Description:      req.Description,
		OccurredAt:       occurredAt,
		Provenance:       ProvenanceUserReported,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, mutationView(res))
}

// ListCorporateActions handles GET /portfolio/corporate-actions. Corporate
// actions are applied automatically by the background pipeline; this endpoint is
// read-only and owner-private. Users cannot record or edit corporate actions.
func (h *Handler) ListCorporateActions(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	if h.corpView == nil {
		// Pipeline not configured: an empty, well-formed list (never an error, so
		// the UI can render the "no automatic adjustments" state).
		httpx.WriteJSON(w, http.StatusOK, []CorporateActionView{})
		return
	}
	views, err := h.corpView.ListCorporateActionViews(r.Context(), uid)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if views == nil {
		views = []CorporateActionView{}
	}
	httpx.WriteJSON(w, http.StatusOK, views)
}

// effectiveAtFutureTolerance absorbs ordinary clock skew between the client
// and the server without opening the door to a genuinely future-dated entry
// (a trade, deposit, or fee that "happens" tomorrow, which would let a public
// profile display performance for a period that hasn't occurred yet).
const effectiveAtFutureTolerance = 5 * time.Minute

// parseEffectiveAt parses an optional ISO-8601 effective_at, writing a 400 and
// returning ok=false on a malformed or future-dated value. Backdating is a
// deliberate, supported feature (see the historical-quantity tests); only a
// date beyond "now" is rejected, since nothing about this product can make an
// activity that hasn't happened yet true.
func parseEffectiveAt(w http.ResponseWriter, raw string) (*time.Time, bool) {
	if raw == "" {
		return nil, true
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "effective_at must be ISO-8601")
		return nil, false
	}
	if parsed.After(time.Now().UTC().Add(effectiveAtFutureTolerance)) {
		httpx.WriteError(w, http.StatusBadRequest, "effective_at cannot be in the future")
		return nil, false
	}
	return &parsed, true
}
