package social

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/httpx"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Follow(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	out, err := h.svc.Follow(r.Context(), userID, chi.URLParam(r, "handle"))
	if err != nil {
		writeSocialError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) Unfollow(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	out, err := h.svc.Unfollow(r.Context(), userID, chi.URLParam(r, "handle"))
	if err != nil {
		writeSocialError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) FollowState(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	out, err := h.svc.FollowState(r.Context(), userID, chi.URLParam(r, "handle"))
	if err != nil {
		writeSocialError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) Following(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	out, err := h.svc.Following(r.Context(), userID)
	if err != nil {
		writeSocialError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) Followers(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	out, err := h.svc.Followers(r.Context(), userID)
	if err != nil {
		writeSocialError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) Friends(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	out, err := h.svc.Friends(r.Context(), userID)
	if err != nil {
		writeSocialError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) Conversations(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	out, err := h.svc.Conversations(r.Context(), userID)
	if err != nil {
		writeSocialError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) CreateConversation(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var input CreateConversationInput
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	out, err := h.svc.CreateConversation(r.Context(), userID, input.Handle)
	if err != nil {
		writeSocialError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) Messages(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	out, err := h.svc.Messages(r.Context(), userID, chi.URLParam(r, "conversationId"), limit)
	if err != nil {
		writeSocialError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) SendMessage(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var input SendMessageInput
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	out, err := h.svc.SendMessage(r.Context(), userID, chi.URLParam(r, "conversationId"), input.Body)
	if err != nil {
		writeSocialError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) HideMessage(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := h.svc.HideMessage(r.Context(), userID, chi.URLParam(r, "messageId")); err != nil {
		writeSocialError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) MarkRead(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var input MarkReadInput
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.svc.MarkRead(r.Context(), userID, chi.URLParam(r, "conversationId"), input.LastReadMessageID); err != nil {
		writeSocialError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) UnreadCount(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	out, err := h.svc.UnreadCount(r.Context(), userID)
	if err != nil {
		writeSocialError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func writeSocialError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidHandle), errors.Is(err, ErrInvalidMessage), errors.Is(err, ErrMessageTooLong), errors.Is(err, ErrSelfFollow), errors.Is(err, ErrSelfDM):
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrNotFriends):
		httpx.WriteError(w, http.StatusForbidden, "You can only message mutual friends.")
	case errors.Is(err, ErrInteractionBlocked):
		httpx.WriteError(w, http.StatusForbidden, "This action isn't available for this account.")
	case errors.Is(err, ErrForbidden):
		httpx.WriteError(w, http.StatusForbidden, "forbidden")
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrConversationNotFound), errors.Is(err, ErrMessageNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not found")
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "social request failed")
	}
}
