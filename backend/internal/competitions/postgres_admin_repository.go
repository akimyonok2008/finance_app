package competitions

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresCompetitionAdminRepository struct{ pool *pgxpool.Pool }

func NewPostgresCompetitionAdminRepository(pool *pgxpool.Pool) *PostgresCompetitionAdminRepository {
	return &PostgresCompetitionAdminRepository{pool: pool}
}

func (r *PostgresCompetitionAdminRepository) RecordAdminAudit(ctx context.Context, a AdminAuditRecord) error {
	_, err := r.pool.Exec(ctx, `
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
