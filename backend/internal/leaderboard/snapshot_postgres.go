package leaderboard

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresSnapshotStore persists index snapshots in Postgres (table
// leaderboard_snapshots, migration 0003).
type PostgresSnapshotStore struct {
	pool *pgxpool.Pool
}

func NewPostgresSnapshotStore(pool *pgxpool.Pool) *PostgresSnapshotStore {
	return &PostgresSnapshotStore{pool: pool}
}

func (s *PostgresSnapshotStore) Record(ctx context.Context, userID string, index float64, at time.Time) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO leaderboard_snapshots (user_id, portfolio_index, captured_at)
		VALUES ($1, $2, $3)
	`, userID, index, at.UTC())
	return err
}

func (s *PostgresSnapshotStore) IndexAtOrBefore(ctx context.Context, userID string, cutoff, notBefore time.Time) (float64, time.Time, bool, error) {
	var index float64
	var capturedAt time.Time
	// The captured_at >= notBefore floor drops legacy pre-epoch snapshots; a zero
	// notBefore ('0001-01-01') is harmlessly below every real timestamp.
	err := s.pool.QueryRow(ctx, `
		SELECT portfolio_index, captured_at FROM leaderboard_snapshots
		WHERE user_id = $1 AND captured_at <= $2 AND captured_at >= $3
		ORDER BY captured_at DESC
		LIMIT 1
	`, userID, cutoff.UTC(), notBefore.UTC()).Scan(&index, &capturedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, time.Time{}, false, nil
	}
	if err != nil {
		return 0, time.Time{}, false, err
	}
	return index, capturedAt, true, nil
}
