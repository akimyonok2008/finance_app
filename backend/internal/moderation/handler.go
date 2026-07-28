package moderation

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/httpx"
)

type Handler struct {
	svc   *Service
	users UserAdmin
}

func NewHandler(svc *Service, users UserAdmin) *Handler {
	return &Handler{svc: svc, users: users}
}

// RequireModerator gates moderation endpoints on backend account state
// (never a frontend-supplied role flag): the caller's role is re-read from
// the database via UserAdmin on every request.
func RequireModerator(users UserAdmin) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, ok := auth.UserIDFromContext(r.Context())
			if !ok || userID == "" {
				httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			view, err := users.UserByID(r.Context(), userID)
			if err != nil || (view.Role != "moderator" && view.Role != "admin") {
				httpx.WriteError(w, http.StatusForbidden, "moderation_forbidden")
				return
			}
			ctx := context.WithValue(r.Context(), actorRoleKey{}, view)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

type actorRoleKey struct{}

func actorFromContext(ctx context.Context) UserView {
	v, _ := ctx.Value(actorRoleKey{}).(UserView)
	return v
}

func (h *Handler) ReportUser(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var input ReportInput
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	out, err := h.svc.ReportUser(r.Context(), userID, chi.URLParam(r, "userId"), input.Category, input.Explanation)
	if err != nil {
		writeModerationError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, out)
}

func (h *Handler) ReportMessage(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var input ReportInput
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	out, err := h.svc.ReportMessage(r.Context(), userID, chi.URLParam(r, "messageId"), input.Category, input.Explanation)
	if err != nil {
		writeModerationError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, out)
}

func (h *Handler) ListReports(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.ListReports(r.Context(), r.URL.Query().Get("status"))
	if err != nil {
		writeModerationError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) GetReport(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.GetReportDetail(r.Context(), chi.URLParam(r, "reportId"))
	if err != nil {
		writeModerationError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) ResolveReport(w http.ResponseWriter, r *http.Request) {
	moderatorID, ok := auth.UserIDFromContext(r.Context())
	if !ok || moderatorID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var input ResolveInput
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	out, err := h.svc.ResolveReport(r.Context(), moderatorID, actorFromContext(r.Context()), chi.URLParam(r, "reportId"), input)
	if err != nil {
		writeModerationError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) Notifications(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	out, err := h.svc.Notifications(r.Context(), userID)
	if err != nil {
		writeModerationError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) UnreadNotificationCount(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	out, err := h.svc.UnreadNotificationCount(r.Context(), userID)
	if err != nil {
		writeModerationError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) MarkNotificationRead(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := h.svc.MarkNotificationRead(r.Context(), userID, chi.URLParam(r, "notificationId")); err != nil {
		writeModerationError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) MarkAllNotificationsRead(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := h.svc.MarkAllNotificationsRead(r.Context(), userID); err != nil {
		writeModerationError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func writeModerationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrSelfReport), errors.Is(err, ErrInvalidCategory), errors.Is(err, ErrInvalidAction):
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrDuplicateReport):
		httpx.WriteError(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrMessageNotAccessible):
		httpx.WriteError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, ErrModerationForbidden):
		httpx.WriteError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not found")
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "moderation request failed")
	}
}
