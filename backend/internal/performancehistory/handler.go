package performancehistory

import (
	"net/http"

	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/httpx"
)

// Handler serves the canonical ranked-performance history. It is owner-private:
// the user id always comes from the authenticated context, never from a query
// parameter.
type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// History handles GET /performance/history?timeframe=1M.
//
// This reads the canonical ranked snapshot history — the same table the
// leaderboard and benchmark-achievement evidence read — NOT the private
// portfolio valuation archive.
func (h *Handler) History(w http.ResponseWriter, r *http.Request) {
	uid, ok := auth.UserIDFromContext(r.Context())
	if !ok || uid == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	history, err := h.svc.RankedHistory(r.Context(), uid, r.URL.Query().Get("timeframe"))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not load performance history")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, history)
}
