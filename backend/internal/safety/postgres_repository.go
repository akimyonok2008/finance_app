package safety

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) Block(ctx context.Context, blockerUserID, blockedUserID string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO user_blocks (blocker_user_id, blocked_user_id)
		VALUES ($1, $2)
		ON CONFLICT (blocker_user_id, blocked_user_id) DO NOTHING
	`, blockerUserID, blockedUserID)
	return err
}

func (r *PostgresRepository) Unblock(ctx context.Context, blockerUserID, blockedUserID string) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM user_blocks WHERE blocker_user_id = $1 AND blocked_user_id = $2
	`, blockerUserID, blockedUserID)
	return err
}

func (r *PostgresRepository) IsBlocked(ctx context.Context, blockerUserID, blockedUserID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM user_blocks WHERE blocker_user_id = $1 AND blocked_user_id = $2)
	`, blockerUserID, blockedUserID).Scan(&exists)
	return exists, err
}

func (r *PostgresRepository) IsBlockedEitherDirection(ctx context.Context, userA, userB string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM user_blocks
			WHERE (blocker_user_id = $1 AND blocked_user_id = $2)
			   OR (blocker_user_id = $2 AND blocked_user_id = $1)
		)
	`, userA, userB).Scan(&exists)
	return exists, err
}

func (r *PostgresRepository) ListBlocked(ctx context.Context, blockerUserID string) ([]Block, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT blocker_user_id, blocked_user_id, created_at
		FROM user_blocks
		WHERE blocker_user_id = $1
		ORDER BY created_at DESC
	`, blockerUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Block, 0)
	for rows.Next() {
		var b Block
		if err := rows.Scan(&b.BlockerUserID, &b.BlockedUserID, &b.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) BlockedPairUserIDs(ctx context.Context, userID string) (map[string]bool, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT blocked_user_id FROM user_blocks WHERE blocker_user_id = $1
		UNION
		SELECT blocker_user_id FROM user_blocks WHERE blocked_user_id = $1
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}
