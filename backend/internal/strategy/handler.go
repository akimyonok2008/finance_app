package strategy

import (
	"errors"
	"net/http"

	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/httpx"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) CopyPreview(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req CopyPreviewRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	resp, err := h.svc.CopyPreview(r.Context(), userID, req.Handle)
	if err != nil {
		writeCopyError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) CopyFromProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req CopyFromProfileRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	resp, err := h.svc.CopyFromProfile(r.Context(), userID, req)
	if err != nil {
		writeCopyError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) CompareProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req CompareProfileRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	resp, err := h.svc.CompareProfile(r.Context(), userID, req.Handle)
	if err != nil {
		switch {
		case errors.Is(err, ErrEmptyPortfolio):
			httpx.WriteError(w, http.StatusBadRequest, "create a strategy baseline before comparing")
		case errors.Is(err, ErrNotFound):
			httpx.WriteError(w, http.StatusNotFound, "public profile not found")
		default:
			httpx.WriteError(w, http.StatusInternalServerError, "could not compare profile")
		}
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func writeCopyError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "public strategy not found")
	case errors.Is(err, ErrSelfCopy):
		httpx.WriteError(w, http.StatusBadRequest, "cannot copy your own strategy")
	case errors.Is(err, portfolio.ErrInvalidWeights):
		httpx.WriteError(w, http.StatusBadRequest, "invalid strategy weights")
	case errors.Is(err, portfolio.ErrInvalidAssetType),
		errors.Is(err, portfolio.ErrUnsupportedSymbol),
		errors.Is(err, portfolio.ErrUnsupportedCurrency):
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "could not copy strategy")
	}
}
