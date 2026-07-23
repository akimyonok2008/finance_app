package portfolio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresRepository is the durable implementation of Repository. NUMERIC
// columns are scanned into float64, which is acceptable at prototype precision
// (positions round to 8 decimal places).
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository wires a Postgres-backed portfolio repository.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// CreatePortfolio persists a portfolio.
func (r *PostgresRepository) CreatePortfolio(p *Portfolio) error {
	_, err := r.pool.Exec(context.Background(),
		`INSERT INTO portfolios (id, user_id, name, currency, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		p.ID, p.UserID, p.Name, p.Currency, p.CreatedAt, p.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("portfolio repository: create portfolio: %w", err)
	}
	return nil
}

// GetPortfolioByUser returns the user's (single default) portfolio.
func (r *PostgresRepository) GetPortfolioByUser(userID string) (*Portfolio, error) {
	var p Portfolio
	err := r.pool.QueryRow(context.Background(),
		`SELECT id, user_id, name, currency, created_at, updated_at
		 FROM portfolios WHERE user_id = $1 ORDER BY created_at LIMIT 1`, userID,
	).Scan(&p.ID, &p.UserID, &p.Name, &p.Currency, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPortfolioNotFound
		}
		return nil, fmt.Errorf("portfolio repository: get portfolio: %w", err)
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
		return nil, fmt.Errorf("portfolio repository: scan position: %w", err)
	}
	return &p, nil
}

// CreatePosition persists a position.
func (r *PostgresRepository) CreatePosition(p *Position) error {
	_, err := r.pool.Exec(context.Background(),
		`INSERT INTO positions (
			id, user_id, portfolio_id, symbol, asset_type, quantity, average_buy_price,
			currency, status, closed_at, close_price, close_price_currency,
			realized_gain_loss_base, realized_gain_loss_percentage, created_at, updated_at
		)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`,
		p.ID, p.UserID, p.PortfolioID, p.Symbol, p.AssetType,
		p.Quantity, p.AverageBuyPrice, p.Currency, firstNonEmptyStatus(p.Status),
		p.ClosedAt, p.ClosePrice, p.CloseCurrency, p.RealizedGainLossBase,
		p.RealizedGainLossPercentage, p.CreatedAt, p.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("portfolio repository: create position: %w", err)
	}
	return nil
}

// GetPosition returns a position by id.
func (r *PostgresRepository) GetPosition(id string) (*Position, error) {
	row := r.pool.QueryRow(context.Background(),
		`SELECT `+positionColumns+` FROM positions WHERE id = $1`, id)
	return scanPosition(row)
}

// ListPositionsByUser returns the user's positions in insertion order.
func (r *PostgresRepository) ListPositionsByUser(userID string) ([]*Position, error) {
	rows, err := r.pool.Query(context.Background(),
		`SELECT `+positionColumns+` FROM positions WHERE user_id = $1 ORDER BY created_at`, userID)
	if err != nil {
		return nil, fmt.Errorf("portfolio repository: list positions: %w", err)
	}
	defer rows.Close()

	out := make([]*Position, 0)
	for rows.Next() {
		var p Position
		if err := rows.Scan(&p.ID, &p.UserID, &p.PortfolioID, &p.Symbol, &p.AssetType,
			&p.Quantity, &p.AverageBuyPrice, &p.Currency, &p.Status, &p.ClosedAt,
			&p.ClosePrice, &p.CloseCurrency, &p.RealizedGainLossBase,
			&p.RealizedGainLossPercentage, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("portfolio repository: scan position row: %w", err)
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) ListActiveSymbols() ([]string, error) {
	rows, err := r.pool.Query(context.Background(), `SELECT DISTINCT symbol FROM positions WHERE COALESCE(status, 'open') = 'open' ORDER BY symbol`)
	if err != nil {
		return nil, fmt.Errorf("portfolio repository: list active symbols: %w", err)
	}
	defer rows.Close()
	out := make([]string, 0)
	for rows.Next() {
		var symbol string
		if err := rows.Scan(&symbol); err != nil {
			return nil, fmt.Errorf("portfolio repository: scan active symbol: %w", err)
		}
		out = append(out, symbol)
	}
	return out, rows.Err()
}

// UpdatePosition replaces the mutable fields of a position.
func (r *PostgresRepository) UpdatePosition(p *Position) error {
	tag, err := r.pool.Exec(context.Background(),
		`UPDATE positions
		 SET symbol = $2, asset_type = $3, quantity = $4, average_buy_price = $5,
		     currency = $6, status = $7, closed_at = $8, close_price = $9,
		     close_price_currency = $10, realized_gain_loss_base = $11,
		     realized_gain_loss_percentage = $12, updated_at = $13
		 WHERE id = $1`,
		p.ID, p.Symbol, p.AssetType, p.Quantity, p.AverageBuyPrice, p.Currency,
		firstNonEmptyStatus(p.Status), p.ClosedAt, p.ClosePrice, p.CloseCurrency,
		p.RealizedGainLossBase, p.RealizedGainLossPercentage, p.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("portfolio repository: update position: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrPositionNotFound
	}
	return nil
}

func (r *PostgresRepository) CreateArchiveSnapshot(s *PortfolioArchiveSnapshot) error {
	positionsJSON, err := json.Marshal(s.Positions)
	if err != nil {
		return fmt.Errorf("portfolio repository: marshal archive positions: %w", err)
	}
	closedJSON, err := json.Marshal(s.ClosedPositions)
	if err != nil {
		return fmt.Errorf("portfolio repository: marshal archive closed positions: %w", err)
	}
	_, err = r.pool.Exec(context.Background(),
		`INSERT INTO portfolio_archive_snapshots (
			id, user_id, portfolio_id, captured_at, base_currency, portfolio_index,
			gain_loss_percentage, total_cost_basis, current_value,
			unrealized_gain_loss_base, realized_gain_loss_base,
			positions_snapshot, closed_positions_snapshot
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		s.ID, s.UserID, s.PortfolioID, s.CapturedAt, s.BaseCurrency, s.PortfolioIndex,
		s.GainLossPercentage, s.TotalCostBasis, s.CurrentValue, s.UnrealizedGainLossBase,
		s.RealizedGainLossBase, positionsJSON, closedJSON,
	)
	if err != nil {
		return fmt.Errorf("portfolio repository: create archive snapshot: %w", err)
	}
	return nil
}

func (r *PostgresRepository) ListArchiveSnapshots(userID string, from, to string) ([]*PortfolioArchiveSnapshot, error) {
	rows, err := r.pool.Query(context.Background(),
		`SELECT id, user_id, portfolio_id, captured_at, base_currency, portfolio_index,
		        gain_loss_percentage, total_cost_basis, current_value,
		        unrealized_gain_loss_base, realized_gain_loss_base,
		        positions_snapshot, closed_positions_snapshot
		 FROM portfolio_archive_snapshots
		 WHERE user_id = $1 AND captured_at >= $2 AND captured_at <= $3
		 ORDER BY captured_at ASC`, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("portfolio repository: list archive snapshots: %w", err)
	}
	defer rows.Close()
	out := make([]*PortfolioArchiveSnapshot, 0)
	for rows.Next() {
		var s PortfolioArchiveSnapshot
		var positionsRaw, closedRaw []byte
		if err := rows.Scan(&s.ID, &s.UserID, &s.PortfolioID, &s.CapturedAt, &s.BaseCurrency,
			&s.PortfolioIndex, &s.GainLossPercentage, &s.TotalCostBasis, &s.CurrentValue,
			&s.UnrealizedGainLossBase, &s.RealizedGainLossBase, &positionsRaw, &closedRaw); err != nil {
			return nil, fmt.Errorf("portfolio repository: scan archive snapshot: %w", err)
		}
		if len(positionsRaw) > 0 {
			if err := json.Unmarshal(positionsRaw, &s.Positions); err != nil {
				return nil, fmt.Errorf("portfolio repository: unmarshal archive positions: %w", err)
			}
		}
		if len(closedRaw) > 0 {
			if err := json.Unmarshal(closedRaw, &s.ClosedPositions); err != nil {
				return nil, fmt.Errorf("portfolio repository: unmarshal archive closed positions: %w", err)
			}
		}
		out = append(out, &s)
	}
	return out, rows.Err()
}

// DeletePosition removes a position by id.
func (r *PostgresRepository) DeletePosition(id string) error {
	tag, err := r.pool.Exec(context.Background(), `DELETE FROM positions WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("portfolio repository: delete position: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrPositionNotFound
	}
	return nil
}

// ReplaceOpenPositions swaps the user's open positions inside a single
// transaction, so a strategy copy either fully lands or fully rolls back.
func (r *PostgresRepository) ReplaceOpenPositions(userID string, newPositions []*Position) error {
	ctx := context.Background()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("portfolio repository: begin replace tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`DELETE FROM positions WHERE user_id = $1 AND COALESCE(status, 'open') = 'open'`, userID); err != nil {
		return fmt.Errorf("portfolio repository: replace delete: %w", err)
	}
	for _, p := range newPositions {
		if _, err := tx.Exec(ctx,
			`INSERT INTO positions (
				id, user_id, portfolio_id, symbol, asset_type, quantity, average_buy_price,
				currency, status, closed_at, close_price, close_price_currency,
				realized_gain_loss_base, realized_gain_loss_percentage, created_at, updated_at
			)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`,
			p.ID, p.UserID, p.PortfolioID, p.Symbol, p.AssetType,
			p.Quantity, p.AverageBuyPrice, p.Currency, firstNonEmptyStatus(p.Status),
			p.ClosedAt, p.ClosePrice, p.CloseCurrency, p.RealizedGainLossBase,
			p.RealizedGainLossPercentage, p.CreatedAt, p.UpdatedAt,
		); err != nil {
			return fmt.Errorf("portfolio repository: replace insert: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("portfolio repository: commit replace tx: %w", err)
	}
	return nil
}
