package competitions

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/httpx"
)

// AchievementEvaluator lets the handler trigger achievement checks without
// importing the achievements package (avoids an import cycle). It is optional;
// a nil evaluator simply skips the triggers.
type AchievementEvaluator interface {
	EvaluateSprintJoinAchievements(ctx context.Context, userID string) error
	EvaluateSprintRankAchievements(ctx context.Context, userID, competitionID string) error
}

// Handler adapts competition HTTP requests to the Service.
type Handler struct {
	svc       *Service
	evaluator AchievementEvaluator // optional
}

// NewHandler constructs a competitions Handler. evaluator may be nil.
func NewHandler(svc *Service, evaluator AchievementEvaluator) *Handler {
	return &Handler{svc: svc, evaluator: evaluator}
}

// ListCompetitions handles GET /competitions.
func (h *Handler) ListCompetitions(w http.ResponseWriter, r *http.Request) {
	comps, err := h.svc.ListCompetitions(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not list competitions")
		return
	}
	out := make([]CompetitionResponse, 0, len(comps))
	for _, c := range comps {
		out = append(out, toCompetitionResponse(c))
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// JoinCompetition handles POST /competitions/{competitionId}/join.
func (h *Handler) JoinCompetition(w http.ResponseWriter, r *http.Request) {
	userID, ok := userID(w, r)
	if !ok {
		return
	}
	competitionID := chi.URLParam(r, "competitionId")

	resp, err := h.svc.JoinCompetition(r.Context(), competitionID, userID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	if h.evaluator != nil {
		_ = h.evaluator.EvaluateSprintJoinAchievements(r.Context(), userID)
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

// GetMyCompetitionStatus handles GET /competitions/{competitionId}/me.
func (h *Handler) GetMyCompetitionStatus(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	competitionID := chi.URLParam(r, "competitionId")

	st, err := h.svc.MyStatus(r.Context(), competitionID, uid)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	if h.evaluator != nil {
		_ = h.evaluator.EvaluateSprintRankAchievements(r.Context(), uid, competitionID)
	}
	httpx.WriteJSON(w, http.StatusOK, st)
}

// GetCompetitionLeaderboard handles GET /competitions/{competitionId}/leaderboard.
func (h *Handler) GetCompetitionLeaderboard(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	competitionID := chi.URLParam(r, "competitionId")

	board, err := h.svc.Leaderboard(r.Context(), competitionID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	if h.evaluator != nil {
		_ = h.evaluator.EvaluateSprintRankAchievements(r.Context(), uid, competitionID)
	}
	httpx.WriteJSON(w, http.StatusOK, board)
}

// userID extracts the authenticated user id (401 if missing).
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
	case errors.Is(err, ErrCompetitionNotFound):
		httpx.WriteError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrCompetitionNotActive),
		errors.Is(err, ErrEmptyPortfolio),
		errors.Is(err, ErrJoinSnapshot):
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
	}
}
