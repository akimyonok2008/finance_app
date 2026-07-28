package moderation

import (
	"context"
	"sort"
	"sync"

	"github.com/google/uuid"
)

type InMemoryRepository struct {
	mu            sync.RWMutex
	reports       map[string]Report
	evidence      map[string]Evidence // keyed by reportID
	actions       []ModerationAction
	notifications map[string]Notification
	dedupe        map[string]string // recipientID|dedupeKey -> notificationID
}

func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		reports:       map[string]Report{},
		evidence:      map[string]Evidence{},
		notifications: map[string]Notification{},
		dedupe:        map[string]string{},
	}
}

func (r *InMemoryRepository) CreateReport(_ context.Context, rep Report) (Report, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reports[rep.ID] = rep
	return rep, nil
}

func (r *InMemoryRepository) GetReport(_ context.Context, id string) (Report, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rep, ok := r.reports[id]
	if !ok {
		return Report{}, ErrNotFound
	}
	return rep, nil
}

func (r *InMemoryRepository) ListReports(_ context.Context, status string) ([]Report, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Report, 0)
	for _, rep := range r.reports {
		if status == "" || rep.Status == status {
			out = append(out, rep)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (r *InMemoryRepository) HasOpenDuplicate(_ context.Context, reporterID, reportedID string, messageID *string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, rep := range r.reports {
		if rep.Status != StatusOpen && rep.Status != StatusUnderReview {
			continue
		}
		if rep.ReporterUserID != reporterID || rep.ReportedUserID != reportedID {
			continue
		}
		if samePtr(rep.MessageID, messageID) {
			return true, nil
		}
	}
	return false, nil
}

func samePtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func (r *InMemoryRepository) UpdateReport(_ context.Context, rep Report) (Report, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.reports[rep.ID]; !ok {
		return Report{}, ErrNotFound
	}
	r.reports[rep.ID] = rep
	return rep, nil
}

func (r *InMemoryRepository) SaveEvidence(_ context.Context, e Evidence) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.evidence[e.ReportID] = e
	return nil
}

func (r *InMemoryRepository) GetEvidence(_ context.Context, reportID string) (Evidence, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.evidence[reportID]
	return e, ok, nil
}

func (r *InMemoryRepository) RecordAction(_ context.Context, a ModerationAction) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.actions = append(r.actions, a)
	return nil
}

func (r *InMemoryRepository) CreateNotification(_ context.Context, n Notification) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := n.RecipientID + "|" + n.DedupeKey
	if _, exists := r.dedupe[key]; exists {
		return nil
	}
	if n.ID == "" {
		n.ID = uuid.NewString()
	}
	r.notifications[n.ID] = n
	r.dedupe[key] = n.ID
	return nil
}

func (r *InMemoryRepository) ListNotifications(_ context.Context, recipientID string, limit int) ([]Notification, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Notification, 0)
	for _, n := range r.notifications {
		if n.RecipientID == recipientID {
			out = append(out, n)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *InMemoryRepository) UnreadNotificationCount(_ context.Context, recipientID string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	count := 0
	for _, n := range r.notifications {
		if n.RecipientID == recipientID && n.ReadAt == nil {
			count++
		}
	}
	return count, nil
}

func (r *InMemoryRepository) MarkNotificationRead(_ context.Context, recipientID, notificationID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	n, ok := r.notifications[notificationID]
	if !ok || n.RecipientID != recipientID {
		return ErrNotFound
	}
	if n.ReadAt == nil {
		now := clockNow()
		n.ReadAt = &now
		r.notifications[notificationID] = n
	}
	return nil
}

func (r *InMemoryRepository) MarkAllNotificationsRead(_ context.Context, recipientID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := clockNow()
	for id, n := range r.notifications {
		if n.RecipientID == recipientID && n.ReadAt == nil {
			n.ReadAt = &now
			r.notifications[id] = n
		}
	}
	return nil
}
