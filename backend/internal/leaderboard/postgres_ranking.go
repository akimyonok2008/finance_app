package leaderboard

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ardakimyonok/finance_app/internal/money"
)

// PostgresRankingStore implements RankingStore (table leaderboard_rankings,
// migrations 0035, 0038, and 0040): the denormalized ranking projection that
// lets board/rank/standing reads become a single indexed query instead of
// enumerating every user. It is populated by Service.RefreshCache, never by
// request handling. Migration 0040 added the materialized rnk column:
// CompleteCycle computes the population-wide rank once per generation, so
// TopPage/RankOf/ValueAtRank below are plain indexed lookups rather than a
// window function re-ranking every row on every read.
//
// Rows are tagged with a generation (leaderboard_ranking_state, migration
// 0038). Writes always target the "building" generation; reads always target
// the "active" one. A generation only becomes active — visible to reads —
// once CompleteCycle promotes it, which the caller must only do after a full
// pass over every eligible user. This keeps readers from ever seeing a
// partially-built generation, and keeps a single perpetually-failing user
// from pinning the whole projection as stale forever (see migration 0038's
// header comment for the two bugs this fixes).
type PostgresRankingStore struct {
	pool *pgxpool.Pool
}

func NewPostgresRankingStore(pool *pgxpool.Pool) *PostgresRankingStore {
	return &PostgresRankingStore{pool: pool}
}

func (r *PostgresRankingStore) Upsert(ctx context.Context, tf Timeframe, userID string, idx money.IndexValue, retPct money.Ratio, trackingStartedAt time.Time) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO leaderboard_rankings
			(user_id, timeframe, generation, ranked_index, ranked_return_percentage, tracking_started_at, computed_at)
		VALUES ($1, $2, (SELECT building_generation FROM leaderboard_ranking_state WHERE id), $3, $4, $5, now())
		ON CONFLICT (timeframe, generation, user_id) DO UPDATE SET
			ranked_index             = EXCLUDED.ranked_index,
			ranked_return_percentage = EXCLUDED.ranked_return_percentage,
			tracking_started_at      = EXCLUDED.tracking_started_at,
			computed_at              = EXCLUDED.computed_at
	`, userID, string(tf), idx, retPct, trackingStartedAt.UTC())
	return err
}

func (r *PostgresRankingStore) Delete(ctx context.Context, tf Timeframe, userID string) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM leaderboard_rankings
		WHERE timeframe = $1 AND user_id = $2
		  AND generation = (SELECT building_generation FROM leaderboard_ranking_state WHERE id)
	`, string(tf), userID)
	return err
}

// TopPage returns up to limit rows for tf from the active generation, using
// the rnk materialized once per generation by CompleteCycle (see its doc) —
// a plain indexed range scan on (timeframe, generation, rnk), not a
// per-request window function over the whole population.
func (r *PostgresRankingStore) TopPage(ctx context.Context, tf Timeframe, limit int) ([]rankedRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT lr.user_id, lr.ranked_index, lr.ranked_return_percentage,
		       u.display_name, u.avatar_key, lr.rnk
		FROM leaderboard_rankings lr
		JOIN users u ON u.id = lr.user_id AND u.deleted_at IS NULL
		WHERE lr.timeframe = $1
		  AND lr.generation = (SELECT active_generation FROM leaderboard_ranking_state WHERE id)
		  AND lr.rnk IS NOT NULL
		ORDER BY lr.rnk
		LIMIT $2
	`, string(tf), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]rankedRow, 0, limit)
	for rows.Next() {
		var (
			userID, displayName, avatarKey string
			idx                            money.IndexValue
			retPct                         money.Ratio
			rank                           int
		)
		if err := rows.Scan(&userID, &idx, &retPct, &displayName, &avatarKey, &rank); err != nil {
			return nil, err
		}
		out = append(out, rankedRow{
			userID:      userID,
			entry:       rankedEntry(rank, displayName, avatarKey, retPct, idx),
			returnPct:   retPct,
			rankedIndex: idx,
		})
	}
	return out, rows.Err()
}

// RankOf reads userID's materialized rnk (see CompleteCycle) directly off
// the active generation's primary key — a single-row lookup, not a
// whole-board window function filtered down to one user.
func (r *PostgresRankingStore) RankOf(ctx context.Context, tf Timeframe, userID string) (int, money.IndexValue, money.Ratio, bool, error) {
	var (
		idx    money.IndexValue
		retPct money.Ratio
		rank   int
	)
	err := r.pool.QueryRow(ctx, `
		SELECT lr.ranked_index, lr.ranked_return_percentage, lr.rnk
		FROM leaderboard_rankings lr
		JOIN users u ON u.id = lr.user_id AND u.deleted_at IS NULL
		WHERE lr.timeframe = $1
		  AND lr.generation = (SELECT active_generation FROM leaderboard_ranking_state WHERE id)
		  AND lr.user_id = $2
		  AND lr.rnk IS NOT NULL
	`, string(tf), userID).Scan(&idx, &retPct, &rank)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, money.ZeroIndexValue(), money.ZeroRatio(), false, nil
	}
	if err != nil {
		return 0, money.ZeroIndexValue(), money.ZeroRatio(), false, err
	}
	return rank, idx, retPct, true, nil
}

// ValueAtRank returns the return percentage of the row at an exact 1-based
// rank in the active generation, for milestone-gap calculations. Reads the
// rnk materialized by CompleteCycle directly via the
// (timeframe, generation, rnk) index, instead of re-ranking the population
// to find one row.
func (r *PostgresRankingStore) ValueAtRank(ctx context.Context, tf Timeframe, rank int) (money.Ratio, bool, error) {
	var retPct money.Ratio
	err := r.pool.QueryRow(ctx, `
		SELECT lr.ranked_return_percentage
		FROM leaderboard_rankings lr
		JOIN users u ON u.id = lr.user_id AND u.deleted_at IS NULL
		WHERE lr.timeframe = $1
		  AND lr.generation = (SELECT active_generation FROM leaderboard_ranking_state WHERE id)
		  AND lr.rnk = $2
		LIMIT 1
	`, string(tf), rank).Scan(&retPct)
	if errors.Is(err, pgx.ErrNoRows) {
		return money.ZeroRatio(), false, nil
	}
	if err != nil {
		return money.ZeroRatio(), false, err
	}
	return retPct, true, nil
}

func (r *PostgresRankingStore) Count(ctx context.Context, tf Timeframe) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM leaderboard_rankings lr
		JOIN users u ON u.id = lr.user_id AND u.deleted_at IS NULL
		WHERE lr.timeframe = $1
		  AND lr.generation = (SELECT active_generation FROM leaderboard_ranking_state WHERE id)
	`, string(tf)).Scan(&n)
	return n, err
}

// ActiveGenerationAge reports when the currently-active generation was
// promoted by CompleteCycle — the staleness signal Service.rankingFresh uses
// to decide whether to trust this projection over live computation.
// found=false means no generation has ever completed a full, verified pass
// (a cold start, or rows inherited from the pre-generation schema that no
// cycle has confirmed complete yet).
func (r *PostgresRankingStore) ActiveGenerationAge(ctx context.Context) (time.Time, bool, error) {
	var at *time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT activated_at FROM leaderboard_ranking_state WHERE id
	`).Scan(&at)
	if err != nil {
		return time.Time{}, false, err
	}
	if at == nil {
		return time.Time{}, false, nil
	}
	return *at, true, nil
}

// CompleteCycle materializes the final ordinal rank (rnk) for the building
// generation, then atomically promotes it to active (making its rows the
// ones every read method above serves), starts the next building
// generation, and prunes the rows of the generation that was just replaced.
//
// rnk is computed exactly once per generation, here, with the same RANK()
// window function and tie-break (return% desc, display_name asc, user_id
// asc) TopPage/RankOf/ValueAtRank used to run on every single read. Because
// this runs inside one transaction that only commits once rnk is fully
// populated and the generation promoted, a reader's query either sees the
// old active generation in full (with its own already-materialized rnk) or
// the new one in full, never a partially-ranked generation and never a mix.
func (r *PostgresRankingStore) CompleteCycle(ctx context.Context) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var buildingGeneration, previousGeneration int64
	if err := tx.QueryRow(ctx, `
		SELECT active_generation, building_generation
		FROM leaderboard_ranking_state WHERE id FOR UPDATE
	`).Scan(&previousGeneration, &buildingGeneration); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE leaderboard_rankings lr
		SET rnk = ranked.rnk
		FROM (
			SELECT lr2.timeframe, lr2.user_id,
			       RANK() OVER (
			           PARTITION BY lr2.timeframe
			           ORDER BY lr2.ranked_return_percentage DESC, u.display_name ASC, lr2.user_id ASC
			       ) AS rnk
			FROM leaderboard_rankings lr2
			JOIN users u ON u.id = lr2.user_id AND u.deleted_at IS NULL
			WHERE lr2.generation = $1
		) ranked
		WHERE lr.generation = $1 AND lr.timeframe = ranked.timeframe AND lr.user_id = ranked.user_id
	`, buildingGeneration); err != nil {
		return err
	}

	var promotedGeneration int64
	if err := tx.QueryRow(ctx, `
		UPDATE leaderboard_ranking_state
		SET active_generation   = building_generation,
		    building_generation = building_generation + 1,
		    cycle_started_at    = now(),
		    activated_at        = now()
		WHERE id
		RETURNING active_generation
	`).Scan(&promotedGeneration); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		DELETE FROM leaderboard_rankings
		WHERE generation = $1 AND generation <> $2
	`, previousGeneration, promotedGeneration); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
