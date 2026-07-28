package safety

import (
	"errors"
	"net/http"

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

func (h *Handler) Block(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	handle := chi.URLParam(r, "handle")
	if err := h.svc.Block(r.Context(), userID, handle); err != nil {
		writeSafetyError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, BlockStateResponse{Handle: handle, IsBlocked: true})
}

func (h *Handler) Unblock(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	handle := chi.URLParam(r, "handle")
	if err := h.svc.Unblock(r.Context(), userID, handle); err != nil {
		writeSafetyError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, BlockStateResponse{Handle: handle, IsBlocked: false})
}

func (h *Handler) BlockedUsers(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	out, err := h.svc.BlockedUsers(r.Context(), userID)
	if err != nil {
		writeSafetyError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func writeSafetyError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrSelfBlock), errors.Is(err, ErrInvalidHandle):
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not found")
	case errors.Is(err, ErrForbidden):
		httpx.WriteError(w, http.StatusForbidden, "forbidden")
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "request failed")
	}
}
