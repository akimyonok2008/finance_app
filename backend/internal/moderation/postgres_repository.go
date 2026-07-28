package moderation

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

const reportColumns = `id, reporter_user_id, reported_user_id, message_id, category, explanation,
	status, created_at, reviewed_at, reviewer_id, decision, moderator_notes`

func scanReport(row pgx.Row) (Report, error) {
	var rep Report
	var decision, notes *string
	err := row.Scan(&rep.ID, &rep.ReporterUserID, &rep.ReportedUserID, &rep.MessageID,
		&rep.Category, &rep.Explanation, &rep.Status, &rep.CreatedAt,
		&rep.ReviewedAt, &rep.ReviewerID, &decision, &notes)
	if errors.Is(err, pgx.ErrNoRows) {
		return Report{}, ErrNotFound
	}
	if err != nil {
		return Report{}, err
	}
	if decision != nil {
		rep.Decision = *decision
	}
	if notes != nil {
		rep.ModeratorNotes = *notes
	}
	return rep, nil
}

func (r *PostgresRepository) CreateReport(ctx context.Context, rep Report) (Report, error) {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO user_reports (id, reporter_user_id, reported_user_id, message_id, category, explanation, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING `+reportColumns,
		rep.ID, rep.ReporterUserID, rep.ReportedUserID, rep.MessageID, rep.Category, rep.Explanation, rep.Status, rep.CreatedAt,
	)
	return scanReport(row)
}

func (r *PostgresRepository) GetReport(ctx context.Context, id string) (Report, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+reportColumns+` FROM user_reports WHERE id = $1`, id)
	return scanReport(row)
}

func (r *PostgresRepository) ListReports(ctx context.Context, status string) ([]Report, error) {
	var rows pgx.Rows
	var err error
	if status == "" {
		rows, err = r.pool.Query(ctx, `SELECT `+reportColumns+` FROM user_reports ORDER BY created_at DESC`)
	} else {
		rows, err = r.pool.Query(ctx, `SELECT `+reportColumns+` FROM user_reports WHERE status = $1 ORDER BY created_at DESC`, status)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Report, 0)
	for rows.Next() {
		rep, err := scanReport(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rep)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) HasOpenDuplicate(ctx context.Context, reporterID, reportedID string, messageID *string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM user_reports
			WHERE reporter_user_id = $1 AND reported_user_id = $2
			  AND status IN ('open', 'under_review')
			  AND COALESCE(message_id, '00000000-0000-0000-0000-000000000000') =
			      COALESCE($3::uuid, '00000000-0000-0000-0000-000000000000')
		)
	`, reporterID, reportedID, messageID).Scan(&exists)
	return exists, err
}

func (r *PostgresRepository) UpdateReport(ctx context.Context, rep Report) (Report, error) {
	row := r.pool.QueryRow(ctx, `
		UPDATE user_reports
		SET status = $2, reviewed_at = $3, reviewer_id = $4, decision = $5, moderator_notes = $6
		WHERE id = $1
		RETURNING `+reportColumns,
		rep.ID, rep.Status, rep.ReviewedAt, rep.ReviewerID, nullIfEmpty(rep.Decision), nullIfEmpty(rep.ModeratorNotes),
	)
	return scanReport(row)
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (r *PostgresRepository) SaveEvidence(ctx context.Context, e Evidence) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO report_evidence (id, report_id, message_id, conversation_id, sender_id, participant_ids, message_text, message_created_at, report_created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, e.ID, e.ReportID, e.MessageID, e.ConversationID, e.SenderID, e.ParticipantIDs, e.MessageText, e.MessageCreatedAt, e.ReportCreatedAt)
	return err
}

func (r *PostgresRepository) GetEvidence(ctx context.Context, reportID string) (Evidence, bool, error) {
	var e Evidence
	err := r.pool.QueryRow(ctx, `
		SELECT id, report_id, message_id, conversation_id, sender_id, participant_ids, message_text, message_created_at, report_created_at
		FROM report_evidence WHERE report_id = $1
	`, reportID).Scan(&e.ID, &e.ReportID, &e.MessageID, &e.ConversationID, &e.SenderID, &e.ParticipantIDs, &e.MessageText, &e.MessageCreatedAt, &e.ReportCreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Evidence{}, false, nil
	}
	if err != nil {
		return Evidence{}, false, err
	}
	return e, true, nil
}

func (r *PostgresRepository) RecordAction(ctx context.Context, a ModerationAction) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO moderation_actions (id, moderator_id, target_user_id, report_id, action_type, reason, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, a.ID, a.ModeratorID, a.TargetUserID, a.ReportID, a.ActionType, a.Reason, a.CreatedAt, a.ExpiresAt)
	return err
}

func (r *PostgresRepository) CreateNotification(ctx context.Context, n Notification) error {
	payload, err := json.Marshal(n.Payload)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO notifications (id, recipient_user_id, type, payload, dedupe_key, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (recipient_user_id, dedupe_key) DO NOTHING
	`, n.ID, n.RecipientID, n.Type, payload, n.DedupeKey, n.CreatedAt)
	return err
}

func (r *PostgresRepository) ListNotifications(ctx context.Context, recipientID string, limit int) ([]Notification, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, recipient_user_id, type, payload, dedupe_key, read_at, created_at
		FROM notifications WHERE recipient_user_id = $1 ORDER BY created_at DESC LIMIT $2
	`, recipientID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Notification, 0)
	for rows.Next() {
		var n Notification
		var payload []byte
		if err := rows.Scan(&n.ID, &n.RecipientID, &n.Type, &payload, &n.DedupeKey, &n.ReadAt, &n.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(payload, &n.Payload)
		out = append(out, n)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) UnreadNotificationCount(ctx context.Context, recipientID string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM notifications WHERE recipient_user_id = $1 AND read_at IS NULL
	`, recipientID).Scan(&count)
	return count, err
}

func (r *PostgresRepository) MarkNotificationRead(ctx context.Context, recipientID, notificationID string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE notifications SET read_at = now() WHERE id = $1 AND recipient_user_id = $2 AND read_at IS NULL
	`, notificationID, recipientID)
	return err
}

func (r *PostgresRepository) MarkAllNotificationsRead(ctx context.Context, recipientID string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE notifications SET read_at = now() WHERE recipient_user_id = $1 AND read_at IS NULL
	`, recipientID)
	return err
}
