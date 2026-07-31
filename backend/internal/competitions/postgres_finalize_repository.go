package competitions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Postgres implementation of FinalizationRepository (tables
// competition_finalization_generations and competition_finalization_rows,
// migration 0054; terminal target competition_results, migration 0048).
// Mirrors PostgresCompetitionRepository's ranking-generation implementation
// (postgres_ranking_repository.go) — building generation, bounded claims,
// coverage counters, DB-side rank materialization — adapted for a one-shot
// terminal promotion instead of a continuously-superseded live board.

var _ FinalizationRepository = (*PostgresCompetitionRepository)(nil)

func (r *PostgresCompetitionRepository) EnsureBuildingFinalizationGeneration(ctx context.Context, competitionID string) (FinalizationGeneration, error) {
	var g FinalizationGeneration
	err := r.pool.QueryRow(ctx, `
		SELECT generation, status, expected_entries, processed_entries, successful_entries,
		       failed_entries, COALESCE(cursor_entry_id::text, ''), write_failure, started_at
		FROM competition_finalization_generations
		WHERE competition_id = $1 AND status = 'building'
		ORDER BY generation DESC LIMIT 1
	`, competitionID).Scan(&g.Generation, &g.Status, &g.ExpectedEntries, &g.ProcessedEntries,
		&g.SuccessfulEntries, &g.FailedEntries, &g.CursorEntryID, &g.WriteFailure, &g.StartedAt)
	if err == nil {
		g.CompetitionID = competitionID
		return g, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return FinalizationGeneration{}, fmt.Errorf("competition repository: ensure building finalization generation: %w", err)
	}

	var next int64
	if err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(MAX(generation), 0) + 1 FROM competition_finalization_generations WHERE competition_id = $1
	`, competitionID).Scan(&next); err != nil {
		return FinalizationGeneration{}, fmt.Errorf("competition repository: next finalization generation number: %w", err)
	}
	id := fmt.Sprintf("%s-final-gen-%d", competitionID, next)
	if _, err := r.pool.Exec(ctx, `
		INSERT INTO competition_finalization_generations (id, competition_id, generation, status, started_at)
		VALUES ($1, $2, $3, 'building', now())
		ON CONFLICT (competition_id, generation) DO NOTHING
	`, id, competitionID, next); err != nil {
		return FinalizationGeneration{}, fmt.Errorf("competition repository: create building finalization generation: %w", err)
	}
	return FinalizationGeneration{CompetitionID: competitionID, Generation: next, Status: GenerationBuilding, StartedAt: time.Now().UTC()}, nil
}

func (r *PostgresCompetitionRepository) ClaimEntriesForFinalization(ctx context.Context, competitionID, afterEntryID string, limit int) ([]CompetitionEntry, error) {
	var rows pgxRows
	var err error
	if afterEntryID == "" {
		rows, err = r.pool.Query(ctx, `
			SELECT id, competition_id, user_id, eligible_starting_value_base
			FROM competition_entries
			WHERE competition_id = $1 AND entry_status = $2
			ORDER BY id LIMIT $3
		`, competitionID, EntryActive, limit)
	} else {
		rows, err = r.pool.Query(ctx, `
			SELECT id, competition_id, user_id, eligible_starting_value_base
			FROM competition_entries
			WHERE competition_id = $1 AND entry_status = $2 AND id > $3
			ORDER BY id LIMIT $4
		`, competitionID, EntryActive, afterEntryID, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("competition repository: claim entries for finalization: %w", err)
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

func (r *PostgresCompetitionRepository) UpsertFinalizationRow(ctx context.Context, competitionID string, generation int64, row CompetitionFinalizationRow) error {
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO competition_finalization_rows
			(competition_id, generation, entry_id, user_id, competition_index, return_percentage, valued_at)
		SELECT $1, $2, $3, $4, $5, $6, $7
		WHERE EXISTS (
			SELECT 1 FROM competition_finalization_generations
			WHERE competition_id = $1 AND generation = $2 AND status = 'building'
		)
		ON CONFLICT (competition_id, generation, entry_id) DO UPDATE SET
			competition_index = EXCLUDED.competition_index,
			return_percentage = EXCLUDED.return_percentage,
			valued_at         = EXCLUDED.valued_at
	`, competitionID, generation, row.EntryID, row.UserID, row.Index, row.ReturnPct, row.ValuedAt.UTC())
	if err != nil {
		return fmt.Errorf("competition repository: upsert finalization row: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrGenerationConflict
	}
	return nil
}

func (r *PostgresCompetitionRepository) AdvanceFinalizationGeneration(ctx context.Context, competitionID string, generation int64, cursorEntryID string, processed, successful, failed int, writeFailed bool) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE competition_finalization_generations
		SET cursor_entry_id    = $1,
		    processed_entries  = processed_entries + $2,
		    successful_entries = successful_entries + $3,
		    failed_entries     = failed_entries + $4,
		    write_failure      = write_failure OR $5,
		    expected_entries   = GREATEST(expected_entries, processed_entries + $2)
		WHERE competition_id = $6 AND generation = $7 AND status = 'building'
	`, nullableUUID(cursorEntryID), processed, successful, failed, writeFailed, competitionID, generation)
	if err != nil {
		return fmt.Errorf("competition repository: advance finalization generation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrGenerationConflict
	}
	return nil
}

func (r *PostgresCompetitionRepository) FailFinalizationGeneration(ctx context.Context, competitionID string, generation int64, reason string, now time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("competition repository: begin fail finalization generation tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		UPDATE competition_finalization_generations
		SET status = 'failed', completed_at = $1, failure_reason = $2
		WHERE competition_id = $3 AND generation = $4 AND status = 'building'
	`, now.UTC(), reason, competitionID, generation)
	if err != nil {
		return fmt.Errorf("competition repository: fail finalization generation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrGenerationConflict
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM competition_finalization_rows WHERE competition_id = $1 AND generation = $2
	`, competitionID, generation); err != nil {
		return fmt.Errorf("competition repository: prune failed finalization generation: %w", err)
	}
	return tx.Commit(ctx)
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

// PromoteFinalizationGeneration atomically: locks the competition row FOR
// UPDATE and returns early (idempotent no-op success) if it is already
// completed — a worker restart or retry replaying the same finalization,
// regardless of which generation number it holds; locks and validates the
// finalization generation is still cleanly building; materializes sequential
// ranks over its draft rows with the same tie-break chain as
// PromoteGeneration; copies them into the immutable competition_results
// table; transitions scored entries to finalized; transitions the edition
// finalizing -> completed; commits the competition.finalized outbox event
// (participant list read back from the just-written results, never from
// Go-side accumulated state); marks the generation completed; and prunes its
// draft rows.
func (r *PostgresCompetitionRepository) PromoteFinalizationGeneration(ctx context.Context, competitionID string, generation int64, now time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("competition repository: begin promote finalization tx: %w", err)
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

	var status string
	var writeFailure bool
	var failed int
	if err := tx.QueryRow(ctx, `
		SELECT status, write_failure, failed_entries
		FROM competition_finalization_generations
		WHERE competition_id = $1 AND generation = $2 FOR UPDATE
	`, competitionID, generation).Scan(&status, &writeFailure, &failed); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrGenerationConflict
		}
		return fmt.Errorf("competition repository: lock finalization generation: %w", err)
	}
	if status != GenerationBuilding || writeFailure || failed != 0 {
		return ErrGenerationConflict
	}

	if _, err := tx.Exec(ctx, `
		UPDATE competition_finalization_rows cfr
		SET materialized_rank = ranked.rnk
		FROM (
			SELECT cfr2.entry_id,
			       ROW_NUMBER() OVER (
			           ORDER BY cfr2.return_percentage DESC, u.display_name ASC, cfr2.user_id ASC
			       ) AS rnk
			FROM competition_finalization_rows cfr2
			JOIN users u ON u.id = cfr2.user_id AND u.deleted_at IS NULL
			WHERE cfr2.competition_id = $1 AND cfr2.generation = $2
		) ranked
		WHERE cfr.competition_id = $1 AND cfr.generation = $2 AND cfr.entry_id = ranked.entry_id
	`, competitionID, generation); err != nil {
		return fmt.Errorf("competition repository: materialize final ranks: %w", err)
	}

	evidence, err := json.Marshal(map[string]any{
		"valuation_policy":  "fixed_basket_price_return_v1",
		"corporate_actions": "splits_and_symbol_changes_normalized",
		"finalized_at":      now,
	})
	if err != nil {
		return fmt.Errorf("competition repository: marshal result evidence: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO competition_results
			(id, competition_id, entry_id, user_id, final_rank, final_index, final_return_percentage, finalized_at, result_evidence_json)
		SELECT gen_random_uuid(), competition_id, entry_id, user_id, materialized_rank, competition_index, return_percentage, $3, $4
		FROM competition_finalization_rows
		WHERE competition_id = $1 AND generation = $2 AND materialized_rank IS NOT NULL
		ON CONFLICT (competition_id, entry_id) DO NOTHING
	`, competitionID, generation, now.UTC(), evidence); err != nil {
		return fmt.Errorf("competition repository: insert final results: %w", err)
	}

	// Scored entries become terminally 'finalized' — this is what makes the
	// result immutable end to end: a finalized entry can no longer be
	// disqualified, withdrawn, or otherwise mutated by any other guarded
	// transition (all of which require entry_status = 'active').
	if _, err := tx.Exec(ctx, `
		UPDATE competition_entries ce
		SET entry_status = $1
		FROM competition_finalization_rows cfr
		WHERE cfr.competition_id = $2 AND cfr.generation = $3
		  AND ce.id = cfr.entry_id AND ce.entry_status = $4
	`, EntryFinalized, competitionID, generation, EntryActive); err != nil {
		return fmt.Errorf("competition repository: finalize entry statuses: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE competitions SET lifecycle_status = $1, finalized_at = $2 WHERE id = $3
	`, LifecycleCompleted, now.UTC(), competitionID); err != nil {
		return fmt.Errorf("competition repository: complete lifecycle: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO competition_outbox (id, event_type, competition_id, participant_ids, created_at)
		SELECT $1, $2, $3, COALESCE(jsonb_agg(user_id), '[]'::jsonb), $4
		FROM competition_results WHERE competition_id = $3
		ON CONFLICT (event_type, competition_id) DO NOTHING
	`, uuid.NewString(), CompetitionFinalizedEvent, competitionID, now.UTC()); err != nil {
		return fmt.Errorf("competition repository: enqueue finalization: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE competition_finalization_generations
		SET status = 'completed', completed_at = $1
		WHERE competition_id = $2 AND generation = $3 AND status = 'building'
	`, now.UTC(), competitionID, generation); err != nil {
		return fmt.Errorf("competition repository: complete finalization generation: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		DELETE FROM competition_finalization_rows WHERE competition_id = $1 AND generation = $2
	`, competitionID, generation); err != nil {
		return fmt.Errorf("competition repository: prune finalization rows: %w", err)
	}

	return tx.Commit(ctx)
}

// TryLockCompetitionFinalization mirrors TryLockCompetitionRanking (see its
// doc comment) — a session-level Postgres advisory lock keyed on
// competitionID, distinct from the ranking lock's key so a competition's
// ranking refresh and its finalization pass never contend with each other.
func (r *PostgresCompetitionRepository) TryLockCompetitionFinalization(ctx context.Context, competitionID string) (func(context.Context), bool, error) {
	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("competition repository: acquire finalization lock connection: %w", err)
	}
	var locked bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended($1, 1))`, competitionID).Scan(&locked); err != nil {
		conn.Release()
		return nil, false, fmt.Errorf("competition repository: try finalization advisory lock: %w", err)
	}
	if !locked {
		conn.Release()
		return nil, false, nil
	}
	return func(unlockCtx context.Context) {
		_, _ = conn.Exec(unlockCtx, `SELECT pg_advisory_unlock(hashtextextended($1, 1))`, competitionID)
		conn.Release()
	}, true, nil
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
