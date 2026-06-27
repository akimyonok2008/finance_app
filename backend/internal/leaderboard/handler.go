package leaderboard

import (
	"net/http"

	"github.com/ardakimyonok/finance_app/internal/auth"
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

// GetLeaderboard handles GET /leaderboard?timeframe=1W|1M|3M|6M|1Y|ALL. An
// unknown/absent timeframe defaults to ALL. Only an inability to enumerate users
// yields a 500.
func (h *Handler) GetLeaderboard(w http.ResponseWriter, r *http.Request) {
	tf := ParseTimeframe(r.URL.Query().Get("timeframe"))
	board, err := h.svc.BuildTimeframe(r.Context(), tf)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not build leaderboard")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, board)
}

// standingView is the privacy-safe shape for GET /leaderboard/me. Rank is null
// when the caller has no rankable portfolio yet.
type standingView struct {
	Timeframe              Timeframe `json:"timeframe"`
	Eligible               bool      `json:"eligible"`
	Rank                   *int      `json:"rank"`
	TotalParticipants      int       `json:"total_participants"`
	RankedReturnPercentage float64   `json:"ranked_return_percentage"`
	RankedIndex            float64   `json:"ranked_index"`
}

// GetMyStanding handles GET /leaderboard/me?timeframe=... — the caller's own
// rank and timeframe performance. Authenticated.
func (h *Handler) GetMyStanding(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	tf := ParseTimeframe(r.URL.Query().Get("timeframe"))
	st, err := h.svc.UserStanding(r.Context(), userID, tf)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not load standing")
		return
	}
	view := standingView{
		Timeframe:              st.Timeframe,
		Eligible:               st.Ranked,
		TotalParticipants:      st.TotalParticipants,
		RankedReturnPercentage: st.RankedReturnPercentage,
		RankedIndex:            st.RankedIndex,
	}
	if st.Ranked {
		rank := st.Rank
		view.Rank = &rank
	}
	httpx.WriteJSON(w, http.StatusOK, view)
}
