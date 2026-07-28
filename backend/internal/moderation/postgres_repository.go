package moderation

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

const reportColumns = `id, reporter_user_id, reported_user_id, message_id, category, explanation,
	status, created_at, reviewed_at, reviewer_id, decision, moderator_notes,
	reporter_subject_id, reported_subject_id`

// scanReport reads a user_reports row. reporter_user_id/reported_user_id are
// nullable (migration 0028_moderation_retention.sql: SET NULL on account
// deletion instead of CASCADE); a NULL there maps to Report.ReporterUserID/
// ReportedUserID == "" with the corresponding *Deleted flag set, rather than
// erroring or omitting the row.
func scanReport(row pgx.Row) (Report, error) {
	var rep Report
	var decision, notes, reporterID, reportedID *string
	err := row.Scan(&rep.ID, &reporterID, &reportedID, &rep.MessageID,
		&rep.Category, &rep.Explanation, &rep.Status, &rep.CreatedAt,
		&rep.ReviewedAt, &rep.ReviewerID, &decision, &notes,
		&rep.reporterSubjectID, &rep.reportedSubjectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Report{}, ErrNotFound
	}
	if err != nil {
		return Report{}, err
	}
	if reporterID != nil {
		rep.ReporterUserID = *reporterID
	} else {
		rep.ReporterDeleted = true
	}
	if reportedID != nil {
		rep.ReportedUserID = *reportedID
	} else {
		rep.ReportedDeleted = true
	}
	if decision != nil {
		rep.Decision = *decision
	}
	if notes != nil {
		rep.ModeratorNotes = *notes
	}
	return rep, nil
}

// subjectID upserts (user_id) into moderation_subject_ids inside tx and
// returns its (possibly pre-existing) opaque subject_id. Using
// ON CONFLICT ... DO UPDATE (a harmless no-op reassignment) rather than DO
// NOTHING is required to make RETURNING fire on the already-present case too.
func subjectID(ctx context.Context, tx pgx.Tx, userID string) (string, error) {
	var sid string
	err := tx.QueryRow(ctx, `
		INSERT INTO moderation_subject_ids (user_id) VALUES ($1)
		ON CONFLICT (user_id) DO UPDATE SET user_id = EXCLUDED.user_id
		RETURNING subject_id
	`, userID).Scan(&sid)
	return sid, err
}

// isUniqueViolation reports whether err is a Postgres unique_violation
// (SQLSTATE 23505), the constraint-level backstop for the
// user_reports_open_dedupe_idx partial unique index.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// createReportTx is the shared implementation behind CreateUserReport and
// CreateMessageReport: it takes an advisory transaction lock keyed on the
// unordered-by-role (reporter, reported) pair, re-checks the open-duplicate
// constraint inside the transaction (closing the TOCTOU window a bare
// check-then-insert would have), inserts the report, optionally inserts the
// evidence row in the same transaction, and commits both together — the
// unique index is also checked as a backstop in case of a concurrent insert
// this connection's lock did not observe.
func (r *PostgresRepository) createReportTx(ctx context.Context, rep Report, ev *Evidence) (Report, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Report{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1), hashtext($2))`,
		rep.ReporterUserID, rep.ReportedUserID); err != nil {
		return Report{}, err
	}

	var exists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM user_reports
			WHERE reporter_user_id = $1 AND reported_user_id = $2
			  AND status IN ('open', 'under_review')
			  AND COALESCE(message_id, '00000000-0000-0000-0000-000000000000') =
			      COALESCE($3::uuid, '00000000-0000-0000-0000-000000000000')
		)
	`, rep.ReporterUserID, rep.ReportedUserID, rep.MessageID).Scan(&exists); err != nil {
		return Report{}, err
	}
	if exists {
		return Report{}, ErrDuplicateReport
	}

	reporterSubject, err := subjectID(ctx, tx, rep.ReporterUserID)
	if err != nil {
		return Report{}, err
	}
	reportedSubject, err := subjectID(ctx, tx, rep.ReportedUserID)
	if err != nil {
		return Report{}, err
	}

	row := tx.QueryRow(ctx, `
		INSERT INTO user_reports (id, reporter_user_id, reported_user_id, message_id, category, explanation, status, created_at, reporter_subject_id, reported_subject_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING `+reportColumns,
		rep.ID, rep.ReporterUserID, rep.ReportedUserID, rep.MessageID, rep.Category, rep.Explanation, rep.Status, rep.CreatedAt,
		reporterSubject, reportedSubject,
	)
	saved, err := scanReport(row)
	if err != nil {
		if isUniqueViolation(err) {
			return Report{}, ErrDuplicateReport
		}
		return Report{}, err
	}

	if ev != nil {
		ev.ReportID = saved.ID
		if _, err := tx.Exec(ctx, `
			INSERT INTO report_evidence (id, report_id, message_id, conversation_id, sender_id, participant_ids, message_text, message_created_at, report_created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`, ev.ID, ev.ReportID, ev.MessageID, ev.ConversationID, ev.SenderID, ev.ParticipantIDs, ev.MessageText, ev.MessageCreatedAt, ev.ReportCreatedAt); err != nil {
			// Evidence insert failed: the whole transaction (including the
			// report insert above) rolls back via the deferred Rollback, so
			// no evidence-less report is left behind.
			return Report{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Report{}, err
	}
	return saved, nil
}

func (r *PostgresRepository) CreateUserReport(ctx context.Context, rep Report) (Report, error) {
	return r.createReportTx(ctx, rep, nil)
}

func (r *PostgresRepository) CreateMessageReport(ctx context.Context, rep Report, ev Evidence) (Report, error) {
	return r.createReportTx(ctx, rep, &ev)
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

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
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

// ResolveReport locks the report row with SELECT ... FOR UPDATE and, if it
// is not already resolved, persists the audit action, the report update,
// and the notification in the same transaction as build's ExternalApply
// side effect (the account mutation, e.g. suspend/ban, which lives behind
// another package's own pool and so cannot literally join this pgx.Tx —
// running it here, while the row lock is held and immediately before
// commit, keeps the unsafe window to a single round trip instead of the
// whole resolution). If anything fails, the transaction rolls back and the
// report is left exactly as it was; a concurrent resolver blocked on the
// row lock will then see it still unresolved and can retry safely. If the
// report is already resolved, build is never invoked and no write happens.
func (r *PostgresRepository) ResolveReport(ctx context.Context, reportID string, build ResolveBuilder) (Report, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Report{}, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	row := tx.QueryRow(ctx, `SELECT `+reportColumns+` FROM user_reports WHERE id = $1 FOR UPDATE`, reportID)
	current, err := scanReport(row)
	if err != nil {
		return Report{}, false, err
	}

	if current.Status == StatusResolvedActionTaken || current.Status == StatusResolvedNoAction {
		return current, false, nil
	}

	write, err := build(ctx, current)
	if err != nil {
		return Report{}, false, err
	}

	moderatorSubject, err := subjectID(ctx, tx, write.Action.ModeratorID)
	if err != nil {
		return Report{}, false, err
	}
	targetSubject, err := subjectID(ctx, tx, write.Action.TargetUserID)
	if err != nil {
		return Report{}, false, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO moderation_actions (id, moderator_id, target_user_id, report_id, action_type, reason, created_at, expires_at, moderator_subject_id, target_subject_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, write.Action.ID, write.Action.ModeratorID, write.Action.TargetUserID, write.Action.ReportID,
		write.Action.ActionType, write.Action.Reason, write.Action.CreatedAt, write.Action.ExpiresAt,
		moderatorSubject, targetSubject); err != nil {
		return Report{}, false, err
	}

	if write.Notification != nil {
		payload, err := json.Marshal(write.Notification.Payload)
		if err != nil {
			return Report{}, false, err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO notifications (id, recipient_user_id, type, payload, dedupe_key, created_at)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (recipient_user_id, dedupe_key) DO NOTHING
		`, write.Notification.ID, write.Notification.RecipientID, write.Notification.Type, payload,
			write.Notification.DedupeKey, write.Notification.CreatedAt); err != nil {
			return Report{}, false, err
		}
	}

	updated := write.Report
	row2 := tx.QueryRow(ctx, `
		UPDATE user_reports
		SET status = $2, reviewed_at = $3, reviewer_id = $4, decision = $5, moderator_notes = $6
		WHERE id = $1
		RETURNING `+reportColumns,
		updated.ID, updated.Status, updated.ReviewedAt, updated.ReviewerID, nullIfEmpty(updated.Decision), nullIfEmpty(updated.ModeratorNotes),
	)
	final, err := scanReport(row2)
	if err != nil {
		return Report{}, false, err
	}

	if write.ExternalApply != nil {
		if err := write.ExternalApply(ctx); err != nil {
			return Report{}, false, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Report{}, false, err
	}
	return final, true, nil
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
