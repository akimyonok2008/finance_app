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

func (r *PostgresRepository) GetByHandle(ctx context.Context, handle string) (Profile, error) {
	return scanProfile(r.pool.QueryRow(ctx, `
		SELECT user_id, handle, display_name, avatar_key, bio, strategy_tag,
			is_public, show_public_weights, created_at, updated_at
		FROM profiles WHERE handle = $1
	`, handle))
}

func (r *PostgresRepository) ListPublicProfiles(ctx context.Context) ([]Profile, error) {
	// TODO: accept ExploreFilter and push search/symbol/sort/pagination into SQL
	// (plus a materialized public-card view + cached trending holdings) once the
	// public-profile set is large enough that fetching all rows is wasteful.
	rows, err := r.pool.Query(ctx, `
		SELECT user_id, handle, display_name, avatar_key, bio, strategy_tag,
			is_public, show_public_weights, created_at, updated_at
		FROM profiles WHERE is_public = true
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
