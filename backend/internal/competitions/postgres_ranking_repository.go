package competitions

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Postgres implementation of RankingRepository (tables
// competition_ranking_generations and competition_rankings, migration 0047).
// Mirrors leaderboard.PostgresRankingStore's generation-promotion pattern
// (see its doc comment), but keeps one row per generation ATTEMPT — rather
// than a single active/building pointer row — so expected/processed/
// successful/failed counts and any failure reason stay attached to the
// specific attempt they describe, per the spec's competition_ranking_generations
// shape.

var _ RankingRepository = (*PostgresCompetitionRepository)(nil)

func (r *PostgresCompetitionRepository) EnsureBuildingGeneration(ctx context.Context, competitionID string) (RankingGeneration, error) {
	var g RankingGeneration
	err := r.pool.QueryRow(ctx, `
		SELECT generation, status, expected_entries, processed_entries, successful_entries,
		       failed_entries, COALESCE(cursor_entry_id::text, ''), write_failure, started_at
		FROM competition_ranking_generations
		WHERE competition_id = $1 AND status = 'building'
		ORDER BY generation DESC LIMIT 1
	`, competitionID).Scan(&g.Generation, &g.Status, &g.ExpectedEntries, &g.ProcessedEntries,
		&g.SuccessfulEntries, &g.FailedEntries, &g.CursorEntryID, &g.WriteFailure, &g.StartedAt)
	if err == nil {
		g.CompetitionID = competitionID
		return g, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return RankingGeneration{}, fmt.Errorf("competition repository: ensure building generation: %w", err)
	}

	// No building generation exists (cold start, or the previous one was just
	// promoted). Start the next one after the highest generation ever seen.
	var next int64
	if err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(MAX(generation), 0) + 1 FROM competition_ranking_generations WHERE competition_id = $1
	`, competitionID).Scan(&next); err != nil {
		return RankingGeneration{}, fmt.Errorf("competition repository: next generation number: %w", err)
	}
	id := generationRowID(competitionID, next)
	if _, err := r.pool.Exec(ctx, `
		INSERT INTO competition_ranking_generations (id, competition_id, generation, status, started_at)
		VALUES ($1, $2, $3, 'building', now())
		ON CONFLICT (competition_id, generation) DO NOTHING
	`, id, competitionID, next); err != nil {
		return RankingGeneration{}, fmt.Errorf("competition repository: create building generation: %w", err)
	}
	return RankingGeneration{CompetitionID: competitionID, Generation: next, Status: GenerationBuilding, StartedAt: time.Now().UTC()}, nil
}

// generationRowID derives a deterministic id for a generation row so
// concurrent replicas racing to create the same (competition, generation)
// collide on the same primary key rather than creating duplicates — the
// ON CONFLICT above then makes creation idempotent.
func generationRowID(competitionID string, generation int64) string {
	return fmt.Sprintf("%s-gen-%d", competitionID, generation)
}

func (r *PostgresCompetitionRepository) ClaimActiveEntries(ctx context.Context, competitionID, afterEntryID string, limit int) ([]CompetitionEntry, error) {
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
		return nil, fmt.Errorf("competition repository: claim active entries: %w", err)
	}
	defer rows.Close()

	var out []CompetitionEntry
	for rows.Next() {
		var e CompetitionEntry
		if err := rows.Scan(&e.ID, &e.CompetitionID, &e.UserID, &e.EligibleStartingValueBase); err != nil {
			return nil, fmt.Errorf("competition repository: scan claimed entry: %w", err)
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

// pgxRows is the narrow subset of pgx.Rows this file needs, letting
// ClaimActiveEntries share one variable across its two query branches.
type pgxRows interface {
	Next() bool
	Scan(dest ...any) error
	Close()
	Err() error
}

func (r *PostgresCompetitionRepository) UpsertRanking(ctx context.Context, competitionID string, generation int64, row CompetitionRankingRow) error {
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO competition_rankings
			(competition_id, generation, entry_id, user_id, competition_index, return_percentage, valued_at)
		SELECT $1, $2, $3, $4, $5, $6, $7
		WHERE EXISTS (
			SELECT 1 FROM competition_ranking_generations
			WHERE competition_id = $1 AND generation = $2 AND status = 'building'
		)
		ON CONFLICT (competition_id, generation, entry_id) DO UPDATE SET
			competition_index = EXCLUDED.competition_index,
			return_percentage = EXCLUDED.return_percentage,
			valued_at         = EXCLUDED.valued_at
	`, competitionID, generation, row.EntryID, row.UserID, row.Index, row.ReturnPct, row.ValuedAt.UTC())
	if err != nil {
		return fmt.Errorf("competition repository: upsert ranking: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrGenerationConflict
	}
	return nil
}

func (r *PostgresCompetitionRepository) AdvanceGeneration(ctx context.Context, competitionID string, generation int64, cursorEntryID string, processed, successful, failed int, writeFailed bool) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE competition_ranking_generations
		SET cursor_entry_id    = $1,
		    processed_entries  = processed_entries + $2,
		    successful_entries = successful_entries + $3,
		    failed_entries     = failed_entries + $4,
		    write_failure      = write_failure OR $5,
		    expected_entries   = GREATEST(expected_entries, processed_entries + $2)
		WHERE competition_id = $6 AND generation = $7 AND status = 'building'
	`, nullableUUID(cursorEntryID), processed, successful, failed, writeFailed, competitionID, generation)
	if err != nil {
		return fmt.Errorf("competition repository: advance generation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrGenerationConflict
	}
	return nil
}

func (r *PostgresCompetitionRepository) FailGeneration(ctx context.Context, competitionID string, generation int64, reason string, now time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("competition repository: begin fail generation tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		UPDATE competition_ranking_generations
		SET status = 'failed', completed_at = $1, failure_reason = $2
		WHERE competition_id = $3 AND generation = $4 AND status = 'building'
	`, now.UTC(), reason, competitionID, generation)
	if err != nil {
		return fmt.Errorf("competition repository: fail generation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrGenerationConflict
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM competition_rankings
		WHERE competition_id = $1 AND generation = $2
	`, competitionID, generation); err != nil {
		return fmt.Errorf("competition repository: prune failed generation: %w", err)
	}
	return tx.Commit(ctx)
}

// PromoteGeneration materializes sequential ranks (return% desc, display name
// asc, user_id asc — a strict total order since user_id is unique, so ranks
// are always sequential and never shared, see the package's tie-break policy
// doc), then atomically: marks the generation active, marks the previously
// active generation superseded and deletes its rows, and commits — all inside
// one transaction, so a leaderboard read never observes a partially-ranked
// generation or a gap between the old and new active generation.
func (r *PostgresCompetitionRepository) PromoteGeneration(ctx context.Context, competitionID string, generation int64, now time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("competition repository: begin promote tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var status string
	var writeFailure bool
	var processed, successful, failed int
	if err := tx.QueryRow(ctx, `
		SELECT status, write_failure, processed_entries, successful_entries, failed_entries
		WHERE competition_id = $1 AND generation = $2 FOR UPDATE
	`, competitionID, generation).Scan(&status, &writeFailure, &processed, &successful, &failed); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrGenerationConflict
		}
		return fmt.Errorf("competition repository: lock generation: %w", err)
	}
	if status != GenerationBuilding || writeFailure || failed != 0 || processed != successful {
		return ErrGenerationConflict
	}

	if _, err := tx.Exec(ctx, `
		UPDATE competition_rankings cr
		SET materialized_rank = ranked.rnk
		FROM (
			SELECT cr2.entry_id,
			       ROW_NUMBER() OVER (
			           ORDER BY cr2.return_percentage DESC, u.display_name ASC, cr2.user_id ASC
			       ) AS rnk
			FROM competition_rankings cr2
			JOIN users u ON u.id = cr2.user_id AND u.deleted_at IS NULL
			WHERE cr2.competition_id = $1 AND cr2.generation = $2
		) ranked
		WHERE cr.competition_id = $1 AND cr.generation = $2 AND cr.entry_id = ranked.entry_id
	`, competitionID, generation); err != nil {
		return fmt.Errorf("competition repository: materialize ranks: %w", err)
	}

	// Find and supersede whichever generation was active before this one
	// FIRST — the one-active-per-competition unique index means the new
	// generation cannot become active while the old one still holds that
	// status. The active generation still remains fully readable right up
	// until this transaction commits (this row lock plus the still-populated
	// competition_rankings rows mean no reader ever observes a gap).
	var previous *int64
	rows, err := tx.Query(ctx, `
		SELECT generation FROM competition_ranking_generations
		WHERE competition_id = $1 AND status = 'active' AND generation <> $2
	`, competitionID, generation)
	if err != nil {
		return fmt.Errorf("competition repository: find previous active generation: %w", err)
	}
	for rows.Next() {
		var g int64
		if err := rows.Scan(&g); err != nil {
			rows.Close()
			return err
		}
		previous = &g
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()

	if previous != nil {
		if _, err := tx.Exec(ctx, `
			UPDATE competition_ranking_generations SET status = 'superseded'
			WHERE competition_id = $1 AND generation = $2
		`, competitionID, *previous); err != nil {
			return fmt.Errorf("competition repository: supersede previous generation: %w", err)
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE competition_ranking_generations
		SET status = 'active', completed_at = $1, activated_at = $1
		WHERE competition_id = $2 AND generation = $3 AND status = 'building'
	`, now.UTC(), competitionID, generation); err != nil {
		return fmt.Errorf("competition repository: activate generation: %w", err)
	}

	if previous != nil {
		if _, err := tx.Exec(ctx, `
			DELETE FROM competition_rankings WHERE competition_id = $1 AND generation = $2
		`, competitionID, *previous); err != nil {
			return fmt.Errorf("competition repository: prune previous generation rows: %w", err)
		}
	}

	return tx.Commit(ctx)
}

func (r *PostgresCompetitionRepository) ActiveGeneration(ctx context.Context, competitionID string) (int64, time.Time, bool, error) {
	var (
		generation  int64
		activatedAt time.Time
	)
	err := r.pool.QueryRow(ctx, `
		SELECT generation, activated_at FROM competition_ranking_generations
		WHERE competition_id = $1 AND status = 'active'
	`, competitionID).Scan(&generation, &activatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, time.Time{}, false, nil
	}
	if err != nil {
		return 0, time.Time{}, false, fmt.Errorf("competition repository: active generation: %w", err)
	}
	return generation, activatedAt, true, nil
}

func (r *PostgresCompetitionRepository) LeaderboardPage(ctx context.Context, competitionID string, afterRank, limit int) ([]CompetitionLeaderboardEntry, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT cr.materialized_rank, u.display_name, u.avatar_key, cr.return_percentage, cr.competition_index
		FROM competition_rankings cr
		JOIN users u ON u.id = cr.user_id AND u.deleted_at IS NULL
		WHERE cr.competition_id = $1
		  AND cr.generation = (SELECT generation FROM competition_ranking_generations WHERE competition_id = $1 AND status = 'active')
		  AND cr.materialized_rank IS NOT NULL AND cr.materialized_rank > $2
		ORDER BY cr.materialized_rank
		LIMIT $3
	`, competitionID, afterRank, limit)
	if err != nil {
		return nil, fmt.Errorf("competition repository: leaderboard page: %w", err)
	}
	defer rows.Close()

	out := make([]CompetitionLeaderboardEntry, 0, limit)
	for rows.Next() {
		var e CompetitionLeaderboardEntry
		if err := rows.Scan(&e.Rank, &e.DisplayName, &e.AvatarKey, &e.ReturnPercentage, &e.CompetitionIndex); err != nil {
			return nil, fmt.Errorf("competition repository: scan leaderboard row: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *PostgresCompetitionRepository) UserRankRow(ctx context.Context, competitionID, userID string) (CompetitionRankingRow, bool, error) {
	var row CompetitionRankingRow
	row.UserID = userID
	err := r.pool.QueryRow(ctx, `
		SELECT cr.entry_id, cr.materialized_rank, cr.competition_index, cr.return_percentage, cr.valued_at
		FROM competition_rankings cr
		JOIN users u ON u.id = cr.user_id AND u.deleted_at IS NULL
		WHERE cr.competition_id = $1 AND cr.user_id = $2
		  AND cr.generation = (SELECT generation FROM competition_ranking_generations WHERE competition_id = $1 AND status = 'active')
		  AND cr.materialized_rank IS NOT NULL
	`, competitionID, userID).Scan(&row.EntryID, &row.Rank, &row.Index, &row.ReturnPct, &row.ValuedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return CompetitionRankingRow{}, false, nil
	}
	if err != nil {
		return CompetitionRankingRow{}, false, fmt.Errorf("competition repository: user rank row: %w", err)
	}
	return row, true, nil
}
