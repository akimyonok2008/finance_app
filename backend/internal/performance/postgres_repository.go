package performance

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresRepository is the durable implementation of Repository, backed by the
// ranked_performance_state table (migration 0009). Optimistic concurrency is
// enforced with a version column: Update matches on (portfolio_id, version) and
// bumps the version, so a lost update affects zero rows and surfaces as
// ErrVersionConflict.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) GetByPortfolio(ctx context.Context, portfolioID string) (*State, error) {
	var (
		st     State
		status string
	)
	err := r.pool.QueryRow(ctx, `
		SELECT portfolio_id, user_id, checkpoint_index, segment_start_value_base,
		       status, tracking_started_at, segment_started_at, updated_at, version
		FROM ranked_performance_state
		WHERE portfolio_id = $1
	`, portfolioID).Scan(
		&st.PortfolioID, &st.UserID, &st.CheckpointIndex, &st.SegmentStartValueBase,
		&status, &st.TrackingStartedAt, &st.SegmentStartedAt, &st.UpdatedAt, &st.Version,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrStateNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("performance repository: get state: %w", err)
	}
	st.Status = Status(status)
	return &st, nil
}

func (r *PostgresRepository) Create(ctx context.Context, state State) error {
	if err := validate(state); err != nil {
		return err
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO ranked_performance_state (
			portfolio_id, user_id, checkpoint_index, segment_start_value_base,
			status, tracking_started_at, segment_started_at, updated_at, version
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (portfolio_id) DO NOTHING
	`,
		state.PortfolioID, state.UserID, state.CheckpointIndex, state.SegmentStartValueBase,
		string(state.Status), state.TrackingStartedAt, state.SegmentStartedAt, state.UpdatedAt, state.Version,
	)
	if err != nil {
		return fmt.Errorf("performance repository: create state: %w", err)
	}
	// ON CONFLICT DO NOTHING means a row already existed: treat as a race the
	// caller should resolve by re-reading and updating.
	var exists bool
	if err := r.pool.QueryRow(ctx, `SELECT version = $2 FROM ranked_performance_state WHERE portfolio_id = $1`,
		state.PortfolioID, state.Version).Scan(&exists); err != nil {
		return fmt.Errorf("performance repository: verify create: %w", err)
	}
	if !exists {
		return ErrVersionConflict
	}
	return nil
}

func (r *PostgresRepository) Update(ctx context.Context, state State, expectedVersion int64) error {
	if err := validate(state); err != nil {
		return err
	}
	tag, err := r.pool.Exec(ctx, `
		UPDATE ranked_performance_state
		SET checkpoint_index = $3, segment_start_value_base = $4, status = $5,
		    segment_started_at = $6, updated_at = $7, version = $8
		WHERE portfolio_id = $1 AND version = $2
	`,
		state.PortfolioID, expectedVersion, state.CheckpointIndex, state.SegmentStartValueBase,
		string(state.Status), state.SegmentStartedAt, state.UpdatedAt, state.Version,
	)
	if err != nil {
		return fmt.Errorf("performance repository: update state: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrVersionConflict
	}
	return nil
}
