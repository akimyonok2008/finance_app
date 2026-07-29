package profile

import (
	"context"
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
	// discards those rows (Explore requires public weights to build a card at
	// all), so there's no reason to fetch or scan them.
	//
	// TODO: accept ExploreFilter and push search/symbol/sort/pagination into
	// SQL (plus a materialized public-card view + cached trending holdings)
	// once the public-profile set is large enough that fetching all
	// (public, weights-visible) rows is wasteful. Symbol/composition data
	// itself isn't in this table (it's derived from live portfolio positions
	// per profile), so that part needs its own materialized projection, not
	// just a WHERE clause here.
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
