package safety

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresBlockCoordinator implements BlockCoordinator by performing the
// block insert and both-direction follow removal in a single transaction,
// guarded by a Postgres advisory lock keyed on the (unordered) user pair.
// The advisory lock is taken with the same key ordering used by
// social.PostgresRepository.Follow, so a concurrent Follow and Block for the
// same pair are serialized at the database level, not just in-process.
type PostgresBlockCoordinator struct {
	pool *pgxpool.Pool
}

func NewPostgresBlockCoordinator(pool *pgxpool.Pool) *PostgresBlockCoordinator {
	return &PostgresBlockCoordinator{pool: pool}
}

func (c *PostgresBlockCoordinator) BlockAndRemoveRelationships(ctx context.Context, blockerUserID, blockedUserID string) error {
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	lo, hi := blockerUserID, blockedUserID
	if hi < lo {
		lo, hi = hi, lo
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1), hashtext($2))`, lo, hi); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO user_blocks (blocker_user_id, blocked_user_id)
		VALUES ($1, $2)
		ON CONFLICT (blocker_user_id, blocked_user_id) DO NOTHING
	`, blockerUserID, blockedUserID); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		DELETE FROM follows
		WHERE (follower_user_id = $1 AND following_user_id = $2)
		   OR (follower_user_id = $2 AND following_user_id = $1)
	`, blockerUserID, blockedUserID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
