package prices

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ardakimyonok/finance_app/internal/httpx"
)

// Handler exposes price lookups over HTTP.
type Handler struct {
	provider PriceProvider
}

// NewHandler constructs a price Handler backed by the given provider.
func NewHandler(provider PriceProvider) *Handler {
	return &Handler{provider: provider}
}

// GetPrice handles GET /prices/{symbol}. Provider failures map to 502 so the
// client can distinguish an upstream data issue from a client error.
func (h *Handler) GetPrice(w http.ResponseWriter, r *http.Request) {
	symbol := chi.URLParam(r, "symbol")
	if symbol == "" {
		httpx.WriteError(w, http.StatusBadRequest, "symbol is required")
		return
	}

	// Reject obviously malformed symbols before hitting the provider.
	if _, err := ValidateAndNormalizeSymbol(symbol); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid symbol")
		return
	}

	price, err := h.provider.GetLatestPrice(r.Context(), symbol)
	if err != nil {
		// A symbol the provider simply doesn't know is a 404; anything else is
		// treated as an upstream data-source failure (502).
		if errors.Is(err, ErrPriceUnavailable) {
			httpx.WriteError(w, http.StatusNotFound, "unsupported or unpriceable symbol")
			return
		}
		httpx.WriteError(w, http.StatusBadGateway, "could not fetch price: "+err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, price)
}
