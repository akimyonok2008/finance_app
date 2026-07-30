package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/ardakimyonok/finance_app/internal/httpx"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
)

// OutboxAdmin is the operator escape hatch for outbox events that exhausted
// their retry budget (see outboxMaxAttempts in the portfolio package): a
// permanently unpriceable symbol or a since-fixed provider outage otherwise
// has no way to be inspected or retried without direct database access.
type OutboxAdmin interface {
	ListDeadLetteredOutbox(ctx context.Context, limit int) ([]portfolio.OutboxEvent, error)
	RequeueOutboxEvent(ctx context.Context, id string) error
}

type deadLetterView struct {
	ID               string  `json:"id"`
	EventType        string  `json:"event_type"`
	UserID           string  `json:"user_id"`
	AggregateID      string  `json:"aggregate_id"`
	AggregateVersion int64   `json:"aggregate_version"`
	AttemptCount     int     `json:"attempt_count"`
	LastError        string  `json:"last_error"`
	CreatedAt        string  `json:"created_at"`
	DeadLetteredAt   *string `json:"dead_lettered_at,omitempty"`
}

// listDeadLettersHandler answers GET /outbox/dead-letters: the events the
// projector gave up on, most recently dead-lettered first.
func listDeadLettersHandler(admin OutboxAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		events, err := admin.ListDeadLetteredOutbox(ctx, 100)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "list_dead_letters_failed")
			return
		}
		views := make([]deadLetterView, 0, len(events))
		for _, ev := range events {
			view := deadLetterView{
				ID: ev.ID, EventType: string(ev.EventType), UserID: ev.UserID,
				AggregateID: ev.AggregateID, AggregateVersion: ev.AggregateVersion,
				AttemptCount: ev.AttemptCount, LastError: ev.LastError,
				CreatedAt: ev.CreatedAt.UTC().Format(time.RFC3339),
			}
			if ev.DeadLetteredAt != nil {
				s := ev.DeadLetteredAt.UTC().Format(time.RFC3339)
				view.DeadLetteredAt = &s
			}
			views = append(views, view)
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"dead_letters": views, "count": len(views)})
	}
}

// requeueDeadLetterHandler answers POST /outbox/dead-letters/{id}/requeue: it
// clears the dead-lettered state so the processor claims and retries the
// event again, for the case the underlying provider gap has since been fixed.
func requeueDeadLetterHandler(admin OutboxAdmin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if _, err := uuid.Parse(id); err != nil {
			// The store's underlying id column is UUID-typed: a malformed id
			// would otherwise surface as a database error, not "not found".
			httpx.WriteError(w, http.StatusNotFound, "not_dead_lettered")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		if err := admin.RequeueOutboxEvent(ctx, id); err != nil {
			if errors.Is(err, portfolio.ErrOutboxEventNotDeadLettered) {
				httpx.WriteError(w, http.StatusNotFound, "not_dead_lettered")
				return
			}
			httpx.WriteError(w, http.StatusInternalServerError, "requeue_failed")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "requeued", "id": id})
	}
}
