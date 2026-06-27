package portfolio

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/httpx"
)

// AchievementEvaluator lets the portfolio handler trigger achievement checks
// after a position change or summary view, without importing the achievements
// package (avoids an import cycle). Optional — a nil evaluator skips triggers.
type AchievementEvaluator interface {
	EvaluatePortfolioAchievements(ctx context.Context, userID string) error
}

// Handler adapts HTTP requests to the portfolio Service. Every handler assumes
// it runs behind auth.RequireAuth and reads the user id from the context.
type Handler struct {
	svc       *Service
	evaluator AchievementEvaluator // optional
}

// NewHandler constructs a portfolio Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// SetAchievementEvaluator attaches an optional achievement evaluator that fires
// (best-effort) after a position is added and after the summary is computed.
func (h *Handler) SetAchievementEvaluator(e AchievementEvaluator) {
	h.evaluator = e
}

// evaluatePortfolio fires the achievement evaluator if one is attached. Errors
// are intentionally ignored so badge evaluation never breaks the main request.
func (h *Handler) evaluatePortfolio(ctx context.Context, userID string) {
	if h.evaluator != nil {
		_ = h.evaluator.EvaluatePortfolioAchievements(ctx, userID)
	}
}

// --- response views ----------------------------------------------------------

type portfolioView struct {
	ID       string `json:"id"`
	UserID   string `json:"user_id"`
	Name     string `json:"name"`
	Currency string `json:"currency"`
}

// positionView is the owner-private position shape. BaselinePrice is the price
// locked at add time (today's market price) — there is no average buy price in
// the product.
type positionView struct {
	ID            string  `json:"id"`
	Symbol        string  `json:"symbol"`
	AssetType     string  `json:"asset_type"`
	Quantity      float64 `json:"quantity"`
	BaselinePrice float64 `json:"baseline_price"`
	Currency      string  `json:"currency"`
}

// positionRequest is the create payload: no price, no currency — the baseline
// is locked server-side at the current market quote.
type positionRequest struct {
	Symbol    string  `json:"symbol"`
	AssetType string  `json:"asset_type"`
	Quantity  float64 `json:"quantity"`
}

// updatePositionRequest allows quantity changes only; the symbol and locked
// baseline price are immutable after creation.
type updatePositionRequest struct {
	Quantity float64 `json:"quantity"`
}

func toPositionView(p *Position) positionView {
	return positionView{
		ID:            p.ID,
		Symbol:        p.Symbol,
		AssetType:     p.AssetType,
		Quantity:      p.Quantity,
		BaselinePrice: p.AverageBuyPrice,
		Currency:      p.Currency,
	}
}

func (r positionRequest) toInput() PositionInput {
	return PositionInput{
		Symbol:    r.Symbol,
		AssetType: r.AssetType,
		Quantity:  r.Quantity,
	}
}

// --- handlers ----------------------------------------------------------------

// GetPortfolio handles GET /portfolio.
func (h *Handler) GetPortfolio(w http.ResponseWriter, r *http.Request) {
	userID, ok := userID(w, r)
	if !ok {
		return
	}
	pf, err := h.svc.GetOrCreateDefaultPortfolio(userID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, portfolioView{
		ID: pf.ID, UserID: pf.UserID, Name: pf.Name, Currency: pf.Currency,
	})
}

// AddPosition handles POST /portfolio/positions.
func (h *Handler) AddPosition(w http.ResponseWriter, r *http.Request) {
	userID, ok := userID(w, r)
	if !ok {
		return
	}
	var req positionRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	pos, err := h.svc.AddPosition(r.Context(), userID, req.toInput())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	// First position (and any positive/index-110 state) may unlock badges.
	h.evaluatePortfolio(r.Context(), userID)
	httpx.WriteJSON(w, http.StatusCreated, toPositionView(pos))
}

// ListPositions handles GET /portfolio/positions.
func (h *Handler) ListPositions(w http.ResponseWriter, r *http.Request) {
	userID, ok := userID(w, r)
	if !ok {
		return
	}
	positions, err := h.svc.ListPositions(userID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	views := make([]positionView, 0, len(positions))
	for _, p := range positions {
		views = append(views, toPositionView(p))
	}
	httpx.WriteJSON(w, http.StatusOK, views)
}

// UpdatePosition handles PUT /portfolio/positions/{positionId}.
func (h *Handler) UpdatePosition(w http.ResponseWriter, r *http.Request) {
	userID, ok := userID(w, r)
	if !ok {
		return
	}
	positionID := chi.URLParam(r, "positionId")
	var req updatePositionRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	pos, err := h.svc.UpdatePosition(r.Context(), userID, positionID, req.Quantity)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toPositionView(pos))
}

// DeletePosition handles DELETE /portfolio/positions/{positionId}.
func (h *Handler) DeletePosition(w http.ResponseWriter, r *http.Request) {
	userID, ok := userID(w, r)
	if !ok {
		return
	}
	positionID := chi.URLParam(r, "positionId")
	if err := h.svc.DeletePosition(userID, positionID); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Summary handles GET /portfolio/summary.
func (h *Handler) Summary(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	summary, err := h.svc.Summary(r.Context(), uid)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	// Viewing the summary may unlock green_portfolio / index_110.
	h.evaluatePortfolio(r.Context(), uid)
	httpx.WriteJSON(w, http.StatusOK, summary)
}

// --- helpers -----------------------------------------------------------------

// userID extracts the authenticated user id, writing a 401 if absent (which
// should not happen behind RequireAuth).
func userID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id, ok := auth.UserIDFromContext(r.Context())
	if !ok || id == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return "", false
	}
	return id, true
}

// writeServiceError maps domain errors to HTTP status codes.
func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrSymbolRequired),
		errors.Is(err, ErrInvalidAssetType),
		errors.Is(err, ErrInvalidQuantity),
		errors.Is(err, ErrUnsupportedSymbol),
		errors.Is(err, ErrUnsupportedCurrency):
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrPositionNotFound):
		httpx.WriteError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrPriceProvider):
		httpx.WriteError(w, http.StatusBadGateway, "could not fetch prices from provider")
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
	}
}
