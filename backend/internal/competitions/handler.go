package competitions

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/httpx"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
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

// GetArenaCatalogue handles GET /arena/competitions: one paginated bucket/tab
// of the catalogue (legacy sprint and engine editions) as privacy-safe
// catalogue cards. This is Arena's own discovery surface — deliberately
// separate from the global leaderboard and never redirected to it.
//
// Query params: ?bucket=live|upcoming|mine|completed (omit for no bucket
// filter), ?category=, ?limit=, ?offset=.
func (h *Handler) GetArenaCatalogue(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	page, err := h.svc.ArenaCatalogue(r.Context(), uid, ArenaCatalogueQuery{
		Bucket:   r.URL.Query().Get("bucket"),
		Category: r.URL.Query().Get("category"),
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, ArenaCataloguePageResponse{
		Items:   page.Cards,
		HasMore: page.HasMore,
	})
}

// GetArenaCatalogueItem handles GET /arena/competitions/{competitionId}: one
// competition's catalogue card, used by the competition detail screen
// alongside eligibility/me/leaderboard/results.
func (h *Handler) GetArenaCatalogueItem(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	competitionID := chi.URLParam(r, "competitionId")
	card, err := h.svc.ArenaCatalogueItem(r.Context(), competitionID, uid)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, card)
}

// JoinCompetition handles POST /competitions/{competitionId}/join. Engine
// editions require an Idempotency-Key header (application-wide mutation
// convention); legacy weekly sprints keep accepting key-less joins for
// compatibility. An ineligible join returns 422 with the rule evidence.
func (h *Handler) JoinCompetition(w http.ResponseWriter, r *http.Request) {
	userID, ok := userID(w, r)
	if !ok {
		return
	}
	competitionID := chi.URLParam(r, "competitionId")
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))

	resp, err := h.svc.Join(r.Context(), competitionID, userID, key)
	if errors.Is(err, ErrNotEligible) {
		httpx.WriteJSON(w, http.StatusUnprocessableEntity, resp)
		return
	}
	if err != nil {
		writeServiceError(w, err)
		return
	}

	if h.evaluator != nil {
		_ = h.evaluator.EvaluateSprintJoinAchievements(r.Context(), userID)
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

// GetMyCompetitionStatus handles GET /competitions/{competitionId}/me. Legacy
// weekly sprints keep the pre-engine live-snapshot path; engine editions read
// the caller's rank from the ranking projection (never a live scan).
func (h *Handler) GetMyCompetitionStatus(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	competitionID := chi.URLParam(r, "competitionId")
	ctx := r.Context()

	comp, err := h.svc.GetCompetition(ctx, competitionID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	var st *MyCompetitionStatusResponse
	if comp.IsLegacy() {
		st, err = h.svc.MyStatus(ctx, competitionID, uid)
	} else {
		st, err = h.svc.EngineMyStatus(ctx, comp, uid)
	}
	if err != nil {
		writeServiceError(w, err)
		return
	}

	if h.evaluator != nil {
		_ = h.evaluator.EvaluateSprintRankAchievements(ctx, uid, competitionID)
	}
	httpx.WriteJSON(w, http.StatusOK, st)
}

// GetCompetitionLeaderboard handles GET /competitions/{competitionId}/leaderboard.
// Legacy weekly sprints keep the pre-engine array response. Engine editions
// return a cursor-paginated page from the ranking projection (?after=<rank>&
// limit=<n>); Unavailable=true (never a live O(N) scan) means no generation
// has been promoted yet.
func (h *Handler) GetCompetitionLeaderboard(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	competitionID := chi.URLParam(r, "competitionId")
	ctx := r.Context()

	comp, err := h.svc.GetCompetition(ctx, competitionID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	if comp.IsLegacy() {
		board, err := h.svc.Leaderboard(ctx, competitionID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		if h.evaluator != nil {
			_ = h.evaluator.EvaluateSprintRankAchievements(ctx, uid, competitionID)
		}
		httpx.WriteJSON(w, http.StatusOK, board)
		return
	}

	after, _ := strconv.Atoi(r.URL.Query().Get("after"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	page, err := h.svc.EditionLeaderboard(ctx, competitionID, after, limit)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if h.evaluator != nil {
		_ = h.evaluator.EvaluateSprintRankAchievements(ctx, uid, competitionID)
	}
	httpx.WriteJSON(w, http.StatusOK, page)
}

// GetCompetitionResults handles GET /competitions/{competitionId}/results:
// the immutable final leaderboard of a completed engine edition, read only
// from competition_results — never repriced with current market data.
func (h *Handler) GetCompetitionResults(w http.ResponseWriter, r *http.Request) {
	if _, ok := userID(w, r); !ok {
		return
	}
	competitionID := chi.URLParam(r, "competitionId")
	results, err := h.svc.Results(r.Context(), competitionID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, results)
}

// WithdrawFromCompetition handles DELETE /competitions/{competitionId}/entry:
// leaving before registration closes (engine editions only).
func (h *Handler) WithdrawFromCompetition(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	competitionID := chi.URLParam(r, "competitionId")
	if err := h.svc.Withdraw(r.Context(), competitionID, uid); err != nil {
		writeServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"competition_id": competitionID,
		"withdrawn":      true,
	})
}

// GetCompetitionEligibility handles GET /competitions/{competitionId}/eligibility:
// a server-computed, privacy-safe preview of whether the caller could enter.
// Percentages, counts and reason codes only — never absolute portfolio values.
func (h *Handler) GetCompetitionEligibility(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	competitionID := chi.URLParam(r, "competitionId")

	resp, err := h.svc.EligibilityPreview(r.Context(), competitionID, uid)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
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
		errors.Is(err, ErrJoinSnapshot),
		errors.Is(err, ErrIdempotencyKeyRequired),
		errors.Is(err, portfolio.ErrCompetitionSnapshotUnpriced):
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrNotEligible),
		errors.Is(err, ErrNothingToScore):
		httpx.WriteError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, ErrJoinWindowClosed),
		errors.Is(err, ErrWithdrawalClosed),
		errors.Is(err, ErrEntryConflict),
		errors.Is(err, ErrIdempotencyConflict):
		httpx.WriteError(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrEntryNotFound):
		httpx.WriteError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, portfolio.ErrCompetitionSnapshotConflict):
		httpx.WriteError(w, http.StatusConflict, "portfolio is changing; retry shortly")
	case errors.Is(err, ErrEligibilityUnavailable), errors.Is(err, ErrRankingUnavailable):
		httpx.WriteError(w, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, ErrResultsNotAvailable):
		httpx.WriteError(w, http.StatusConflict, err.Error())
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
	}
}
