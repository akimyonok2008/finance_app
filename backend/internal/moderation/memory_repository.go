package moderation

import (
	"context"
	"sort"
	"sync"

	"github.com/google/uuid"
)

type InMemoryRepository struct {
	mu            sync.Mutex
	reports       map[string]Report
	evidence      map[string]Evidence // keyed by reportID
	actions       []ModerationAction
	notifications map[string]Notification
	dedupe        map[string]string // recipientID|dedupeKey -> notificationID

	// failHook, if set, is invoked at each write stage of the atomic
	// operations below ("duplicate_check", "report_insert",
	// "evidence_insert", "action_insert", "external_apply",
	// "notification_insert", "report_update"). Returning a non-nil error
	// aborts the whole operation before any state is mutated for that call,
	// simulating a mid-write failure for tests that verify no partial state
	// is left behind.
	failHook func(stage string) error
}

func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		reports:       map[string]Report{},
		evidence:      map[string]Evidence{},
		notifications: map[string]Notification{},
		dedupe:        map[string]string{},
	}
}

// SetFailHook installs a test-only failure injection hook. Pass nil to clear
// it. Not safe to call concurrently with in-flight requests against the repo.
func (r *InMemoryRepository) SetFailHook(f func(stage string) error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failHook = f
}

func (r *InMemoryRepository) fail(stage string) error {
	if r.failHook == nil {
		return nil
	}
	return r.failHook(stage)
}

func (r *InMemoryRepository) hasOpenDuplicateLocked(reporterID, reportedID string, messageID *string) bool {
	for _, rep := range r.reports {
		if rep.Status != StatusOpen && rep.Status != StatusUnderReview {
			continue
		}
		if rep.ReporterUserID != reporterID || rep.ReportedUserID != reportedID {
			continue
		}
		if samePtr(rep.MessageID, messageID) {
			return true
		}
	}
	return false
}

// CreateUserReport inserts a user report under a single critical section
// that re-checks the open-duplicate constraint immediately before the
// insert, closing the TOCTOU window a separate check-then-insert would have.
func (r *InMemoryRepository) CreateUserReport(_ context.Context, rep Report) (Report, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.fail("duplicate_check"); err != nil {
		return Report{}, err
	}
	if r.hasOpenDuplicateLocked(rep.ReporterUserID, rep.ReportedUserID, rep.MessageID) {
		return Report{}, ErrDuplicateReport
	}
	if err := r.fail("report_insert"); err != nil {
		return Report{}, err
	}
	r.reports[rep.ID] = rep
	return rep, nil
}

// CreateMessageReport inserts a message report and its evidence atomically:
// if the evidence write fails (or the injected failure hook fires), the
// report insert is rolled back too, leaving no evidence-less report behind.
func (r *InMemoryRepository) CreateMessageReport(_ context.Context, rep Report, ev Evidence) (Report, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.fail("duplicate_check"); err != nil {
		return Report{}, err
	}
	if r.hasOpenDuplicateLocked(rep.ReporterUserID, rep.ReportedUserID, rep.MessageID) {
		return Report{}, ErrDuplicateReport
	}
	if err := r.fail("report_insert"); err != nil {
		return Report{}, err
	}
	if err := r.fail("evidence_insert"); err != nil {
		// Neither the report nor the evidence is committed to state.
		return Report{}, err
	}
	r.reports[rep.ID] = rep
	r.evidence[rep.ID] = ev
	return rep, nil
}

func (r *InMemoryRepository) GetReport(_ context.Context, id string) (Report, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rep, ok := r.reports[id]
	if !ok {
		return Report{}, ErrNotFound
	}
	return rep, nil
}

func (r *InMemoryRepository) ListReports(_ context.Context, status string) ([]Report, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
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
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.hasOpenDuplicateLocked(reporterID, reportedID, messageID), nil
}

func samePtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func (r *InMemoryRepository) GetEvidence(_ context.Context, reportID string) (Evidence, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.evidence[reportID]
	return e, ok, nil
}

// ResolveReport takes the repository's single mutex as its critical section
// (the in-memory equivalent of a `SELECT ... FOR UPDATE` row lock), so two
// concurrent resolutions for the same report fully serialize: the loser
// observes the report already resolved and takes the idempotent path
// without invoking build again or double-applying any action.
func (r *InMemoryRepository) ResolveReport(ctx context.Context, reportID string, build ResolveBuilder) (Report, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	current, ok := r.reports[reportID]
	if !ok {
		return Report{}, false, ErrNotFound
	}
	if current.Status == StatusResolvedActionTaken || current.Status == StatusResolvedNoAction {
		return current, false, nil
	}

	write, err := build(ctx, current)
	if err != nil {
		return Report{}, false, err
	}

	if err := r.fail("action_insert"); err != nil {
		return Report{}, false, err
	}
	// Stage the action locally; only appended to r.actions once every later
	// stage (including VolatileApply) has succeeded, so an injected or real
	// failure at any point leaves the actions slice untouched.
	pendingAction := write.Action

	if write.Notification != nil {
		if err := r.fail("notification_insert"); err != nil {
			return Report{}, false, err
		}
	}

	if err := r.fail("report_update"); err != nil {
		return Report{}, false, err
	}

	// VolatileApply is the last fallible operation. Once it succeeds, publishing
	// the already-staged in-memory maps/slices below cannot fail, so no later
	// error can leave the mirrored account/content effect on its own.
	if write.VolatileApply != nil {
		if err := r.fail("external_apply"); err != nil {
			return Report{}, false, err
		}
		if err := write.VolatileApply(ctx); err != nil {
			return Report{}, false, err
		}
	}

	// All checks passed: publish every mutation together.
	r.actions = append(r.actions, pendingAction)
	if write.Notification != nil {
		n := *write.Notification
		key := n.RecipientID + "|" + n.DedupeKey
		if _, exists := r.dedupe[key]; !exists {
			if n.ID == "" {
				n.ID = uuid.NewString()
			}
			r.notifications[n.ID] = n
			r.dedupe[key] = n.ID
		}
	}
	r.reports[write.Report.ID] = write.Report
	return write.Report, true, nil
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
	r.mu.Lock()
	defer r.mu.Unlock()
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
	r.mu.Lock()
	defer r.mu.Unlock()
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
