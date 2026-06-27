package coach

import (
	"errors"
	"net/http"

	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/httpx"
)

// Handler exposes the Portfolio Coach over HTTP. It runs behind the JWT
// middleware (RequireAuthWithUser).
type Handler struct {
	svc *Service
}

// NewHandler constructs a coach Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Coach handles POST /portfolio/coach. It authenticates the user, validates the
// requested mode, and returns structured analysis. The provider is never called
// for an empty portfolio.
func (h *Handler) Coach(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req CoachRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !SupportedMode(req.Mode) {
		httpx.WriteError(w, http.StatusBadRequest, "unsupported coach mode")
		return
	}

	resp, err := h.svc.Analyze(r.Context(), userID, req.Mode)
	if err != nil {
		switch {
		case errors.Is(err, ErrUnsupportedMode):
			httpx.WriteError(w, http.StatusBadRequest, "unsupported coach mode")
		case errors.Is(err, ErrEmptyPortfolio):
			httpx.WriteError(w, http.StatusBadRequest, "portfolio has no positions to analyze")
		default:
			httpx.WriteError(w, http.StatusInternalServerError, "could not generate coach analysis")
		}
		return
	}

	httpx.WriteJSON(w, http.StatusOK, resp)
}
