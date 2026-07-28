package moderation

import (
	"context"
	"time"
)

// Repository is the persistence boundary for reports, evidence, moderation
// actions, and notifications.
type Repository interface {
	CreateReport(ctx context.Context, r Report) (Report, error)
	GetReport(ctx context.Context, id string) (Report, error)
	ListReports(ctx context.Context, status string) ([]Report, error)
	// HasOpenDuplicate reports whether an open/under_review report already
	// exists for the same (reporter, reported user, message) triple.
	// messageID is nil for user reports.
	HasOpenDuplicate(ctx context.Context, reporterID, reportedID string, messageID *string) (bool, error)
	UpdateReport(ctx context.Context, r Report) (Report, error)

	SaveEvidence(ctx context.Context, e Evidence) error
	GetEvidence(ctx context.Context, reportID string) (Evidence, bool, error)

	RecordAction(ctx context.Context, a ModerationAction) error

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
