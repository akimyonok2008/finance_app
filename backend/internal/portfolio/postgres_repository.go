package portfolio

import (
	"context"
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

const positionColumns = `id, user_id, portfolio_id, symbol, asset_type, quantity, average_buy_price, currency, created_at, updated_at`

func scanPosition(row pgx.Row) (*Position, error) {
	var p Position
	err := row.Scan(&p.ID, &p.UserID, &p.PortfolioID, &p.Symbol, &p.AssetType,
		&p.Quantity, &p.AverageBuyPrice, &p.Currency, &p.CreatedAt, &p.UpdatedAt)
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
		`INSERT INTO positions (`+positionColumns+`)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		p.ID, p.UserID, p.PortfolioID, p.Symbol, p.AssetType,
		p.Quantity, p.AverageBuyPrice, p.Currency, p.CreatedAt, p.UpdatedAt,
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
			&p.Quantity, &p.AverageBuyPrice, &p.Currency, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("portfolio repository: scan position row: %w", err)
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}

// UpdatePosition replaces the mutable fields of a position.
func (r *PostgresRepository) UpdatePosition(p *Position) error {
	tag, err := r.pool.Exec(context.Background(),
		`UPDATE positions
		 SET symbol = $2, asset_type = $3, quantity = $4, average_buy_price = $5,
		     currency = $6, updated_at = $7
		 WHERE id = $1`,
		p.ID, p.Symbol, p.AssetType, p.Quantity, p.AverageBuyPrice, p.Currency, p.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("portfolio repository: update position: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrPositionNotFound
	}
	return nil
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
