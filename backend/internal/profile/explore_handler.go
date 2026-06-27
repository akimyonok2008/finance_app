package profile

import (
	"errors"
	"net/http"

	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/httpx"
)

// Explore handles GET /profiles/explore. It stays authenticated, consistent
// with the app's other protected social/portfolio screens.
func (h *Handler) Explore(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	filter, err := ParseExploreFilter(r.URL.Query().Get)
	if err != nil {
		if errors.Is(err, ErrInvalid) {
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
		} else {
			httpx.WriteError(w, http.StatusInternalServerError, "could not load explore page")
		}
		return
	}

	out, err := h.svc.Explore(r.Context(), userID, filter)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not load explore page")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
