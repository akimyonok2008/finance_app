package performance

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// StateReader is the READ side of ranked-performance state.
//
// There is deliberately no write method here. Ranked state is written only
// inside the portfolio aggregate transaction (portfolio.AggregateTx), together
// with the position mutation it belongs to. Removing the standalone write API
// removes the possibility of reintroducing a non-atomic checkpoint.
type StateReader interface {
	// GetByPortfolio returns the portfolio's ranked state, or ErrStateNotFound.
	GetByPortfolio(ctx context.Context, portfolioID string) (*State, error)
}

// MutableRepository is retained for the current portfolio checkpoint path.
// PostgreSQL aggregate transactions can replace this compatibility boundary
// once the in-progress aggregate-store implementation is fully wired.
type MutableRepository interface {
	StateReader
	Create(ctx context.Context, state State) error
	Update(ctx context.Context, state State, expectedVersion int64) error
}

// PostgresStateReader reads ranked_performance_state (migration 0009) through
// the connection pool. Every call takes the caller's context.
type PostgresStateReader struct {
	pool *pgxpool.Pool
}

func NewPostgresStateReader(pool *pgxpool.Pool) *PostgresStateReader {
	return &PostgresStateReader{pool: pool}
}

func (r *PostgresStateReader) GetByPortfolio(ctx context.Context, portfolioID string) (*State, error) {
	st, err := ScanState(ctx, r.pool, portfolioID)
	if err != nil {
		return nil, err
	}
	return st, nil
}

func (r *PostgresStateReader) Create(ctx context.Context, state State) error {
	if err := ValidateState(state); err != nil {
		return err
	}
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO ranked_performance_state (
			portfolio_id, user_id, checkpoint_index, segment_start_value_base,
			status, tracking_started_at, segment_started_at, updated_at, version
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (portfolio_id) DO NOTHING
	`, state.PortfolioID, state.UserID, state.CheckpointIndex,
		state.SegmentStartValueBase, string(state.Status), state.TrackingStartedAt,
		state.SegmentStartedAt, state.UpdatedAt, state.Version)
	if err != nil {
		return fmt.Errorf("performance: create ranked state: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrVersionConflict
	}
	return nil
}

func (r *PostgresStateReader) Update(ctx context.Context, state State, expectedVersion int64) error {
	if err := ValidateState(state); err != nil {
		return err
	}
	tag, err := r.pool.Exec(ctx, `
		UPDATE ranked_performance_state
		SET checkpoint_index=$3, segment_start_value_base=$4, status=$5,
		    segment_started_at=$6, updated_at=$7, version=$8
		WHERE portfolio_id=$1 AND version=$2
	`, state.PortfolioID, expectedVersion, state.CheckpointIndex,
		state.SegmentStartValueBase, string(state.Status), state.SegmentStartedAt,
		state.UpdatedAt, state.Version)
	if err != nil {
		return fmt.Errorf("performance: update ranked state: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrVersionConflict
	}
	return nil
}

// RowQuerier is the minimal query surface shared by *pgxpool.Pool and pgx.Tx, so
// ranked state can be read either through the pool or inside the aggregate
// transaction without duplicating SQL.
type RowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// ScanState loads ranked state through any pool/transaction querier.
func ScanState(ctx context.Context, q RowQuerier, portfolioID string) (*State, error) {
	var (
		st     State
		status string
	)
	err := q.QueryRow(ctx, `
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
		return nil, fmt.Errorf("performance: read ranked state: %w", err)
	}
	st.Status = Status(status)
	return &st, nil
}
