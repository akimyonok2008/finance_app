package portfolio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresRepository is the durable implementation of Repository and
// AggregateStore. Every method takes the caller's context — none fabricates a
// context.Background() — so cancellation, deadlines and tracing propagate and a
// client disconnect rolls the transaction back.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository wires a Postgres-backed portfolio repository.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// EnsureDefaultPortfolio returns the user's single default portfolio, creating
// it on first access. The insert relies on UNIQUE (user_id) (migration 0010):
// on conflict it does nothing and the row is re-read, so two concurrent first
// requests always converge on the same portfolio instead of creating two.
func (r *PostgresRepository) EnsureDefaultPortfolio(ctx context.Context, userID string) (*Portfolio, error) {
	if pf, err := r.GetPortfolioByUser(ctx, userID); err == nil {
		return pf, nil
	} else if !errors.Is(err, ErrPortfolioNotFound) {
		return nil, err
	}

	now := time.Now().UTC()
	if _, err := r.pool.Exec(ctx,
		`INSERT INTO portfolios (id, user_id, name, currency, version, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,1,$5,$6)
		 ON CONFLICT (user_id) DO NOTHING`,
		uuid.NewString(), userID, DefaultPortfolioName, "USD", now, now,
	); err != nil {
		return nil, fmt.Errorf("portfolio: create default portfolio: %w", err)
	}
	// Re-read: this returns OUR row if we inserted, or the winner's row if a
	// concurrent request got there first.
	return r.GetPortfolioByUser(ctx, userID)
}

// GetPortfolioByUser returns the user's (single default) portfolio.
func (r *PostgresRepository) GetPortfolioByUser(ctx context.Context, userID string) (*Portfolio, error) {
	var p Portfolio
	err := r.pool.QueryRow(ctx,
		`SELECT id, user_id, name, currency, COALESCE(version,1), created_at, updated_at
		 FROM portfolios WHERE user_id = $1 ORDER BY created_at LIMIT 1`, userID,
	).Scan(&p.ID, &p.UserID, &p.Name, &p.Currency, &p.Version, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPortfolioNotFound
		}
		return nil, fmt.Errorf("portfolio: get portfolio: %w", err)
	}
	return &p, nil
}

const positionColumns = `id, user_id, portfolio_id, symbol, asset_type, quantity, average_buy_price, currency,
	COALESCE(status, 'open'), closed_at, close_price, COALESCE(close_price_currency, ''),
	COALESCE(realized_gain_loss_base, 0), COALESCE(realized_gain_loss_percentage, 0),
	created_at, updated_at`

func scanPosition(row pgx.Row) (*Position, error) {
	var p Position
	err := row.Scan(&p.ID, &p.UserID, &p.PortfolioID, &p.Symbol, &p.AssetType,
		&p.Quantity, &p.AverageBuyPrice, &p.Currency, &p.Status, &p.ClosedAt,
		&p.ClosePrice, &p.CloseCurrency, &p.RealizedGainLossBase,
		&p.RealizedGainLossPercentage, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPositionNotFound
		}
		return nil, fmt.Errorf("portfolio: scan position: %w", err)
	}
	return &p, nil
}

// scanPositions runs a position query through any pool/transaction querier.
func scanPositions(ctx context.Context, q DBTX, sql string, args ...any) ([]*Position, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("portfolio: list positions: %w", err)
	}
	defer rows.Close()

	out := make([]*Position, 0)
	for rows.Next() {
		var p Position
		if err := rows.Scan(&p.ID, &p.UserID, &p.PortfolioID, &p.Symbol, &p.AssetType,
			&p.Quantity, &p.AverageBuyPrice, &p.Currency, &p.Status, &p.ClosedAt,
			&p.ClosePrice, &p.CloseCurrency, &p.RealizedGainLossBase,
			&p.RealizedGainLossPercentage, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("portfolio: scan position row: %w", err)
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}

// GetPosition returns a position by id.
func (r *PostgresRepository) GetPosition(ctx context.Context, id string) (*Position, error) {
	return scanPosition(r.pool.QueryRow(ctx, `SELECT `+positionColumns+` FROM positions WHERE id = $1`, id))
}

// ListPositionsByUser returns the user's positions in insertion order.
func (r *PostgresRepository) ListPositionsByUser(ctx context.Context, userID string) ([]*Position, error) {
	return scanPositions(ctx, r.pool,
		`SELECT `+positionColumns+` FROM positions WHERE user_id = $1 ORDER BY created_at`, userID)
}

func (r *PostgresRepository) ListOpenPositionsBySymbol(ctx context.Context, symbol string) ([]*Position, error) {
	return scanPositions(ctx, r.pool,
		`SELECT `+positionColumns+` FROM positions WHERE symbol = $1 AND COALESCE(status, 'open') = 'open' ORDER BY created_at`, symbol)
}

func (r *PostgresRepository) ListActiveSymbols(ctx context.Context) ([]string, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT DISTINCT symbol FROM positions WHERE COALESCE(status, 'open') = 'open' ORDER BY symbol`)
	if err != nil {
		return nil, fmt.Errorf("portfolio: list active symbols: %w", err)
	}
	defer rows.Close()
	out := make([]string, 0)
	for rows.Next() {
		var symbol string
		if err := rows.Scan(&symbol); err != nil {
			return nil, fmt.Errorf("portfolio: scan active symbol: %w", err)
		}
		out = append(out, symbol)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) ListCashBalances(ctx context.Context, userID string) ([]CashBalance, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT c.portfolio_id, c.currency, c.amount, c.created_at, c.updated_at
		FROM portfolio_cash_balances c
		JOIN portfolios p ON p.id=c.portfolio_id
		WHERE p.user_id=$1
		ORDER BY c.currency`, userID)
	if err != nil {
		return nil, fmt.Errorf("portfolio: list cash balances: %w", err)
	}
	defer rows.Close()
	out := make([]CashBalance, 0)
	for rows.Next() {
		var balance CashBalance
		if err := rows.Scan(&balance.PortfolioID, &balance.Currency, &balance.Amount,
			&balance.CreatedAt, &balance.UpdatedAt); err != nil {
			return nil, fmt.Errorf("portfolio: scan cash balance: %w", err)
		}
		out = append(out, balance)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) ListActivities(ctx context.Context, userID string, limit int) ([]Activity, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 100000 {
		limit = 100000
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, COALESCE(request_id,''), portfolio_id, user_id, activity_type,
		       COALESCE(symbol,''), COALESCE(asset_type,''), currency, quantity,
		       unit_price, gross_amount, cost_basis_allocated,
		       realized_gain_loss_base, realized_gain_loss_percentage,
		       occurred_at, portfolio_version, metadata_json, created_at,
		       position_episode_id
		FROM portfolio_activities
		WHERE user_id=$1
		ORDER BY occurred_at DESC, created_at DESC
		LIMIT $2`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("portfolio: list activities: %w", err)
	}
	defer rows.Close()
	out := make([]Activity, 0)
	for rows.Next() {
		var activity Activity
		var activityType string
		var metadata []byte
		var episodeID *string
		if err := rows.Scan(&activity.ID, &activity.RequestID, &activity.PortfolioID,
			&activity.UserID, &activityType, &activity.Symbol, &activity.AssetType,
			&activity.Currency, &activity.Quantity, &activity.UnitPrice,
			&activity.GrossAmount, &activity.CostBasisAllocated,
			&activity.RealizedGainLossBase, &activity.RealizedGainLossPercentage,
			&activity.OccurredAt, &activity.PortfolioVersion, &metadata,
			&activity.CreatedAt, &episodeID); err != nil {
			return nil, fmt.Errorf("portfolio: scan activity: %w", err)
		}
		activity.Type = ActivityType(activityType)
		_ = json.Unmarshal(metadata, &activity.Metadata)
		if episodeID != nil {
			activity.PositionEpisodeID = *episodeID
		}
		out = append(out, activity)
	}
	return out, rows.Err()
}

// CreateArchiveSnapshot inserts at most one snapshot per portfolio per UTC day.
// Uniqueness is enforced by the database (migration 0010), so concurrent workers
// cannot create duplicates; inserted=false means today's row already existed.
func (r *PostgresRepository) CreateArchiveSnapshot(ctx context.Context, s *PortfolioArchiveSnapshot) (bool, error) {
	positionsJSON, err := json.Marshal(s.Positions)
	if err != nil {
		return false, fmt.Errorf("portfolio: marshal archive positions: %w", err)
	}
	closedJSON, err := json.Marshal(s.ClosedPositions)
	if err != nil {
		return false, fmt.Errorf("portfolio: marshal archive closed positions: %w", err)
	}
	cashJSON, err := json.Marshal(s.CashBalances)
	if err != nil {
		return false, fmt.Errorf("portfolio: marshal archive cash: %w", err)
	}
	tag, err := r.pool.Exec(ctx,
		`INSERT INTO portfolio_archive_snapshots (
			id, user_id, portfolio_id, captured_at, base_currency, portfolio_index,
			gain_loss_percentage, total_cost_basis, current_value,
			unrealized_gain_loss_base, realized_gain_loss_base,
			positions_snapshot, closed_positions_snapshot, total_cash_value_base,
			cash_balances_snapshot
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		ON CONFLICT (portfolio_id, captured_date) DO NOTHING`,
		s.ID, s.UserID, s.PortfolioID, s.CapturedAt, s.BaseCurrency, s.PortfolioIndex,
		s.GainLossPercentage, s.TotalCostBasis, s.CurrentValue, s.UnrealizedGainLossBase,
		s.RealizedGainLossBase, positionsJSON, closedJSON, s.TotalCashValueBase, cashJSON,
	)
	if err != nil {
		return false, fmt.Errorf("portfolio: create archive snapshot: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (r *PostgresRepository) ListArchiveSnapshots(ctx context.Context, userID string, from, to string) ([]*PortfolioArchiveSnapshot, error) {
	fromTime, err := timeFromRFC3339(from)
	if err != nil {
		return nil, err
	}
	toTime, err := timeFromRFC3339(to)
	if err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, portfolio_id, captured_at, base_currency, portfolio_index,
		        gain_loss_percentage, total_cost_basis, current_value,
		        unrealized_gain_loss_base, realized_gain_loss_base,
		        positions_snapshot, closed_positions_snapshot, total_cash_value_base,
		        cash_balances_snapshot
		 FROM portfolio_archive_snapshots
		 WHERE user_id = $1 AND captured_at >= $2 AND captured_at <= $3
		 ORDER BY captured_at`, userID, fromTime, toTime)
	if err != nil {
		return nil, fmt.Errorf("portfolio: list archive snapshots: %w", err)
	}
	defer rows.Close()

	out := make([]*PortfolioArchiveSnapshot, 0)
	for rows.Next() {
		var (
			s                                   PortfolioArchiveSnapshot
			positionsJSON, closedJSON, cashJSON []byte
		)
		if err := rows.Scan(&s.ID, &s.UserID, &s.PortfolioID, &s.CapturedAt, &s.BaseCurrency,
			&s.PortfolioIndex, &s.GainLossPercentage, &s.TotalCostBasis, &s.CurrentValue,
			&s.UnrealizedGainLossBase, &s.RealizedGainLossBase, &positionsJSON,
			&closedJSON, &s.TotalCashValueBase, &cashJSON); err != nil {
			return nil, fmt.Errorf("portfolio: scan archive snapshot: %w", err)
		}
		_ = json.Unmarshal(positionsJSON, &s.Positions)
		_ = json.Unmarshal(closedJSON, &s.ClosedPositions)
		_ = json.Unmarshal(cashJSON, &s.CashBalances)
		out = append(out, &s)
	}
	return out, rows.Err()
}
