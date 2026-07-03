package marketdata

import (
	"errors"
	"net/http"
	"strings"

	"github.com/ardakimyonok/finance_app/internal/httpx"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) SearchInstruments(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	resp, err := h.svc.SearchInstruments(r.Context(), q)
	if err != nil {
		writeMarketDataError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) Quotes(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimSpace(r.URL.Query().Get("symbols"))
	resp, err := h.svc.Quotes(r.Context(), strings.Split(raw, ","))
	if err != nil {
		writeMarketDataError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func writeMarketDataError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidQuery), errors.Is(err, ErrInvalidSymbols):
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrSymbolNotFound):
		httpx.WriteError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrProviderRateLimited), errors.Is(err, ErrDailyBudgetExhausted):
		httpx.WriteError(w, http.StatusTooManyRequests, "market data provider rate limited")
	case errors.Is(err, ErrProviderUnavailable):
		httpx.WriteError(w, http.StatusBadGateway, "market data provider unavailable")
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "market data error")
	}
}
