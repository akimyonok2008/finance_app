package leaderboard

import (
	"net/http"

	"github.com/ardakimyonok/finance_app/internal/httpx"
)

// Handler exposes the leaderboard over HTTP. It assumes it runs behind
// auth.RequireAuth.
type Handler struct {
	svc *Service
}

// NewHandler constructs a leaderboard Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// GetLeaderboard handles GET /leaderboard. It returns the ranked, privacy-safe
// entries; only an inability to enumerate users yields a 500.
func (h *Handler) GetLeaderboard(w http.ResponseWriter, r *http.Request) {
	board, err := h.svc.Build(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not build leaderboard")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, board)
}
