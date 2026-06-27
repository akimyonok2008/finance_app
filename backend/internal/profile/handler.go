package profile

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

func (h *Handler) GetMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	out, err := h.svc.GetMe(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not load profile")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var input UpdateInput
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	out, err := h.svc.UpdateMe(r.Context(), userID, input)
	if err != nil {
		switch {
		case errors.Is(err, ErrHandleExists):
			httpx.WriteError(w, http.StatusConflict, err.Error())
		case errors.Is(err, ErrNotFound):
			httpx.WriteError(w, http.StatusNotFound, "profile not found")
		case errors.Is(err, ErrInvalid):
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
		default:
			httpx.WriteError(w, http.StatusInternalServerError, "could not update profile")
		}
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) GetPublic(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.GetPublic(r.Context(), chi.URLParam(r, "handle"))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.WriteError(w, http.StatusNotFound, "profile not found")
		} else {
			httpx.WriteError(w, http.StatusInternalServerError, "could not load profile")
		}
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
