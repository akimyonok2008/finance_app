package instrument

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresRepository is the durable implementation of Repository, backed by
// instrument_master / instrument_aliases (migration 0020). Every statement is
// parameterized; no identifier is interpolated into SQL.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository wires a Postgres-backed identity register.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

var _ Repository = (*PostgresRepository)(nil)

const instrumentColumns = `id, COALESCE(figi,''), COALESCE(composite_figi,''), COALESCE(share_class_figi,''),
	COALESCE(isin,''), COALESCE(cusip,''), COALESCE(cik,''), COALESCE(current_symbol,''),
	COALESCE(name,''), COALESCE(security_type,''), COALESCE(asset_type,''),
	COALESCE(exchange_code,''), COALESCE(mic,''), COALESCE(currency,''), COALESCE(country,''),
	status, listed_at, delisted_at, identity_quality, COALESCE(identity_provider,''),
	created_at, updated_at`

func scanInstrument(row pgx.Row) (Instrument, error) {
	var in Instrument
	err := row.Scan(&in.ID, &in.FIGI, &in.CompositeFIGI, &in.ShareClassFIGI,
		&in.ISIN, &in.CUSIP, &in.CIK, &in.CurrentSymbol, &in.Name, &in.SecurityType,
		&in.AssetType, &in.ExchangeCode, &in.MIC, &in.Currency, &in.Country,
		&in.Status, &in.ListedAt, &in.DelistedAt, &in.IdentityQuality,
		&in.IdentityProvider, &in.CreatedAt, &in.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Instrument{}, ErrInstrumentNotFound
		}
		return Instrument{}, fmt.Errorf("instrument: scan instrument: %w", err)
	}
	return in, nil
}

// nullable turns "" into a SQL NULL so the partial unique index on figi treats
// unresolved instruments as unconstrained rather than colliding on the empty string.
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func (r *PostgresRepository) CreateInstrument(ctx context.Context, in Instrument) (Instrument, error) {
	if in.ID == "" {
		in.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	if in.CreatedAt.IsZero() {
		in.CreatedAt = now
	}
	in.UpdatedAt = now
	if in.Status == "" {
		in.Status = StatusActive
	}
	if in.IdentityQuality == "" {
		in.IdentityQuality = QualityUnresolved
	}
	in.FIGI = normalizeAliasValue(in.FIGI)
	in.CompositeFIGI = normalizeAliasValue(in.CompositeFIGI)
	in.ShareClassFIGI = normalizeAliasValue(in.ShareClassFIGI)
	in.CurrentSymbol = normalizeAliasValue(in.CurrentSymbol)

	row := r.pool.QueryRow(ctx,
		`INSERT INTO instrument_master (
			id, figi, composite_figi, share_class_figi, isin, cusip, cik,
			current_symbol, name, security_type, asset_type, exchange_code, mic,
			currency, country, status, listed_at, delisted_at, identity_quality,
			identity_provider, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)
		 RETURNING `+instrumentColumns,
		in.ID, nullable(in.FIGI), nullable(in.CompositeFIGI), nullable(in.ShareClassFIGI),
		nullable(in.ISIN), nullable(in.CUSIP), nullable(in.CIK), nullable(in.CurrentSymbol),
		nullable(in.Name), nullable(in.SecurityType), nullable(in.AssetType),
		nullable(in.ExchangeCode), nullable(in.MIC), nullable(in.Currency),
		nullable(in.Country), in.Status, in.ListedAt, in.DelistedAt,
		string(in.IdentityQuality), nullable(in.IdentityProvider), in.CreatedAt, in.UpdatedAt,
	)
	out, err := scanInstrument(row)
	if err != nil {
		if isUniqueViolation(err) {
			return Instrument{}, ErrAliasConflict
		}
		return Instrument{}, err
	}
	return out, nil
}

func (r *PostgresRepository) GetInstrumentByID(ctx context.Context, id string) (Instrument, error) {
	if _, err := uuid.Parse(id); err != nil {
		return Instrument{}, ErrInstrumentNotFound
	}
	return scanInstrument(r.pool.QueryRow(ctx,
		`SELECT `+instrumentColumns+` FROM instrument_master WHERE id = $1`, id))
}

func (r *PostgresRepository) UpdateInstrumentSymbol(ctx context.Context, id, currentSymbol string, updatedAt time.Time) error {
	if _, err := uuid.Parse(id); err != nil {
		return ErrInstrumentNotFound
	}
	// Only current_symbol moves. figi/isin/cusip are untouched by design: a
	// rename does not change what the instrument is.
	tag, err := r.pool.Exec(ctx,
		`UPDATE instrument_master SET current_symbol = $2, updated_at = $3 WHERE id = $1`,
		id, normalizeAliasValue(currentSymbol), updatedAt.UTC())
	if err != nil {
		return fmt.Errorf("instrument: update current symbol: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrInstrumentNotFound
	}
	return nil
}

const aliasColumns = `id, instrument_id, alias_type, alias_value, exchange_code, mic,
	valid_from, valid_to, provider, provider_event_id, created_at`

func scanAlias(row pgx.Row) (InstrumentAlias, error) {
	var a InstrumentAlias
	err := row.Scan(&a.ID, &a.InstrumentID, &a.AliasType, &a.AliasValue,
		&a.ExchangeCode, &a.MIC, &a.ValidFrom, &a.ValidTo, &a.Provider,
		&a.ProviderEventID, &a.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return InstrumentAlias{}, ErrAliasNotFound
		}
		return InstrumentAlias{}, fmt.Errorf("instrument: scan alias: %w", err)
	}
	return a, nil
}

func (r *PostgresRepository) CreateAlias(ctx context.Context, alias InstrumentAlias) (InstrumentAlias, error) {
	if !alias.AliasType.Valid() || normalizeAliasValue(alias.AliasValue) == "" {
		return InstrumentAlias{}, ErrInvalidAlias
	}
	if alias.ID == "" {
		alias.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	if alias.ValidFrom.IsZero() {
		alias.ValidFrom = now
	}

	row := r.pool.QueryRow(ctx,
		`INSERT INTO instrument_aliases (
			id, instrument_id, alias_type, alias_value, exchange_code, mic,
			valid_from, valid_to, provider, provider_event_id, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		 RETURNING `+aliasColumns,
		alias.ID, alias.InstrumentID, string(alias.AliasType),
		normalizeAliasValue(alias.AliasValue), normalizeScope(alias.ExchangeCode),
		normalizeScope(alias.MIC), alias.ValidFrom.UTC(), alias.ValidTo,
		alias.Provider, alias.ProviderEventID, now,
	)
	out, err := scanAlias(row)
	if err != nil {
		if isUniqueViolation(err) {
			return InstrumentAlias{}, ErrAliasConflict
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case "23503": // FK violation: unknown instrument
				return InstrumentAlias{}, ErrInstrumentNotFound
			case "23514": // CHECK violation: bad alias_type
				return InstrumentAlias{}, ErrInvalidAlias
			}
		}
		return InstrumentAlias{}, err
	}
	return out, nil
}

func (r *PostgresRepository) CloseAlias(ctx context.Context, aliasID string, validTo time.Time) error {
	if _, err := uuid.Parse(aliasID); err != nil {
		return ErrAliasNotFound
	}
	tag, err := r.pool.Exec(ctx,
		`UPDATE instrument_aliases SET valid_to = $2 WHERE id = $1 AND valid_to IS NULL`,
		aliasID, validTo.UTC())
	if err != nil {
		return fmt.Errorf("instrument: close alias: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Either the row does not exist or it is already closed. Distinguish so
		// callers can tell a typo from a double-close.
		var exists bool
		if err := r.pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM instrument_aliases WHERE id = $1)`, aliasID,
		).Scan(&exists); err != nil {
			return fmt.Errorf("instrument: close alias: %w", err)
		}
		if exists {
			return ErrAliasNotActive
		}
		return ErrAliasNotFound
	}
	return nil
}

func (r *PostgresRepository) ListAliasesForInstrument(ctx context.Context, instrumentID string) ([]InstrumentAlias, error) {
	if _, err := uuid.Parse(instrumentID); err != nil {
		return []InstrumentAlias{}, nil
	}
	rows, err := r.pool.Query(ctx,
		`SELECT `+aliasColumns+` FROM instrument_aliases
		 WHERE instrument_id = $1 ORDER BY valid_from, id`, instrumentID)
	if err != nil {
		return nil, fmt.Errorf("instrument: list aliases: %w", err)
	}
	defer rows.Close()

	out := make([]InstrumentAlias, 0)
	for rows.Next() {
		a, err := scanAlias(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("instrument: list aliases: %w", err)
	}
	return out, nil
}

func (r *PostgresRepository) FindActiveAlias(ctx context.Context, instrumentID string, aliasType AliasType) (*InstrumentAlias, error) {
	if _, err := uuid.Parse(instrumentID); err != nil {
		return nil, ErrAliasNotFound
	}
	a, err := scanAlias(r.pool.QueryRow(ctx,
		`SELECT `+aliasColumns+` FROM instrument_aliases
		 WHERE instrument_id = $1 AND alias_type = $2 AND valid_to IS NULL
		 ORDER BY valid_from DESC LIMIT 1`, instrumentID, string(aliasType)))
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *PostgresRepository) FindInstrumentByAlias(ctx context.Context, aliasType AliasType, aliasValue, exchangeCode, mic string) (*Instrument, error) {
	return r.findByAlias(ctx, aliasType, aliasValue, exchangeCode, mic, nil)
}

func (r *PostgresRepository) FindInstrumentByAliasAsOf(ctx context.Context, aliasType AliasType, aliasValue, exchangeCode, mic string, asOf time.Time) (*Instrument, error) {
	t := asOf.UTC()
	return r.findByAlias(ctx, aliasType, aliasValue, exchangeCode, mic, &t)
}

func (r *PostgresRepository) findByAlias(ctx context.Context, aliasType AliasType, aliasValue, exchangeCode, mic string, asOf *time.Time) (*Instrument, error) {
	value := normalizeAliasValue(aliasValue)
	if value == "" || !aliasType.Valid() {
		return nil, ErrInvalidAlias
	}
	ex := normalizeScope(exchangeCode)
	m := normalizeScope(mic)

	// A single parameterized statement covers both the active-only and the
	// as-of variant: $5 NULL means "active only". Empty exchange/mic mean
	// "any", so a ticker-only query is a superset of ticker+exchange.
	rows, err := r.pool.Query(ctx,
		`SELECT DISTINCT instrument_id FROM instrument_aliases
		 WHERE alias_type = $1
		   AND alias_value = $2
		   AND ($3 = '' OR exchange_code = $3)
		   AND ($4 = '' OR mic = $4)
		   AND (
		        ($5::timestamptz IS NULL AND valid_to IS NULL)
		     OR ($5::timestamptz IS NOT NULL AND valid_from <= $5::timestamptz
		         AND (valid_to IS NULL OR valid_to > $5::timestamptz))
		   )
		 LIMIT 2`,
		string(aliasType), value, ex, m, asOf)
	if err != nil {
		return nil, fmt.Errorf("instrument: find by alias: %w", err)
	}
	defer rows.Close()

	ids := make([]string, 0, 2)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("instrument: find by alias: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("instrument: find by alias: %w", err)
	}
	rows.Close()

	switch len(ids) {
	case 0:
		return nil, nil
	case 1:
		in, err := r.GetInstrumentByID(ctx, ids[0])
		if err != nil {
			return nil, err
		}
		return &in, nil
	}
	return nil, ErrAliasConflict
}
