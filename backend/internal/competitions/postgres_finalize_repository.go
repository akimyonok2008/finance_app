package competitions

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Postgres implementation of FinalizationRepository (table
// competition_results, migration 0048).

var _ FinalizationRepository = (*PostgresCompetitionRepository)(nil)

func (r *PostgresCompetitionRepository) ListActiveEntriesForFinalization(ctx context.Context, competitionID string) ([]CompetitionEntry, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, competition_id, user_id, eligible_starting_value_base
		FROM competition_entries
		WHERE competition_id = $1 AND entry_status = $2
		ORDER BY id
	`, competitionID, EntryActive)
	if err != nil {
		return nil, fmt.Errorf("competition repository: list active entries for finalization: %w", err)
	}
	defer rows.Close()

	var out []CompetitionEntry
	for rows.Next() {
		var e CompetitionEntry
		if err := rows.Scan(&e.ID, &e.CompetitionID, &e.UserID, &e.EligibleStartingValueBase); err != nil {
			return nil, fmt.Errorf("competition repository: scan finalization entry: %w", err)
		}
		e.EntryStatus = EntryActive
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if err := r.loadEngineSnapshots(ctx, &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (r *PostgresCompetitionRepository) DisqualifyEntry(ctx context.Context, entryID, reason string, _ time.Time) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE competition_entries
		SET entry_status = $1, disqualification_reason = $2
		WHERE id = $3 AND entry_status = $4
	`, EntryDisqualified, reason, entryID, EntryActive)
	if err != nil {
		return fmt.Errorf("competition repository: disqualify entry: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrEntryConflict
	}
	return nil
}

// FinalizeResults atomically writes the immutable result rows and transitions
// the edition finalizing -> completed. Locks the competition row FOR UPDATE
// first: if it is already 'completed', this is treated as an idempotent
// no-op success (a worker restart or retry replaying the same finalization)
// rather than an error, so callers never need to distinguish "just finalized"
// from "already finalized" — both simply mean "done".
func (r *PostgresCompetitionRepository) FinalizeResults(ctx context.Context, competitionID string, results []CompetitionResultRow, now time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("competition repository: begin finalize tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var lifecycle string
	if err := tx.QueryRow(ctx, `
		SELECT lifecycle_status FROM competitions WHERE id = $1 FOR UPDATE
	`, competitionID).Scan(&lifecycle); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrCompetitionNotFound
		}
		return fmt.Errorf("competition repository: lock competition: %w", err)
	}
	if lifecycle == LifecycleCompleted {
		return tx.Commit(ctx) // idempotent replay: already finalized
	}
	if lifecycle != LifecycleFinalizing {
		return ErrLifecycleConflict
	}

	for _, res := range results {
		if _, err := tx.Exec(ctx, `
			INSERT INTO competition_results
				(id, competition_id, entry_id, user_id, final_rank, final_index, final_return_percentage, finalized_at, result_evidence_json)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (competition_id, entry_id) DO NOTHING
		`, uuid.NewString(), competitionID, res.EntryID, res.UserID, res.Rank, res.Index, res.ReturnPct, now.UTC(), res.EvidenceJSON); err != nil {
			return fmt.Errorf("competition repository: insert result: %w", err)
		}
		// Scored entries become terminally 'finalized' — this is what makes
		// the result immutable end to end: a finalized entry can no longer be
		// disqualified, withdrawn, or otherwise mutated by any other guarded
		// transition (all of which require entry_status = 'active').
		if _, err := tx.Exec(ctx, `
			UPDATE competition_entries SET entry_status = $1
			WHERE id = $2 AND entry_status = $3
		`, EntryFinalized, res.EntryID, EntryActive); err != nil {
			return fmt.Errorf("competition repository: finalize entry status: %w", err)
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE competitions SET lifecycle_status = $1, finalized_at = $2 WHERE id = $3
	`, LifecycleCompleted, now.UTC(), competitionID); err != nil {
		return fmt.Errorf("competition repository: complete lifecycle: %w", err)
	}

	return tx.Commit(ctx)
}

func (r *PostgresCompetitionRepository) ListResults(ctx context.Context, competitionID string) ([]CompetitionResultEntry, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT cr.final_rank, u.display_name, u.avatar_key, cr.final_return_percentage, cr.final_index
		FROM competition_results cr
		JOIN users u ON u.id = cr.user_id AND u.deleted_at IS NULL
		WHERE cr.competition_id = $1
		ORDER BY cr.final_rank
	`, competitionID)
	if err != nil {
		return nil, fmt.Errorf("competition repository: list results: %w", err)
	}
	defer rows.Close()

	out := make([]CompetitionResultEntry, 0)
	for rows.Next() {
		var e CompetitionResultEntry
		if err := rows.Scan(&e.Rank, &e.DisplayName, &e.AvatarKey, &e.ReturnPercentage, &e.CompetitionIndex); err != nil {
			return nil, fmt.Errorf("competition repository: scan result: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
