package achievements

import (
	"net/http"

	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/httpx"
)

// Handler exposes a user's achievements over HTTP.
type Handler struct {
	svc *Service
}

// NewHandler constructs an achievements Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// ListAchievements handles GET /achievements for the authenticated user.
func (h *Handler) ListAchievements(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	list, err := h.svc.ListAchievementsForUser(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not list achievements")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, list)
}

// Evaluate handles POST /achievements/evaluate: it re-evaluates all of the
// authenticated user's achievements and returns the updated list.
func (h *Handler) Evaluate(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	list, err := h.svc.EvaluateAll(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not evaluate achievements")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, list)
}
