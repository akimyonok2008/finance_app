package moderation

import (
	"context"
	"time"
)

// ResolveWrite is everything a single ResolveReport call needs to persist,
// computed exactly once by the caller's ResolveBuilder while the report row
// is locked. Repository implementations persist Action, the account
// mutation implied by ActionType/SuspendUntil, Notification, and Report
// together, atomically.
type ResolveWrite struct {
	// Report carries the updated report fields (status, decision,
	// moderator_notes, reviewed_at, reviewer_id) to persist.
	Report Report
	// Action is the append-only audit record to insert.
	Action ModerationAction
	// ActionType mirrors Action.ActionType and tells the repository which
	// (if any) account-level side effect accompanies this write.
	ActionType string
	// SuspendUntil is set (only) when ActionType is a temporary suspension;
	// it is computed exactly once by the builder so retries never recompute
	// a later expiration.
	SuspendUntil *time.Time
	// Notification is optional; nil means no notification is created.
	Notification *Notification
	// ExternalApply performs any account mutation that must happen through
	// another package's boundary (e.g. auth.Service.Suspend/Ban). It runs
	// while the report row is still locked, after the audit/report/
	// notification writes are staged but before they are committed (for
	// Postgres) or made visible (for the in-memory repo), so a failure here
	// leaves no partial state and a concurrent resolver cannot race it.
	// May be nil.
	ExternalApply func(ctx context.Context) error
}

// ResolveBuilder is invoked by Repository.ResolveReport with the current,
// locked report row. It must not itself mutate persistent state; it only
// decides what to write. Returning an error aborts the whole resolution
// (nothing is persisted).
type ResolveBuilder func(ctx context.Context, current Report) (ResolveWrite, error)

// Repository is the persistence boundary for reports, evidence, moderation
// actions, and notifications.
type Repository interface {
	// CreateUserReport atomically re-checks the open-duplicate constraint and
	// inserts the report row, so a concurrent duplicate report cannot slip in
	// between the check and the insert (TOCTOU).
	CreateUserReport(ctx context.Context, r Report) (Report, error)
	// CreateMessageReport atomically re-checks the open-duplicate constraint
	// and inserts both the report row and its evidence snapshot together: if
	// the evidence write fails, the report insert is rolled back too.
	CreateMessageReport(ctx context.Context, r Report, e Evidence) (Report, error)

	GetReport(ctx context.Context, id string) (Report, error)
	ListReports(ctx context.Context, status string) ([]Report, error)
	// HasOpenDuplicate reports whether an open/under_review report already
	// exists for the same (reporter, reported user, message) triple.
	// messageID is nil for user reports. Exposed for read-only/UI use;
	// CreateUserReport/CreateMessageReport perform their own authoritative,
	// race-free check internally and do not rely on a prior call to this.
	HasOpenDuplicate(ctx context.Context, reporterID, reportedID string, messageID *string) (bool, error)

	GetEvidence(ctx context.Context, reportID string) (Evidence, bool, error)

	// ResolveReport locks the report row (or, for the in-memory repository,
	// takes an equivalent single critical section), and:
	//   - if the report is already resolved, returns (current, false, nil)
	//     without invoking build and without writing anything;
	//   - otherwise invokes build to compute the write, persists
	//     Action + the account mutation implied by ActionType/SuspendUntil +
	//     Notification + the updated Report atomically (all-or-nothing), and
	//     returns (updated, true, nil).
	// Two concurrent calls for the same reportID serialize: the loser sees
	// the row already resolved and takes the idempotent (false) path.
	ResolveReport(ctx context.Context, reportID string, build ResolveBuilder) (result Report, applied bool, err error)

	// CreateNotification is idempotent on (recipient_user_id, dedupe_key): a
	// retried event does not create a duplicate notification.
	CreateNotification(ctx context.Context, n Notification) error
	ListNotifications(ctx context.Context, recipientID string, limit int) ([]Notification, error)
	UnreadNotificationCount(ctx context.Context, recipientID string) (int, error)
	MarkNotificationRead(ctx context.Context, recipientID, notificationID string) error
	MarkAllNotificationsRead(ctx context.Context, recipientID string) error
}

// clockNow is overridable in tests.
var clockNow = func() time.Time { return time.Now().UTC() }
