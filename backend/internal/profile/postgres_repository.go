package profile

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

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) Create(ctx context.Context, p Profile) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO profiles (
			user_id, handle, display_name, avatar_key, bio, strategy_tag,
			is_public, show_public_weights, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	`, p.UserID, p.Handle, p.DisplayName, p.AvatarKey, p.Bio, p.StrategyTag,
		p.IsPublic, p.ShowPublicWeights, p.CreatedAt, p.UpdatedAt)
	return mapPostgresError(err)
}

func (r *PostgresRepository) GetByUserID(ctx context.Context, userID string) (Profile, error) {
	return scanProfile(r.pool.QueryRow(ctx, `
		SELECT user_id, handle, display_name, avatar_key, bio, strategy_tag,
			is_public, show_public_weights, created_at, updated_at
		FROM profiles WHERE user_id = $1
	`, userID))
}

// GetByUserIDs batch-fetches profiles for exactly the given IDs in one query
// — used to enrich a leaderboard page without one round trip per row. Missing
// IDs (no profile row) are simply absent from the result map.
func (r *PostgresRepository) GetByUserIDs(ctx context.Context, userIDs []string) (map[string]Profile, error) {
	out := make(map[string]Profile, len(userIDs))
	if len(userIDs) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT user_id, handle, display_name, avatar_key, bio, strategy_tag,
			is_public, show_public_weights, created_at, updated_at
		FROM profiles WHERE user_id = ANY($1)
	`, userIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}
		out[p.UserID] = p
	}
	return out, rows.Err()
}

func (r *PostgresRepository) GetByHandle(ctx context.Context, handle string) (Profile, error) {
	return scanProfile(r.pool.QueryRow(ctx, `
		SELECT user_id, handle, display_name, avatar_key, bio, strategy_tag,
			is_public, show_public_weights, created_at, updated_at
		FROM profiles WHERE handle = $1
	`, handle))
}

func (r *PostgresRepository) ListPublicProfiles(ctx context.Context) ([]Profile, error) {
	// show_public_weights=false is pushed down too: every caller immediately
	// discards those rows. This full scan is performed only by the
	// leader-elected background projection refresh; request handling reads the
	// bounded explore_public_cards projection below.
	rows, err := r.pool.Query(ctx, `
		SELECT user_id, handle, display_name, avatar_key, bio, strategy_tag,
			is_public, show_public_weights, created_at, updated_at
		FROM profiles WHERE is_public = true AND show_public_weights = true
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Profile, 0)
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ReplaceExploreProjection atomically swaps a complete privacy-safe Explore
// generation. Readers keep seeing the previous generation until every new row
// has been copied and the state pointer advances in the same transaction.
func (r *PostgresRepository) ReplaceExploreProjection(ctx context.Context, rows []ExploreProjectionRow) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var active int64
	if err := tx.QueryRow(ctx, `
		SELECT active_generation
		FROM explore_projection_state
		WHERE id = TRUE
		FOR UPDATE
	`).Scan(&active); err != nil {
		return err
	}
	next := active + 1
	if _, err := tx.Exec(ctx, `DELETE FROM explore_public_cards WHERE generation = $1`, next); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM explore_trending_holdings WHERE generation = $1`, next); err != nil {
		return err
	}

	copyRows := make([][]any, 0, len(rows))
	for _, row := range rows {
		card, err := json.Marshal(row.Card)
		if err != nil {
			return err
		}
		copyRows = append(copyRows, []any{
			next, row.Timeframe, row.UserID, row.UpdatedAt.UTC(), string(card),
		})
	}
	if len(copyRows) > 0 {
		if _, err := tx.CopyFrom(
			ctx,
			pgx.Identifier{"explore_public_cards"},
			[]string{"generation", "timeframe", "user_id", "updated_at", "card"},
			pgx.CopyFromRows(copyRows),
		); err != nil {
			return err
		}
	}
	trending := buildExploreTrendingProjection(rows)
	trendingRows := make([][]any, 0, len(trending))
	for _, row := range trending {
		trendingRows = append(trendingRows, []any{
			next, row.Timeframe, row.Symbol, row.ProfileCount,
			row.WeightSum, row.Top10Count, row.AssetType,
		})
	}
	if len(trendingRows) > 0 {
		if _, err := tx.CopyFrom(
			ctx,
			pgx.Identifier{"explore_trending_holdings"},
			[]string{
				"generation", "timeframe", "symbol", "profile_count",
				"weight_sum", "top10_count", "asset_type",
			},
			pgx.CopyFromRows(trendingRows),
		); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE explore_projection_state
		SET active_generation = $1, updated_at = now()
		WHERE id = TRUE
	`, next); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM explore_public_cards WHERE generation <> $1`, next); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM explore_trending_holdings WHERE generation <> $1`, next); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func exploreOrderClause(sortMode string) string {
	switch sortMode {
	case SortReturn:
		return `(projected.card->>'return_percentage')::NUMERIC DESC, projected.card->>'handle' ASC`
	case SortRecent:
		return `projected.updated_at DESC, projected.card->>'handle' ASC`
	case SortRank, SortTop:
		return `(projected.card->>'global_rank')::INTEGER ASC, projected.card->>'handle' ASC`
	default:
		return `(projected.card->>'global_rank')::INTEGER ASC, projected.card->>'handle' ASC`
	}
}

// LoadExploreProjection executes all request-size-sensitive Explore work in a
// repeatable-read transaction against one active generation. Only the requested
// page and a bounded recommendation pool are decoded into Go.
func (r *PostgresRepository) LoadExploreProjection(
	ctx context.Context,
	filter ExploreFilter,
	blockedUserIDs []string,
	candidateLimit int,
) (ExploreProjectionSnapshot, bool, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return ExploreProjectionSnapshot{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var generation int64
	var updatedAt *time.Time
	if err := tx.QueryRow(ctx, `
		SELECT active_generation, updated_at
		FROM explore_projection_state
		WHERE id = TRUE
	`).Scan(&generation, &updatedAt); err != nil {
		return ExploreProjectionSnapshot{}, false, err
	}
	if updatedAt == nil {
		return ExploreProjectionSnapshot{}, false, nil
	}

	const filteredPredicate = `
		projected.generation = $1
		AND projected.timeframe = $2
		AND current_profile.is_public = TRUE
		AND current_profile.show_public_weights = TRUE
		AND (projected.card->>'handle') <> ''
		AND (projected.card->'public_weights') <> '[]'::jsonb
		AND (projected.card->>'global_rank') IS NOT NULL
		AND (projected.card->>'global_rank') <> ''
		AND (projected.card->>'portfolio_index') IS NOT NULL
		AND (projected.card->>'return_percentage') IS NOT NULL
		AND (COALESCE(array_length($3::text[], 1), 0) = 0 OR projected.user_id::text <> ALL($3::text[]))
		AND (
			$4 = ''
			OR strpos(lower(projected.card->>'handle'), lower($4)) > 0
			OR strpos(lower(projected.card->>'display_name'), lower($4)) > 0
			OR EXISTS (
				SELECT 1
				FROM jsonb_array_elements(projected.card->'public_weights') weight
				WHERE strpos(lower(weight->>'symbol'), lower($4)) > 0
			)
		)
		AND (
			$5 = ''
			OR EXISTS (
				SELECT 1
				FROM jsonb_array_elements(projected.card->'public_weights') weight
				WHERE weight->>'symbol' = $5
			)
		)`
	args := []any{generation, filter.Timeframe, blockedUserIDs, filter.Query, filter.Symbol}

	var total int
	if err := tx.QueryRow(ctx,
		`SELECT COUNT(*)
		 FROM explore_public_cards projected
		 JOIN profiles current_profile ON current_profile.user_id = projected.user_id
		 WHERE `+filteredPredicate,
		args...,
	).Scan(&total); err != nil {
		return ExploreProjectionSnapshot{}, false, err
	}

	pageQuery := fmt.Sprintf(`
		SELECT projected.user_id::text, projected.updated_at, projected.card
		FROM explore_public_cards projected
		JOIN profiles current_profile ON current_profile.user_id = projected.user_id
		WHERE %s
		ORDER BY %s
		LIMIT $6 OFFSET $7
	`, filteredPredicate, exploreOrderClause(filter.Sort))
	page, err := scanExploreRows(ctx, tx, pageQuery, append(args, filter.Limit, filter.Offset)...)
	if err != nil {
		return ExploreProjectionSnapshot{}, false, err
	}

	candidates, err := scanExploreRows(ctx, tx, `
		SELECT projected.user_id::text, projected.updated_at, projected.card
		FROM explore_public_cards projected
		JOIN profiles current_profile ON current_profile.user_id = projected.user_id
		WHERE projected.generation = $1
			AND projected.timeframe = $2
			AND current_profile.is_public = TRUE
			AND current_profile.show_public_weights = TRUE
			AND (COALESCE(array_length($3::text[], 1), 0) = 0 OR projected.user_id::text <> ALL($3::text[]))
		ORDER BY (projected.card->>'global_rank')::INTEGER ASC, projected.card->>'handle' ASC
		LIMIT $4
	`, generation, filter.Timeframe, blockedUserIDs, candidateLimit)
	if err != nil {
		return ExploreProjectionSnapshot{}, false, err
	}

	trendingRows, err := tx.Query(ctx, `
		WITH excluded AS (
			SELECT weight->>'symbol' AS symbol,
			       COUNT(*)::INTEGER AS profile_count,
			       SUM((weight->>'weight')::DOUBLE PRECISION) AS weight_sum,
			       COUNT(*) FILTER (
			           WHERE (c.card->>'global_rank')::INTEGER <= 10
			       )::INTEGER AS top10_count
			FROM explore_public_cards c
			CROSS JOIN LATERAL jsonb_array_elements(c.card->'public_weights') weight
			WHERE c.generation = $1
				AND c.timeframe = $2
				AND c.user_id::text = ANY($3::text[])
			GROUP BY weight->>'symbol'
		),
		adjusted AS (
			SELECT base.symbol,
			       base.profile_count - COALESCE(excluded.profile_count, 0) AS profile_count,
			       base.weight_sum - COALESCE(excluded.weight_sum, 0) AS weight_sum,
			       base.top10_count - COALESCE(excluded.top10_count, 0) AS top10_count,
			       base.asset_type
			FROM explore_trending_holdings base
			LEFT JOIN excluded ON excluded.symbol = base.symbol
			WHERE base.generation = $1 AND base.timeframe = $2
		)
		SELECT symbol, profile_count, weight_sum / profile_count, top10_count, asset_type
		FROM adjusted
		WHERE profile_count > 0
		ORDER BY profile_count DESC, (weight_sum / profile_count) DESC, symbol ASC
		LIMIT $4
	`, generation, filter.Timeframe, blockedUserIDs, maxTrendingHoldings)
	if err != nil {
		return ExploreProjectionSnapshot{}, false, err
	}
	defer trendingRows.Close()
	trending := make([]TrendingHolding, 0, maxTrendingHoldings)
	for trendingRows.Next() {
		var item TrendingHolding
		if err := trendingRows.Scan(
			&item.Symbol, &item.ProfileCount, &item.AverageWeight, &item.Top10Count, &item.AssetType,
		); err != nil {
			return ExploreProjectionSnapshot{}, false, err
		}
		item.AverageWeight = round2(item.AverageWeight)
		trending = append(trending, item)
	}
	if err := trendingRows.Err(); err != nil {
		return ExploreProjectionSnapshot{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ExploreProjectionSnapshot{}, false, err
	}
	return ExploreProjectionSnapshot{
		Page: page, Candidates: candidates, Trending: trending, Total: total,
	}, true, nil
}

type exploreQueryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func scanExploreRows(ctx context.Context, q exploreQueryer, sql string, args ...any) ([]ExploreProjectionRow, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ExploreProjectionRow, 0)
	for rows.Next() {
		var row ExploreProjectionRow
		var card []byte
		if err := rows.Scan(&row.UserID, &row.UpdatedAt, &card); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(card, &row.Card); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) Update(ctx context.Context, p Profile) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE profiles SET
			handle=$2, display_name=$3, avatar_key=$4, bio=$5, strategy_tag=$6,
			is_public=$7, show_public_weights=$8, updated_at=$9
		WHERE user_id=$1
	`, p.UserID, p.Handle, p.DisplayName, p.AvatarKey, p.Bio, p.StrategyTag,
		p.IsPublic, p.ShowPublicWeights, p.UpdatedAt)
	if err != nil {
		return mapPostgresError(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanProfile(row rowScanner) (Profile, error) {
	var p Profile
	err := row.Scan(&p.UserID, &p.Handle, &p.DisplayName, &p.AvatarKey, &p.Bio,
		&p.StrategyTag, &p.IsPublic, &p.ShowPublicWeights, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, ErrNotFound
	}
	return p, err
}

func mapPostgresError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrHandleExists
	}
	return err
}
