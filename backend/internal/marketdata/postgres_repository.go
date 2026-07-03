package marketdata

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) SearchInstruments(ctx context.Context, query string, limit int) ([]Instrument, error) {
	q := strings.ToUpper(strings.TrimSpace(query))
	if limit <= 0 {
		limit = 10
	}
	rows, err := r.pool.Query(ctx, `
		SELECT symbol, display_symbol, name, exchange, country, currency, asset_type,
		       provider, provider_symbol, is_active, created_at, updated_at, last_seen_at
		FROM instruments
		WHERE is_active = true
		  AND (symbol ILIKE $1 OR display_symbol ILIKE $1 OR name ILIKE $1)
		ORDER BY
		  CASE
		    WHEN upper(symbol) = $2 THEN 0
		    WHEN upper(symbol) LIKE $3 THEN 1
		    WHEN upper(symbol) LIKE $1 THEN 2
		    ELSE 3
		  END,
		  symbol
		LIMIT $4`, "%"+q+"%", q, q+"%", limit)
	if err != nil {
		return nil, fmt.Errorf("marketdata repository: search instruments: %w", err)
	}
	defer rows.Close()

	out := make([]Instrument, 0)
	for rows.Next() {
		var item Instrument
		if err := rows.Scan(
			&item.Symbol, &item.DisplaySymbol, &item.Name, &item.Exchange, &item.Country,
			&item.Currency, &item.AssetType, &item.Provider, &item.ProviderSymbol,
			&item.IsActive, &item.CreatedAt, &item.UpdatedAt, &item.LastSeenAt,
		); err != nil {
			return nil, fmt.Errorf("marketdata repository: scan instrument: %w", err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) UpsertInstruments(ctx context.Context, instruments []Instrument) error {
	now := time.Now().UTC()
	for _, item := range instruments {
		item = normalizeInstrument(item, now)
		_, err := r.pool.Exec(ctx, `
			INSERT INTO instruments (
				symbol, display_symbol, name, exchange, country, currency, asset_type,
				provider, provider_symbol, is_active, created_at, updated_at, last_seen_at
			)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
			ON CONFLICT (symbol) DO UPDATE SET
				display_symbol = EXCLUDED.display_symbol,
				name = EXCLUDED.name,
				exchange = EXCLUDED.exchange,
				country = EXCLUDED.country,
				currency = EXCLUDED.currency,
				asset_type = EXCLUDED.asset_type,
				provider = EXCLUDED.provider,
				provider_symbol = EXCLUDED.provider_symbol,
				is_active = EXCLUDED.is_active,
				updated_at = EXCLUDED.updated_at,
				last_seen_at = EXCLUDED.last_seen_at`,
			item.Symbol, item.DisplaySymbol, item.Name, item.Exchange, item.Country,
			item.Currency, item.AssetType, item.Provider, item.ProviderSymbol, item.IsActive,
			item.CreatedAt, item.UpdatedAt, item.LastSeenAt,
		)
		if err != nil {
			return fmt.Errorf("marketdata repository: upsert instrument: %w", err)
		}
	}
	return nil
}

func (r *PostgresRepository) GetQuote(ctx context.Context, symbol string) (Quote, bool, error) {
	var q Quote
	err := r.pool.QueryRow(ctx, `
		SELECT symbol, price, currency, change_percentage, previous_close, market_time,
		       provider, is_delayed, delay_minutes, is_stale, fetched_at, expires_at,
		       provider_status, raw_provider_symbol, updated_at
		FROM market_quotes
		WHERE symbol = $1`, normalizeSymbol(symbol)).Scan(
		&q.Symbol, &q.Price, &q.Currency, &q.ChangePercentage, &q.PreviousClose, &q.MarketTime,
		&q.Provider, &q.IsDelayed, &q.DelayMinutes, &q.IsStale, &q.FetchedAt, &q.ExpiresAt,
		&q.ProviderStatus, &q.RawProviderSymbol, &q.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return Quote{}, false, nil
		}
		return Quote{}, false, fmt.Errorf("marketdata repository: get quote: %w", err)
	}
	return q, true, nil
}

func (r *PostgresRepository) UpsertQuotes(ctx context.Context, quotes []Quote) error {
	now := time.Now().UTC()
	for _, q := range quotes {
		q.Symbol = normalizeSymbol(q.Symbol)
		q.UpdatedAt = now
		_, err := r.pool.Exec(ctx, `
			INSERT INTO market_quotes (
				symbol, price, currency, change_percentage, previous_close, market_time,
				provider, is_delayed, delay_minutes, is_stale, fetched_at, expires_at,
				provider_status, raw_provider_symbol, updated_at
			)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
			ON CONFLICT (symbol) DO UPDATE SET
				price = EXCLUDED.price,
				currency = EXCLUDED.currency,
				change_percentage = EXCLUDED.change_percentage,
				previous_close = EXCLUDED.previous_close,
				market_time = EXCLUDED.market_time,
				provider = EXCLUDED.provider,
				is_delayed = EXCLUDED.is_delayed,
				delay_minutes = EXCLUDED.delay_minutes,
				is_stale = EXCLUDED.is_stale,
				fetched_at = EXCLUDED.fetched_at,
				expires_at = EXCLUDED.expires_at,
				provider_status = EXCLUDED.provider_status,
				raw_provider_symbol = EXCLUDED.raw_provider_symbol,
				updated_at = EXCLUDED.updated_at`,
			q.Symbol, q.Price, q.Currency, q.ChangePercentage, q.PreviousClose, q.MarketTime,
			q.Provider, q.IsDelayed, q.DelayMinutes, q.IsStale, q.FetchedAt, q.ExpiresAt,
			q.ProviderStatus, q.RawProviderSymbol, q.UpdatedAt,
		)
		if err != nil {
			return fmt.Errorf("marketdata repository: upsert quote: %w", err)
		}
	}
	return nil
}
