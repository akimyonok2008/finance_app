package income

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore is the durable income-event store. Uniqueness on
// (provider, provider_event_id) and the (income_event_id, portfolio_id) primary
// key make ingestion and application idempotent; ClaimApplication uses a
// transactional insert so two workers never credit the same event twice.
type PostgresStore struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool, now: func() time.Time { return time.Now().UTC() }}
}

func (s *PostgresStore) UpsertEvent(ctx context.Context, ev IncomeEvent) (bool, error) {
	payload, err := json.Marshal(ev)
	if err != nil {
		return false, fmt.Errorf("income: marshal event: %w", err)
	}
	var existingFingerprint string
	var existingStatus Status
	err = s.pool.QueryRow(ctx,
		`SELECT raw_fingerprint, status FROM income_events WHERE id=$1`, ev.ID).
		Scan(&existingFingerprint, &existingStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		_, err = s.pool.Exec(ctx, `
			INSERT INTO income_events (
				id, provider, provider_event_id, event_type, instrument_symbol, instrument_id,
				amount_per_unit, currency, declaration_at, ex_date, record_date, payment_date,
				status, quality, tax_classification, normalized_payload, source_url,
				raw_fingerprint, retrieved_at, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,now(),now())
			ON CONFLICT (id) DO NOTHING`,
			ev.ID, ev.Provider, ev.ProviderEventID, string(ev.Type), ev.Instrument.Symbol,
			nullString(ev.Instrument.InstrumentID), ev.AmountPerUnit, ev.Currency, ev.DeclarationAt, ev.ExDate, ev.RecordDate, ev.PaymentDate,
			string(ev.Status), string(ev.Quality), nullString(ev.TaxClassification), payload,
			nullString(ev.SourceURL), ev.RawFingerprint, ev.RetrievedAt)
		if err != nil {
			return false, fmt.Errorf("income: insert event: %w", err)
		}
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("income: read event: %w", err)
	}
	if existingFingerprint == ev.RawFingerprint {
		return false, nil
	}
	status := ev.Status
	if existingStatus == StatusApplied {
		status = StatusSuperseded // material change after application: correction workflow
	}
	_, err = s.pool.Exec(ctx, `
		UPDATE income_events
		SET event_type=$2, instrument_symbol=$3, instrument_id=$4, amount_per_unit=$5, currency=$6,
		    declaration_at=$7, ex_date=$8, record_date=$9, payment_date=$10, status=$11,
		    quality=$12, tax_classification=$13, normalized_payload=$14, source_url=$15,
		    raw_fingerprint=$16, retrieved_at=$17, updated_at=now()
		WHERE id=$1`,
		ev.ID, string(ev.Type), ev.Instrument.Symbol, nullString(ev.Instrument.InstrumentID), ev.AmountPerUnit, ev.Currency,
		ev.DeclarationAt, ev.ExDate, ev.RecordDate, ev.PaymentDate, string(status),
		string(ev.Quality), nullString(ev.TaxClassification), payload, nullString(ev.SourceURL),
		ev.RawFingerprint, ev.RetrievedAt)
	if err != nil {
		return false, fmt.Errorf("income: update event: %w", err)
	}
	return true, nil
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (s *PostgresStore) ListEventsByStatus(ctx context.Context, statuses ...Status) ([]IncomeEvent, error) {
	strs := make([]string, len(statuses))
	for i, st := range statuses {
		strs[i] = string(st)
	}
	rows, err := s.pool.Query(ctx,
		`SELECT normalized_payload FROM income_events WHERE status = ANY($1) ORDER BY payment_date`, strs)
	if err != nil {
		return nil, fmt.Errorf("income: list events: %w", err)
	}
	defer rows.Close()
	out := make([]IncomeEvent, 0)
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var ev IncomeEvent
		if err := json.Unmarshal(payload, &ev); err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

func (s *PostgresStore) GetEvent(ctx context.Context, id string) (IncomeEvent, bool, error) {
	var payload []byte
	err := s.pool.QueryRow(ctx, `SELECT normalized_payload FROM income_events WHERE id=$1`, id).Scan(&payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return IncomeEvent{}, false, nil
	}
	if err != nil {
		return IncomeEvent{}, false, err
	}
	var ev IncomeEvent
	if err := json.Unmarshal(payload, &ev); err != nil {
		return IncomeEvent{}, false, err
	}
	return ev, true, nil
}

func (s *PostgresStore) SetEventStatus(ctx context.Context, id string, status Status) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE income_events SET status=$2, updated_at=now() WHERE id=$1`, id, string(status))
	return err
}

// ClaimApplication inserts the (event, portfolio) application in the applying
// state. The primary key makes a concurrent second claim fail, so exactly one
// worker proceeds. An existing terminal/in-flight row means "already claimed".
func (s *PostgresStore) ClaimApplication(ctx context.Context, eventID, portfolioID, userID string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO income_event_applications
			(income_event_id, portfolio_id, user_id, status, created_at, updated_at)
		VALUES ($1,$2,$3,$4,now(),now())
		ON CONFLICT (income_event_id, portfolio_id) DO NOTHING`,
		eventID, portfolioID, userID, string(ApplicationApplying))
	if err != nil {
		return false, fmt.Errorf("income: claim application: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func (s *PostgresStore) CompleteApplication(ctx context.Context, app Application) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE income_event_applications
		SET status=$3, eligible_quantity=$4, gross_amount=$5, withholding_amount=$6,
		    fee_amount=$7, net_amount=$8, cash_currency=$9, reinvestment_quantity=$10,
		    estimated=$11, applied_at=now(), updated_at=now()
		WHERE income_event_id=$1 AND portfolio_id=$2`,
		app.IncomeEventID, app.PortfolioID, string(app.Status), app.EligibleQuantity,
		app.GrossAmount, app.WithholdingAmount, app.FeeAmount, app.NetAmount,
		nullString(app.CashCurrency), app.ReinvestmentQuantity, app.Estimated)
	return err
}

func (s *PostgresStore) FailApplication(ctx context.Context, eventID, portfolioID, errorCode string, nextRetryAt time.Time) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE income_event_applications
		SET status=$3, error_code=$4, retry_count=retry_count+1, next_retry_at=$5, updated_at=now()
		WHERE income_event_id=$1 AND portfolio_id=$2`,
		eventID, portfolioID, string(ApplicationFailed), errorCode, nextRetryAt)
	return err
}

func (s *PostgresStore) SkipApplication(ctx context.Context, eventID, portfolioID, reason string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO income_event_applications
			(income_event_id, portfolio_id, user_id, status, error_code, created_at, updated_at)
		VALUES ($1,$2,'00000000-0000-0000-0000-000000000000',$3,$4,now(),now())
		ON CONFLICT (income_event_id, portfolio_id)
		DO UPDATE SET status=$3, error_code=$4, updated_at=now()`,
		eventID, portfolioID, string(ApplicationSkipped), reason)
	return err
}

func (s *PostgresStore) GetApplication(ctx context.Context, eventID, portfolioID string) (Application, bool, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT income_event_id, portfolio_id, user_id, status, eligible_quantity, gross_amount,
		       withholding_amount, fee_amount, net_amount, COALESCE(cash_currency,''),
		       reinvestment_quantity, estimated, applied_at, COALESCE(error_code,''),
		       retry_count, created_at, updated_at
		FROM income_event_applications WHERE income_event_id=$1 AND portfolio_id=$2`, eventID, portfolioID)
	app, err := scanApplication(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Application{}, false, nil
	}
	if err != nil {
		return Application{}, false, err
	}
	return app, true, nil
}

func (s *PostgresStore) ListApplicationsForUser(ctx context.Context, userID string) ([]Application, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT income_event_id, portfolio_id, user_id, status, eligible_quantity, gross_amount,
		       withholding_amount, fee_amount, net_amount, COALESCE(cash_currency,''),
		       reinvestment_quantity, estimated, applied_at, COALESCE(error_code,''),
		       retry_count, created_at, updated_at
		FROM income_event_applications WHERE user_id=$1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Application, 0)
	for rows.Next() {
		app, err := scanApplication(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, app)
	}
	return out, rows.Err()
}

type scannable interface {
	Scan(dest ...any) error
}

func scanApplication(row scannable) (Application, error) {
	var a Application
	var status string
	if err := row.Scan(&a.IncomeEventID, &a.PortfolioID, &a.UserID, &status, &a.EligibleQuantity,
		&a.GrossAmount, &a.WithholdingAmount, &a.FeeAmount, &a.NetAmount, &a.CashCurrency,
		&a.ReinvestmentQuantity, &a.Estimated, &a.AppliedAt, &a.ErrorCode, &a.RetryCount,
		&a.CreatedAt, &a.UpdatedAt); err != nil {
		return Application{}, err
	}
	a.Status = ApplicationStatus(status)
	return a, nil
}
