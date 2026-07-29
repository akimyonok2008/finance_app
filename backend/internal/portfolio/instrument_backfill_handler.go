package portfolio

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/httpx"
)

// ReconciliationView is the admin-facing projection of a queued legacy
// record. It never appears on any user-facing surface.
type ReconciliationView struct {
	ID         string `json:"id"`
	TableName  string `json:"table_name"`
	RecordID   string `json:"record_id"`
	Symbol     string `json:"symbol"`
	Evidence   string `json:"evidence"`
	Confidence string `json:"confidence"`
	CreatedAt  string `json:"created_at"`
}

func reconciliationView(item ReconciliationItem) ReconciliationView {
	return ReconciliationView{
		ID: item.ID, TableName: item.TableName, RecordID: item.RecordID,
		Symbol: item.Symbol, Evidence: item.Evidence, Confidence: item.Confidence,
		CreatedAt: item.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// ListPendingReconciliation handles GET /admin/identity-reconciliation.
func (h *Handler) ListPendingReconciliation(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.ListPendingIdentityReconciliation(r.Context(), 0)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to list reconciliation items")
		return
	}
	out := make([]ReconciliationView, 0, len(items))
	for _, item := range items {
		out = append(out, reconciliationView(item))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

type resolveReconciliationInput struct {
	InstrumentID string `json:"instrument_id"`
}

// ResolvePendingReconciliation handles
// POST /admin/identity-reconciliation/{id}/resolve.
func (h *Handler) ResolvePendingReconciliation(w http.ResponseWriter, r *http.Request) {
	adminID, ok := auth.UserIDFromContext(r.Context())
	if !ok || adminID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var input resolveReconciliationInput
	if err := httpx.DecodeJSON(r, &input); err != nil || input.InstrumentID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "instrument_id is required")
		return
	}
	err := h.svc.ResolveIdentityReconciliation(r.Context(), chi.URLParam(r, "id"), input.InstrumentID, adminID)
	if err != nil {
		if errors.Is(err, ErrReconciliationItemNotFound) {
			httpx.WriteError(w, http.StatusNotFound, "reconciliation item not found or already resolved")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "failed to resolve reconciliation item")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "resolved"})
}

// RejectPendingReconciliation handles
// POST /admin/identity-reconciliation/{id}/reject.
func (h *Handler) RejectPendingReconciliation(w http.ResponseWriter, r *http.Request) {
	adminID, ok := auth.UserIDFromContext(r.Context())
	if !ok || adminID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	err := h.svc.RejectIdentityReconciliation(r.Context(), chi.URLParam(r, "id"), adminID)
	if err != nil {
		if errors.Is(err, ErrReconciliationItemNotFound) {
			httpx.WriteError(w, http.StatusNotFound, "reconciliation item not found or already resolved")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "failed to reject reconciliation item")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "rejected"})
}
