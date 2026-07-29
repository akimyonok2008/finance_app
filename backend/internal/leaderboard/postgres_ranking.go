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
// migration 0035): the denormalized ranking projection that lets board/rank/
// standing reads become a single indexed query instead of enumerating every
// user. It is populated by Service.RefreshCache, never by request handling.
type PostgresRankingStore struct {
	pool *pgxpool.Pool
}

func NewPostgresRankingStore(pool *pgxpool.Pool) *PostgresRankingStore {
	return &PostgresRankingStore{pool: pool}
}

func (r *PostgresRankingStore) Upsert(ctx context.Context, tf Timeframe, userID string, idx money.IndexValue, retPct money.Ratio, trackingStartedAt time.Time) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO leaderboard_rankings
			(user_id, timeframe, ranked_index, ranked_return_percentage, tracking_started_at, computed_at)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (timeframe, user_id) DO UPDATE SET
			ranked_index             = EXCLUDED.ranked_index,
			ranked_return_percentage = EXCLUDED.ranked_return_percentage,
			tracking_started_at      = EXCLUDED.tracking_started_at,
			computed_at              = EXCLUDED.computed_at
	`, userID, string(tf), idx, retPct, trackingStartedAt.UTC())
	return err
}

func (r *PostgresRankingStore) Delete(ctx context.Context, tf Timeframe, userID string) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM leaderboard_rankings WHERE timeframe = $1 AND user_id = $2
	`, string(tf), userID)
	return err
}

// TopPage returns up to limit rows for tf, ranked with a Postgres window
// function and joined to display metadata in one round trip. The ORDER BY
// exactly mirrors sortRankedRows' tie-break (return% desc, display_name asc,
// user_id asc) so results agree with the live-computed path bit-for-bit.
func (r *PostgresRankingStore) TopPage(ctx context.Context, tf Timeframe, limit int) ([]rankedRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT lr.user_id, lr.ranked_index, lr.ranked_return_percentage,
		       u.display_name, u.avatar_key,
		       RANK() OVER (
		           ORDER BY lr.ranked_return_percentage DESC, u.display_name ASC, lr.user_id ASC
		       ) AS rnk
		FROM leaderboard_rankings lr
		JOIN users u ON u.id = lr.user_id AND u.deleted_at IS NULL
		WHERE lr.timeframe = $1
		ORDER BY rnk
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

// RankOf computes the same window function as TopPage in a CTE and filters to
// userID, so a single-user rank lookup never loads the whole board.
func (r *PostgresRankingStore) RankOf(ctx context.Context, tf Timeframe, userID string) (int, money.IndexValue, money.Ratio, bool, error) {
	var (
		idx    money.IndexValue
		retPct money.Ratio
		rank   int
	)
	err := r.pool.QueryRow(ctx, `
		WITH ranked AS (
			SELECT lr.user_id, lr.ranked_index, lr.ranked_return_percentage,
			       RANK() OVER (
			           ORDER BY lr.ranked_return_percentage DESC, u.display_name ASC, lr.user_id ASC
			       ) AS rnk
			FROM leaderboard_rankings lr
			JOIN users u ON u.id = lr.user_id AND u.deleted_at IS NULL
			WHERE lr.timeframe = $1
		)
		SELECT ranked_index, ranked_return_percentage, rnk FROM ranked WHERE user_id = $2
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
// rank, for milestone-gap calculations.
func (r *PostgresRankingStore) ValueAtRank(ctx context.Context, tf Timeframe, rank int) (money.Ratio, bool, error) {
	var retPct money.Ratio
	err := r.pool.QueryRow(ctx, `
		WITH ranked AS (
			SELECT lr.ranked_return_percentage,
			       RANK() OVER (
			           ORDER BY lr.ranked_return_percentage DESC, u.display_name ASC, lr.user_id ASC
			       ) AS rnk
			FROM leaderboard_rankings lr
			JOIN users u ON u.id = lr.user_id AND u.deleted_at IS NULL
			WHERE lr.timeframe = $1
		)
		SELECT ranked_return_percentage FROM ranked WHERE rnk = $2 LIMIT 1
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
	`, string(tf)).Scan(&n)
	return n, err
}

// OldestComputedAt reports the least-recently-refreshed row's timestamp for
// tf, the staleness signal Service.rankingFresh uses to decide whether to
// trust this projection over live computation. found=false means tf has no
// rows yet (never refreshed).
func (r *PostgresRankingStore) OldestComputedAt(ctx context.Context, tf Timeframe) (time.Time, bool, error) {
	var at *time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT MIN(computed_at) FROM leaderboard_rankings WHERE timeframe = $1
	`, string(tf)).Scan(&at)
	if err != nil {
		return time.Time{}, false, err
	}
	if at == nil {
		return time.Time{}, false, nil
	}
	return *at, true, nil
}
