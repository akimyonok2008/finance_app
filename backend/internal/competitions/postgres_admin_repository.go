package competitions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresCompetitionAdminRepository struct{ pool *pgxpool.Pool }

func NewPostgresCompetitionAdminRepository(pool *pgxpool.Pool) *PostgresCompetitionAdminRepository {
	return &PostgresCompetitionAdminRepository{pool: pool}
}

func (r *PostgresCompetitionAdminRepository) RecordAdminAudit(ctx context.Context, a AdminAuditRecord) error {
	return insertAdminAudit(ctx, r.pool, a)
}

type adminExecer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func insertAdminAudit(ctx context.Context, q adminExecer, a AdminAuditRecord) error {
	_, err := q.Exec(ctx, `
		INSERT INTO competition_admin_audit
			(id, actor_user_id, action, target_type, target_id, competition_id,
			 request_id, reason, details_json, succeeded, error_message, created_at)
		VALUES ($1,$2,$3,$4,$5,NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),$9,$10,NULLIF($11,''),$12)
	`, a.ID, a.ActorUserID, a.Action, a.TargetType, a.TargetID, a.CompetitionID,
		a.RequestID, a.Reason, a.Details, a.Succeeded, a.ErrorMessage, a.CreatedAt)
	if err != nil {
		return fmt.Errorf("competition admin: record audit: %w", err)
	}
	return nil
}

func (r *PostgresCompetitionAdminRepository) CreateDefinitionWithAudit(ctx context.Context, d Definition, a AdminAuditRecord) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `INSERT INTO competition_definitions
		(id,slug,name,description,category,icon_key,presentation_config_json,is_enabled)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, d.ID, d.Slug, d.Name, d.Description, d.Category, d.IconKey, d.PresentationConfigJSON, d.IsEnabled)
	if err != nil {
		var pe *pgconn.PgError
		if errors.As(err, &pe) && pe.Code == "23505" {
			return ErrDefinitionExists
		}
		return err
	}
	if err = insertAdminAudit(ctx, tx, a); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresCompetitionAdminRepository) CreateVersionWithAudit(ctx context.Context, v DefinitionVersion, a AdminAuditRecord) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var current int64
	err = tx.QueryRow(ctx, `SELECT current_version FROM competition_definitions WHERE id=$1 FOR UPDATE`, v.DefinitionID).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrDefinitionNotFound
	}
	if err != nil {
		return err
	}
	if v.Version <= current {
		return ErrDefinitionVersionExists
	}
	if v.Version != current+1 {
		return fmt.Errorf("definition repository: versions are append-only: got %d, next is %d", v.Version, current+1)
	}
	_, err = tx.Exec(ctx, `INSERT INTO competition_definition_versions
		(definition_id,version,eligibility_rules_json,scoring_rules_json,schedule_defaults_json,display_rules_json,created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`, v.DefinitionID, v.Version, v.EligibilityRulesJSON, v.ScoringRulesJSON, v.ScheduleDefaultsJSON, v.DisplayRulesJSON, v.CreatedBy)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE competition_definitions SET current_version=$1,updated_at=now() WHERE id=$2`, v.Version, v.DefinitionID); err != nil {
		return err
	}
	if err = insertAdminAudit(ctx, tx, a); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresCompetitionAdminRepository) CreateEditionWithAudit(ctx context.Context, e Competition, a AdminAuditRecord) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `INSERT INTO competitions
		(id,name,type,starts_at,ends_at,status,created_at,definition_id,definition_version,join_opens_at,join_closes_at,lifecycle_status,rules_snapshot_json,scoring_scope)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) ON CONFLICT (id) DO NOTHING`,
		e.ID, e.Name, e.Type, e.StartsAt, e.EndsAt, e.LifecycleStatus, e.CreatedAt, e.DefinitionID, e.DefinitionVersion, e.JoinOpensAt, e.JoinClosesAt, e.LifecycleStatus, e.RulesSnapshotJSON, e.ScoringScope)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrEditionExists
	}
	if err = insertAdminAudit(ctx, tx, a); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresCompetitionAdminRepository) TransitionEditionWithAudit(ctx context.Context, id, from, to string, now time.Time, a AdminAuditRecord) error {
	if err := ValidateLifecycleTransition(from, to); err != nil {
		return err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	query := `UPDATE competitions SET lifecycle_status=$1,status=$1`
	args := []any{to, id, from}
	if col, ok := lifecycleStampColumns[to]; ok {
		query += `, ` + col + `=$4`
		args = append(args, now.UTC())
	}
	query += ` WHERE id=$2 AND lifecycle_status=$3`
	tag, err := tx.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrLifecycleConflict
	}
	if err = insertAdminAudit(ctx, tx, a); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresCompetitionAdminRepository) RequeueAchievementProjectionWithAudit(ctx context.Context, competitionID string, a AdminAuditRecord) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `UPDATE competition_outbox SET attempt_count=0,claimed_at=NULL,next_attempt_at=NULL,dead_lettered_at=NULL,last_error=NULL
		WHERE competition_id=$1 AND processed_at IS NULL AND dead_lettered_at IS NOT NULL`, competitionID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("competition achievement projection has no dead-lettered event")
	}
	if err = insertAdminAudit(ctx, tx, a); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresCompetitionAdminRepository) OperationalStatus(ctx context.Context, competitionID string) (*AdminOperationalStatus, error) {
	status := &AdminOperationalStatus{
		EntryStatusCounts: map[string]int{},
		BaselineCounts:    map[string]int{},
	}
	comp, err := NewPostgresCompetitionRepository(r.pool).GetCompetition(ctx, competitionID)
	if err != nil {
		return nil, err
	}
	status.Competition = *comp

	rows, err := r.pool.Query(ctx, `
		SELECT entry_status, COALESCE(baseline_status, ''), count(*)
		FROM competition_entries WHERE competition_id=$1
		GROUP BY entry_status, baseline_status
	`, competitionID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var entry, baseline string
		var count int
		if err := rows.Scan(&entry, &baseline, &count); err != nil {
			rows.Close()
			return nil, err
		}
		status.EntryStatusCounts[entry] += count
		if baseline != "" {
			status.BaselineCounts[baseline] += count
		}
	}
	rows.Close()

	rows, err = r.pool.Query(ctx, `
		SELECT id::text, entry_status, COALESCE(disqualification_reason,'')
		FROM competition_entries
		WHERE competition_id=$1 AND (entry_status IN ('baseline_failed','disqualified') OR disqualification_reason IS NOT NULL)
		ORDER BY joined_at, id
	`, competitionID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var d AdminFailureDetail
		if err := rows.Scan(&d.EntryID, &d.Status, &d.Reason); err != nil {
			rows.Close()
			return nil, err
		}
		status.Disqualifications = append(status.Disqualifications, d)
	}
	rows.Close()

	rows, err = r.pool.Query(ctx, `
		SELECT competition_id, generation, status, expected_entries, processed_entries,
		       successful_entries, failed_entries, COALESCE(cursor_entry_id::text,''),
		       write_failure, started_at, completed_at, activated_at, COALESCE(failure_reason,'')
		FROM competition_ranking_generations WHERE competition_id=$1
		ORDER BY generation DESC LIMIT 20
	`, competitionID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var g RankingGeneration
		if err := rows.Scan(&g.CompetitionID, &g.Generation, &g.Status, &g.ExpectedEntries,
			&g.ProcessedEntries, &g.SuccessfulEntries, &g.FailedEntries, &g.CursorEntryID,
			&g.WriteFailure, &g.StartedAt, &g.CompletedAt, &g.ActivatedAt, &g.FailureReason); err != nil {
			rows.Close()
			return nil, err
		}
		status.RankingFailures = append(status.RankingFailures, g)
	}
	rows.Close()

	rows, err = r.pool.Query(ctx, `
		SELECT s.boundary_type, s.observation_status, s.effective_at, s.completed_at,
		       (SELECT count(*) FROM competition_price_observations p WHERE p.observation_set_id=s.id),
		       (SELECT count(*) FROM competition_fx_observations f WHERE f.observation_set_id=s.id)
		FROM competition_observation_sets s WHERE s.competition_id=$1 ORDER BY s.boundary_type
	`, competitionID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var o AdminObservationStatus
		if err := rows.Scan(&o.BoundaryType, &o.Status, &o.EffectiveAt, &o.CompletedAt, &o.PriceCount, &o.FXCount); err != nil {
			rows.Close()
			return nil, err
		}
		status.ObservationSets = append(status.ObservationSets, o)
	}
	rows.Close()
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM competition_results WHERE competition_id=$1`, competitionID).Scan(&status.ResultCount); err != nil {
		return nil, err
	}
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE dead_lettered_at IS NULL), count(*) FILTER (WHERE dead_lettered_at IS NOT NULL)
		FROM competition_outbox WHERE competition_id=$1 AND processed_at IS NULL`, competitionID).Scan(&status.AchievementProjectionPending, &status.AchievementProjectionDeadLetters); err != nil {
		return nil, err
	}
	rows, err = r.pool.Query(ctx, `SELECT id::text,attempt_count,COALESCE(last_error,''),dead_lettered_at
		FROM competition_outbox WHERE competition_id=$1 AND dead_lettered_at IS NOT NULL ORDER BY dead_lettered_at DESC`, competitionID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var failure AdminProjectionFailure
		if err := rows.Scan(&failure.EventID, &failure.AttemptCount, &failure.LastError, &failure.DeadLetteredAt); err != nil {
			rows.Close()
			return nil, err
		}
		status.AchievementProjectionFailures = append(status.AchievementProjectionFailures, failure)
	}
	rows.Close()
	return status, nil
}

func (r *PostgresCompetitionAdminRepository) ListAdminAudit(ctx context.Context, competitionID string, limit int) ([]AdminAuditRecord, error) {
	query := `
		SELECT id::text, COALESCE(actor_user_id::text,''), action, target_type, target_id,
		       COALESCE(competition_id,''), COALESCE(request_id,''), COALESCE(reason,''),
		       details_json, succeeded, COALESCE(error_message,''), created_at
		FROM competition_admin_audit`
	args := []any{}
	if competitionID != "" {
		query += ` WHERE competition_id=$1`
		args = append(args, competitionID)
	}
	query += fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d`, len(args)+1)
	args = append(args, limit)
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AdminAuditRecord
	for rows.Next() {
		var a AdminAuditRecord
		var details []byte
		if err := rows.Scan(&a.ID, &a.ActorUserID, &a.Action, &a.TargetType, &a.TargetID,
			&a.CompetitionID, &a.RequestID, &a.Reason, &details, &a.Succeeded,
			&a.ErrorMessage, &a.CreatedAt); err != nil {
			return nil, err
		}
		a.Details = json.RawMessage(details)
		out = append(out, a)
	}
	return out, rows.Err()
}

var _ CompetitionAdminRepository = (*PostgresCompetitionAdminRepository)(nil)
var _ AtomicCompetitionAdminRepository = (*PostgresCompetitionAdminRepository)(nil)
