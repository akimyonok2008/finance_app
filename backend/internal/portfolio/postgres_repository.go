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

	"github.com/ardakimyonok/finance_app/internal/money"
	"github.com/ardakimyonok/finance_app/internal/telemetry"
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

// EnsureDefaultPortfolio idempotently initializes the user's single default
// portfolio. The insert relies on UNIQUE (user_id) (migration 0010):
// on conflict it does nothing and the row is re-read, so two concurrent first
// requests always converge on the same portfolio instead of creating two.
func (r *PostgresRepository) EnsureDefaultPortfolio(ctx context.Context, userID string) (*Portfolio, error) {
	return r.EnsureDefaultPortfolioTx(ctx, r.pool, userID)
}

// EnsureDefaultPortfolioTx is EnsureDefaultPortfolio against an arbitrary
// dbtx.Querier, so a caller (e.g. auth's registration transaction) can create
// the default portfolio in the SAME transaction as the user row it belongs
// to, instead of as a separate, non-atomic step after that transaction
// commits.
func (r *PostgresRepository) EnsureDefaultPortfolioTx(ctx context.Context, q DBTX, userID string) (*Portfolio, error) {
	if pf, err := r.getPortfolioByUserTx(ctx, q, userID); err == nil {
		return pf, nil
	} else if !errors.Is(err, ErrPortfolioNotFound) {
		return nil, err
	}

	now := time.Now().UTC()
	if _, err := q.Exec(ctx,
		`INSERT INTO portfolios (id, user_id, name, currency, version, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,1,$5,$6)
		 ON CONFLICT (user_id) DO NOTHING`,
		uuid.NewString(), userID, DefaultPortfolioName, "USD", now, now,
	); err != nil {
		return nil, fmt.Errorf("portfolio: create default portfolio: %w", err)
	}
	// Re-read: this returns OUR row if we inserted, or the winner's row if a
	// concurrent request got there first.
	return r.getPortfolioByUserTx(ctx, q, userID)
}

// GetPortfolioByUser returns the user's (single default) portfolio.
func (r *PostgresRepository) GetPortfolioByUser(ctx context.Context, userID string) (*Portfolio, error) {
	return r.getPortfolioByUserTx(ctx, r.pool, userID)
}

func (r *PostgresRepository) getPortfolioByUserTx(ctx context.Context, q DBTX, userID string) (*Portfolio, error) {
	var p Portfolio
	err := q.QueryRow(ctx,
		`SELECT id, user_id, name, currency, COALESCE(version,1),
		        COALESCE(auto_fund_purchases, TRUE), created_at, updated_at
		 FROM portfolios WHERE user_id = $1 ORDER BY created_at LIMIT 1`, userID,
	).Scan(&p.ID, &p.UserID, &p.Name, &p.Currency, &p.Version, &p.AutoFundPurchases,
		&p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPortfolioNotFound
		}
		return nil, fmt.Errorf("portfolio: get portfolio: %w", err)
	}
	return &p, nil
}

// SetAutoFundPurchases toggles the automatic-purchase-funding preference.
func (r *PostgresRepository) SetAutoFundPurchases(ctx context.Context, userID string, enabled bool) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE portfolios SET auto_fund_purchases=$2, updated_at=NOW() WHERE user_id=$1`,
		userID, enabled)
	if err != nil {
		return fmt.Errorf("portfolio: set auto_fund_purchases: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrPortfolioNotFound
	}
	return nil
}

const positionColumns = `id, user_id, portfolio_id, symbol, COALESCE(instrument_id::text,''), asset_type, quantity, average_buy_price, currency,
	COALESCE(status, 'open'), closed_at, close_price, COALESCE(close_price_currency, ''),
	COALESCE(realized_gain_loss_base, 0), COALESCE(realized_gain_loss_percentage, 0),
	created_at, updated_at`

func scanPosition(row pgx.Row) (*Position, error) {
	var p Position
	err := row.Scan(&p.ID, &p.UserID, &p.PortfolioID, &p.Symbol, &p.InstrumentID, &p.AssetType,
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
		if err := rows.Scan(&p.ID, &p.UserID, &p.PortfolioID, &p.Symbol, &p.InstrumentID, &p.AssetType,
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

func (r *PostgresRepository) ListOpenPositionsByInstrumentID(ctx context.Context, instrumentID string) ([]*Position, error) {
	if instrumentID == "" {
		return []*Position{}, nil
	}
	return scanPositions(ctx, r.pool,
		`SELECT `+positionColumns+` FROM positions WHERE instrument_id = $1 AND COALESCE(status, 'open') = 'open' ORDER BY created_at`, instrumentID)
}

func (r *PostgresRepository) ListActiveInstruments(ctx context.Context) ([]ActiveInstrument, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT DISTINCT instrument_id::text, symbol FROM positions
		 WHERE COALESCE(status, 'open') = 'open' AND instrument_id IS NOT NULL
		 ORDER BY symbol`)
	if err != nil {
		return nil, fmt.Errorf("portfolio: list active instruments: %w", err)
	}
	defer rows.Close()
	out := make([]ActiveInstrument, 0)
	for rows.Next() {
		var item ActiveInstrument
		if err := rows.Scan(&item.InstrumentID, &item.Symbol); err != nil {
			return nil, fmt.Errorf("portfolio: scan active instrument: %w", err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// ListPositionsMissingInstrumentID returns positions with no resolved
// instrument identity, for BackfillJob. Only ID/Symbol/CreatedAt are
// populated — the backfill job needs nothing else.
func (r *PostgresRepository) ListPositionsMissingInstrumentID(ctx context.Context, limit int) ([]*Position, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := r.pool.Query(ctx,
		`SELECT id, symbol, created_at FROM positions WHERE instrument_id IS NULL ORDER BY created_at LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("portfolio: list positions missing instrument id: %w", err)
	}
	defer rows.Close()
	out := make([]*Position, 0)
	for rows.Next() {
		p := &Position{}
		if err := rows.Scan(&p.ID, &p.Symbol, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("portfolio: scan position missing instrument id: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ListActivitiesMissingInstrumentID is the activity-ledger counterpart of
// ListPositionsMissingInstrumentID.
func (r *PostgresRepository) ListActivitiesMissingInstrumentID(ctx context.Context, limit int) ([]Activity, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := r.pool.Query(ctx,
		`SELECT id, COALESCE(symbol,''), occurred_at FROM portfolio_activities
		 WHERE instrument_id IS NULL AND COALESCE(symbol,'') <> ''
		 ORDER BY occurred_at LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("portfolio: list activities missing instrument id: %w", err)
	}
	defer rows.Close()
	out := make([]Activity, 0)
	for rows.Next() {
		var a Activity
		if err := rows.Scan(&a.ID, &a.Symbol, &a.OccurredAt); err != nil {
			return nil, fmt.Errorf("portfolio: scan activity missing instrument id: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) SetPositionInstrumentID(ctx context.Context, id, instrumentID string) error {
	tag, err := r.pool.Exec(ctx, `UPDATE positions SET instrument_id = $2 WHERE id = $1 AND instrument_id IS NULL`, id, instrumentID)
	if err != nil {
		return fmt.Errorf("portfolio: set position instrument id: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrPositionNotFound
	}
	return nil
}

func (r *PostgresRepository) SetActivityInstrumentID(ctx context.Context, id, instrumentID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE portfolio_activities SET instrument_id = $2 WHERE id = $1 AND instrument_id IS NULL`, id, instrumentID)
	if err != nil {
		return fmt.Errorf("portfolio: set activity instrument id: %w", err)
	}
	return nil
}

func (r *PostgresRepository) EnqueueIdentityReconciliation(ctx context.Context, item ReconciliationItem) error {
	if item.ID == "" {
		item.ID = uuid.NewString()
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO identity_reconciliation_queue (
			id, table_name, record_id, symbol, candidate_instrument_id, evidence, confidence, status, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,'pending',now())
		ON CONFLICT (table_name, record_id) WHERE status = 'pending' DO NOTHING`,
		item.ID, item.TableName, item.RecordID, item.Symbol, nullableUUIDPortfolio(item.CandidateInstrumentID),
		item.Evidence, item.Confidence)
	if err != nil {
		return fmt.Errorf("portfolio: enqueue identity reconciliation: %w", err)
	}
	return nil
}

func (r *PostgresRepository) ListPendingReconciliation(ctx context.Context, limit int) ([]ReconciliationItem, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, table_name, record_id, symbol, COALESCE(candidate_instrument_id::text,''),
		       evidence, confidence, status, created_at
		FROM identity_reconciliation_queue
		WHERE status = 'pending'
		ORDER BY created_at LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("portfolio: list pending reconciliation: %w", err)
	}
	defer rows.Close()
	out := make([]ReconciliationItem, 0)
	for rows.Next() {
		var item ReconciliationItem
		if err := rows.Scan(&item.ID, &item.TableName, &item.RecordID, &item.Symbol,
			&item.CandidateInstrumentID, &item.Evidence, &item.Confidence, &item.Status, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("portfolio: scan reconciliation item: %w", err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) ResolveReconciliation(ctx context.Context, id, instrumentID, resolvedBy string) error {
	var tableName, recordID string
	err := r.pool.QueryRow(ctx,
		`SELECT table_name, record_id FROM identity_reconciliation_queue WHERE id = $1 AND status = 'pending'`, id).
		Scan(&tableName, &recordID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrReconciliationItemNotFound
		}
		return fmt.Errorf("portfolio: read reconciliation item: %w", err)
	}
	switch tableName {
	case "positions":
		if err := r.SetPositionInstrumentID(ctx, recordID, instrumentID); err != nil && !errors.Is(err, ErrPositionNotFound) {
			return err
		}
	case "portfolio_activities":
		if err := r.SetActivityInstrumentID(ctx, recordID, instrumentID); err != nil {
			return err
		}
	default:
		return fmt.Errorf("portfolio: unknown reconciliation table %q", tableName)
	}
	_, err = r.pool.Exec(ctx,
		`UPDATE identity_reconciliation_queue SET status='resolved', candidate_instrument_id=$2, resolved_at=now(), resolved_by=$3 WHERE id=$1`,
		id, instrumentID, nullIfEmpty(resolvedBy))
	if err != nil {
		return fmt.Errorf("portfolio: mark reconciliation resolved: %w", err)
	}
	return nil
}

func (r *PostgresRepository) RejectReconciliation(ctx context.Context, id, resolvedBy string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE identity_reconciliation_queue SET status='rejected', resolved_at=now(), resolved_by=$2 WHERE id=$1 AND status='pending'`,
		id, nullIfEmpty(resolvedBy))
	if err != nil {
		return fmt.Errorf("portfolio: reject reconciliation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrReconciliationItemNotFound
	}
	return nil
}

// nullableUUIDPortfolio mirrors instrument.nullableUUID: an empty string must
// become SQL NULL for a UUID column, not "".
func nullableUUIDPortfolio(s string) any {
	if s == "" {
		return nil
	}
	return s
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

func (r *PostgresRepository) ListIncomeDiscoveryInstruments(ctx context.Context, since time.Time) ([]IncomeDiscoveryInstrument, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT instrument_id, symbol, asset_type
		FROM (
			SELECT COALESCE(instrument_id::text,'') AS instrument_id, symbol, asset_type
			FROM positions
			WHERE COALESCE(status,'open')='open' OR closed_at >= $1
			UNION
			SELECT COALESCE(instrument_id::text,''), symbol, COALESCE(asset_type,'')
			FROM portfolio_activities
			WHERE occurred_at >= $1 AND COALESCE(symbol,'') <> ''
		) discovery
		ORDER BY symbol`, since)
	if err != nil {
		return nil, fmt.Errorf("portfolio: list income discovery instruments: %w", err)
	}
	defer rows.Close()
	out := make([]IncomeDiscoveryInstrument, 0)
	for rows.Next() {
		var item IncomeDiscoveryInstrument
		if err := rows.Scan(&item.InstrumentID, &item.Symbol, &item.AssetType); err != nil {
			return nil, fmt.Errorf("portfolio: scan income discovery instrument: %w", err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) ListIncomeHistoricalHolders(ctx context.Context, instrumentID, symbol string) ([]SymbolHolder, error) {
	if instrumentID == "" {
		// No resolved identity for this income event's instrument: holder
		// discovery falls back to case-insensitive symbol matching, which is
		// exactly the drift this whole identity layer exists to eliminate.
		telemetry.IncLegacySymbolFallback()
	}
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT ON (portfolio_id) user_id::text, portfolio_id::text, asset_type, created_at
		FROM positions
		WHERE (($1 <> '' AND instrument_id::text=$1) OR ($1 = '' AND upper(symbol)=upper($2)))
		ORDER BY portfolio_id, created_at ASC`, instrumentID, symbol)
	if err != nil {
		return nil, fmt.Errorf("portfolio: list income historical holders: %w", err)
	}
	defer rows.Close()
	out := make([]SymbolHolder, 0)
	for rows.Next() {
		var h SymbolHolder
		if err := rows.Scan(&h.UserID, &h.PortfolioID, &h.AssetType, &h.AcquiredAt); err != nil {
			return nil, fmt.Errorf("portfolio: scan income historical holder: %w", err)
		}
		out = append(out, h)
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
		       COALESCE(symbol,''), COALESCE(instrument_id::text,''), COALESCE(asset_type,''), currency, quantity,
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
			&activity.UserID, &activityType, &activity.Symbol, &activity.InstrumentID, &activity.AssetType,
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

// EligibleQuantity folds the complete, instrument-scoped ledger in PostgreSQL.
// It deliberately does not call ListActivities: that API is newest-first and
// capped for presentation use, neither of which is safe for entitlement.
func (r *PostgresRepository) EligibleQuantity(ctx context.Context, userID, instrumentID, symbol string, asOf time.Time) (money.Quantity, error) {
	var quantity money.Quantity
	err := r.pool.QueryRow(ctx, `
		WITH RECURSIVE relevant AS (
			SELECT a.id, a.activity_type, a.quantity, a.metadata_json,
			       row_number() OVER (
			           ORDER BY a.occurred_at, a.created_at, a.id
			       ) AS sequence
			FROM portfolio_activities a
			WHERE a.user_id = $1
			  AND a.occurred_at <= $4
			  AND (
			      ($2 <> '' AND a.instrument_id = NULLIF($2, '')::uuid)
			      OR
			      ($2 = '' AND upper(btrim(COALESCE(a.symbol, ''))) = upper(btrim($3)))
			  )
			  AND NOT EXISTS (
			      SELECT 1
			      FROM portfolio_activities correction
			      WHERE correction.user_id = $1
			        AND correction.metadata_json->>'correction_of_activity_id' = a.id::text
			  )
		),
		fold AS (
			SELECT 0::bigint AS sequence, 0::numeric AS quantity
			UNION ALL
			SELECT r.sequence,
			       CASE
			         WHEN r.activity_type = 'write_off' THEN 0::numeric
			         WHEN r.activity_type IN ('buy', 'opening_balance', 'reinvested_dividend')
			           THEN f.quantity + COALESCE(r.quantity, 0)
			         WHEN r.activity_type = 'sell'
			           THEN f.quantity - COALESCE(r.quantity, 0)
			         WHEN r.activity_type IN ('stock_split', 'reverse_split', 'stock_dividend')
			              AND r.metadata_json->>'ratio_numerator' ~ '^[0-9]+([.][0-9]+)?$'
			              AND r.metadata_json->>'ratio_denominator' ~ '^[0-9]+([.][0-9]+)?$'
			              AND (r.metadata_json->>'ratio_numerator')::numeric > 0
			              AND (r.metadata_json->>'ratio_denominator')::numeric > 0
			           THEN f.quantity * (
			             CASE WHEN r.activity_type = 'stock_dividend' THEN 1 ELSE 0 END
			             + (r.metadata_json->>'ratio_numerator')::numeric
			               / (r.metadata_json->>'ratio_denominator')::numeric
			           )
			         ELSE f.quantity
			       END
			FROM fold f
			JOIN relevant r ON r.sequence = f.sequence + 1
		)
		SELECT GREATEST(COALESCE((SELECT quantity FROM fold ORDER BY sequence DESC LIMIT 1), 0), 0)
	`, userID, instrumentID, symbol, asOf).Scan(&quantity)
	if err != nil {
		return money.ZeroQuantity(), fmt.Errorf("portfolio: calculate eligible quantity: %w", err)
	}
	return money.QuantizeQuantity(quantity), nil
}

func (r *PostgresRepository) GetActivityByID(ctx context.Context, userID, activityID string) (Activity, bool, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, COALESCE(request_id,''), portfolio_id, user_id, activity_type,
		       COALESCE(symbol,''), COALESCE(instrument_id::text,''), COALESCE(asset_type,''), currency, quantity,
		       unit_price, gross_amount, cost_basis_allocated,
		       realized_gain_loss_base, realized_gain_loss_percentage,
		       occurred_at, portfolio_version, metadata_json, created_at,
		       position_episode_id
		FROM portfolio_activities
		WHERE user_id=$1 AND id=$2`, userID, activityID)
	activity, ok, err := scanOptionalActivity(row)
	if err != nil {
		return Activity{}, false, fmt.Errorf("portfolio: get activity by id: %w", err)
	}
	return activity, ok, nil
}

func (r *PostgresRepository) FindCorrectionForActivity(ctx context.Context, userID, activityID string) (Activity, bool, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, COALESCE(request_id,''), portfolio_id, user_id, activity_type,
		       COALESCE(symbol,''), COALESCE(instrument_id::text,''), COALESCE(asset_type,''), currency, quantity,
		       unit_price, gross_amount, cost_basis_allocated,
		       realized_gain_loss_base, realized_gain_loss_percentage,
		       occurred_at, portfolio_version, metadata_json, created_at,
		       position_episode_id
		FROM portfolio_activities
		WHERE user_id=$1 AND metadata_json->>'correction_of_activity_id' = $2
		LIMIT 1`, userID, activityID)
	activity, ok, err := scanOptionalActivity(row)
	if err != nil {
		return Activity{}, false, fmt.Errorf("portfolio: find correction for activity: %w", err)
	}
	return activity, ok, nil
}

func scanOptionalActivity(row pgx.Row) (Activity, bool, error) {
	var activity Activity
	var activityType string
	var metadata []byte
	var episodeID *string
	err := row.Scan(&activity.ID, &activity.RequestID, &activity.PortfolioID,
		&activity.UserID, &activityType, &activity.Symbol, &activity.InstrumentID, &activity.AssetType,
		&activity.Currency, &activity.Quantity, &activity.UnitPrice,
		&activity.GrossAmount, &activity.CostBasisAllocated,
		&activity.RealizedGainLossBase, &activity.RealizedGainLossPercentage,
		&activity.OccurredAt, &activity.PortfolioVersion, &metadata,
		&activity.CreatedAt, &episodeID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Activity{}, false, nil
		}
		return Activity{}, false, err
	}
	activity.Type = ActivityType(activityType)
	_ = json.Unmarshal(metadata, &activity.Metadata)
	if episodeID != nil {
		activity.PositionEpisodeID = *episodeID
	}
	return activity, true, nil
}

func (r *PostgresRepository) ListActivitiesFiltered(ctx context.Context, userID string, types []ActivityType, symbol string, limit, offset int) ([]Activity, int, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	where := "WHERE user_id=$1"
	args := []any{userID}
	if symbol != "" {
		args = append(args, symbol)
		where += fmt.Sprintf(" AND UPPER(COALESCE(symbol,'')) = $%d", len(args))
	}
	if len(types) > 0 {
		typeStrs := make([]string, len(types))
		for i, t := range types {
			typeStrs[i] = string(t)
		}
		args = append(args, typeStrs)
		where += fmt.Sprintf(" AND activity_type = ANY($%d)", len(args))
	}

	var total int
	if err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM portfolio_activities "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("portfolio: count filtered activities: %w", err)
	}

	args = append(args, limit, offset)
	query := fmt.Sprintf(`
		SELECT id, COALESCE(request_id,''), portfolio_id, user_id, activity_type,
		       COALESCE(symbol,''), COALESCE(instrument_id::text,''), COALESCE(asset_type,''), currency, quantity,
		       unit_price, gross_amount, cost_basis_allocated,
		       realized_gain_loss_base, realized_gain_loss_percentage,
		       occurred_at, portfolio_version, metadata_json, created_at,
		       position_episode_id
		FROM portfolio_activities
		%s
		ORDER BY occurred_at DESC, created_at DESC
		LIMIT $%d OFFSET $%d`, where, len(args)-1, len(args))
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("portfolio: list filtered activities: %w", err)
	}
	defer rows.Close()
	out := make([]Activity, 0, limit)
	for rows.Next() {
		var activity Activity
		var activityType string
		var metadata []byte
		var episodeID *string
		if err := rows.Scan(&activity.ID, &activity.RequestID, &activity.PortfolioID,
			&activity.UserID, &activityType, &activity.Symbol, &activity.InstrumentID, &activity.AssetType,
			&activity.Currency, &activity.Quantity, &activity.UnitPrice,
			&activity.GrossAmount, &activity.CostBasisAllocated,
			&activity.RealizedGainLossBase, &activity.RealizedGainLossPercentage,
			&activity.OccurredAt, &activity.PortfolioVersion, &metadata,
			&activity.CreatedAt, &episodeID); err != nil {
			return nil, 0, fmt.Errorf("portfolio: scan filtered activity: %w", err)
		}
		activity.Type = ActivityType(activityType)
		_ = json.Unmarshal(metadata, &activity.Metadata)
		if episodeID != nil {
			activity.PositionEpisodeID = *episodeID
		}
		out = append(out, activity)
	}
	return out, total, rows.Err()
}

// ListActivitiesByPositionEpisode returns every activity sharing the given
// position episode id, scoped to userID for ownership. It uses the
// (portfolio_id, position_episode_id, occurred_at) index from migration 0018
// (portfolio_activities_episode_id_idx) rather than scanning the full ledger.
func (r *PostgresRepository) ListActivitiesByPositionEpisode(ctx context.Context, userID, episodeID string) ([]Activity, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, COALESCE(request_id,''), portfolio_id, user_id, activity_type,
		       COALESCE(symbol,''), COALESCE(instrument_id::text,''), COALESCE(asset_type,''), currency, quantity,
		       unit_price, gross_amount, cost_basis_allocated,
		       realized_gain_loss_base, realized_gain_loss_percentage,
		       occurred_at, portfolio_version, metadata_json, created_at,
		       position_episode_id
		FROM portfolio_activities
		WHERE user_id=$1 AND position_episode_id=$2
		ORDER BY occurred_at ASC, created_at ASC`, userID, episodeID)
	if err != nil {
		return nil, fmt.Errorf("portfolio: list activities by episode: %w", err)
	}
	defer rows.Close()
	out := make([]Activity, 0)
	for rows.Next() {
		var activity Activity
		var activityType string
		var metadata []byte
		var episode *string
		if err := rows.Scan(&activity.ID, &activity.RequestID, &activity.PortfolioID,
			&activity.UserID, &activityType, &activity.Symbol, &activity.InstrumentID, &activity.AssetType,
			&activity.Currency, &activity.Quantity, &activity.UnitPrice,
			&activity.GrossAmount, &activity.CostBasisAllocated,
			&activity.RealizedGainLossBase, &activity.RealizedGainLossPercentage,
			&activity.OccurredAt, &activity.PortfolioVersion, &metadata,
			&activity.CreatedAt, &episode); err != nil {
			return nil, fmt.Errorf("portfolio: scan episode activity: %w", err)
		}
		activity.Type = ActivityType(activityType)
		_ = json.Unmarshal(metadata, &activity.Metadata)
		if episode != nil {
			activity.PositionEpisodeID = *episode
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

// SnapshottedUserIDs queries the generated captured_date column directly
// (see migration 0010's daily-uniqueness index), so it's an index-only
// membership check rather than a valuation.
func (r *PostgresRepository) SnapshottedUserIDs(ctx context.Context, date time.Time) (map[string]bool, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT DISTINCT user_id FROM portfolio_archive_snapshots WHERE captured_date = $1`,
		date.UTC().Format("2006-01-02"))
	if err != nil {
		return nil, fmt.Errorf("portfolio: list snapshotted user ids: %w", err)
	}
	defer rows.Close()

	out := map[string]bool{}
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, fmt.Errorf("portfolio: scan snapshotted user id: %w", err)
		}
		out[userID] = true
	}
	return out, rows.Err()
}
