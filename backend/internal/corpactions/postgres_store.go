package corpactions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore is the durable corporate-action store. Uniqueness on
// (provider, provider_event_id) and the (corporate_action_id, portfolio_id)
// primary key make ingestion and application idempotent; ClaimApplication uses a
// transactional insert so two workers never apply the same event twice.
type PostgresStore struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

// NewPostgresStore wires the durable store.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool, now: func() time.Time { return time.Now().UTC() }}
}

func (s *PostgresStore) UpsertEvent(ctx context.Context, ev CorporateAction) (bool, error) {
	payload, err := json.Marshal(ev)
	if err != nil {
		return false, fmt.Errorf("corpactions: marshal event: %w", err)
	}
	var existingFingerprint string
	var existingStatus Status
	err = s.pool.QueryRow(ctx,
		`SELECT raw_fingerprint, status FROM corporate_actions WHERE id=$1`, ev.ID).
		Scan(&existingFingerprint, &existingStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		_, err = s.pool.Exec(ctx, `
			INSERT INTO corporate_actions (
				id, provider, provider_event_id, event_type, source_symbol, target_symbol,
				effective_at, status, quality, normalized_payload, source_url, raw_fingerprint,
				retrieved_at, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,now(),now())
			ON CONFLICT (id) DO NOTHING`,
			ev.ID, ev.Provider, ev.ProviderEventID, string(ev.Type), ev.Source.Symbol,
			targetSymbol(ev), ev.EffectiveAt, string(ev.Status), string(ev.Quality),
			payload, ev.SourceURL, ev.RawFingerprint, ev.RetrievedAt)
		if err != nil {
			return false, fmt.Errorf("corpactions: insert event: %w", err)
		}
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("corpactions: read event: %w", err)
	}
	if existingFingerprint == ev.RawFingerprint {
		return false, nil
	}
	status := ev.Status
	if existingStatus == StatusApplied {
		status = StatusSuperseded // material change after application: correction workflow
	}
	_, err = s.pool.Exec(ctx, `
		UPDATE corporate_actions
		SET event_type=$2, source_symbol=$3, target_symbol=$4, effective_at=$5, status=$6,
		    quality=$7, normalized_payload=$8, source_url=$9, raw_fingerprint=$10,
		    retrieved_at=$11, updated_at=now()
		WHERE id=$1`,
		ev.ID, string(ev.Type), ev.Source.Symbol, targetSymbol(ev), ev.EffectiveAt,
		string(status), string(ev.Quality), payload, ev.SourceURL, ev.RawFingerprint, ev.RetrievedAt)
	if err != nil {
		return false, fmt.Errorf("corpactions: update event: %w", err)
	}
	return true, nil
}

func targetSymbol(ev CorporateAction) any {
	if ev.Target != nil && ev.Target.Symbol != "" {
		return ev.Target.Symbol
	}
	return nil
}

func (s *PostgresStore) ListEventsByStatus(ctx context.Context, statuses ...Status) ([]CorporateAction, error) {
	strs := make([]string, len(statuses))
	for i, st := range statuses {
		strs[i] = string(st)
	}
	rows, err := s.pool.Query(ctx,
		`SELECT normalized_payload FROM corporate_actions WHERE status = ANY($1) ORDER BY effective_at`, strs)
	if err != nil {
		return nil, fmt.Errorf("corpactions: list events: %w", err)
	}
	defer rows.Close()
	out := make([]CorporateAction, 0)
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var ev CorporateAction
		if err := json.Unmarshal(payload, &ev); err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

func (s *PostgresStore) GetEvent(ctx context.Context, id string) (CorporateAction, bool, error) {
	var payload []byte
	err := s.pool.QueryRow(ctx, `SELECT normalized_payload FROM corporate_actions WHERE id=$1`, id).Scan(&payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return CorporateAction{}, false, nil
	}
	if err != nil {
		return CorporateAction{}, false, err
	}
	var ev CorporateAction
	if err := json.Unmarshal(payload, &ev); err != nil {
		return CorporateAction{}, false, err
	}
	return ev, true, nil
}

func (s *PostgresStore) SetEventStatus(ctx context.Context, id string, status Status) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE corporate_actions SET status=$2, updated_at=now() WHERE id=$1`, id, string(status))
	return err
}

// ClaimApplication inserts the (event, portfolio) application in the applying
// state. The primary key makes a concurrent second claim fail, so exactly one
// worker proceeds. An existing terminal/in-flight row means "already claimed".
func (s *PostgresStore) ClaimApplication(ctx context.Context, eventID, portfolioID, userID string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO corporate_action_applications
			(corporate_action_id, portfolio_id, user_id, status, created_at, updated_at)
		VALUES ($1,$2,$3,$4,now(),now())
		ON CONFLICT (corporate_action_id, portfolio_id) DO NOTHING`,
		eventID, portfolioID, userID, string(ApplicationApplying))
	if err != nil {
		return false, fmt.Errorf("corpactions: claim application: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func (s *PostgresStore) CompleteApplication(ctx context.Context, app Application) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE corporate_action_applications
		SET status=$3, applied_at=now(), updated_at=now()
		WHERE corporate_action_id=$1 AND portfolio_id=$2`,
		app.CorporateActionID, app.PortfolioID, string(ApplicationApplied))
	return err
}

func (s *PostgresStore) FailApplication(ctx context.Context, eventID, portfolioID, errorCode string, nextRetryAt time.Time) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE corporate_action_applications
		SET status=$3, error_code=$4, retry_count=retry_count+1, next_retry_at=$5, updated_at=now()
		WHERE corporate_action_id=$1 AND portfolio_id=$2`,
		eventID, portfolioID, string(ApplicationFailed), errorCode, nextRetryAt)
	return err
}

func (s *PostgresStore) SkipApplication(ctx context.Context, eventID, portfolioID, reason string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO corporate_action_applications
			(corporate_action_id, portfolio_id, user_id, status, error_code, created_at, updated_at)
		VALUES ($1,$2,'00000000-0000-0000-0000-000000000000',$3,$4,now(),now())
		ON CONFLICT (corporate_action_id, portfolio_id)
		DO UPDATE SET status=$3, error_code=$4, updated_at=now()`,
		eventID, portfolioID, string(ApplicationSkipped), reason)
	return err
}

func (s *PostgresStore) ListApplicationsForUser(ctx context.Context, userID string) ([]Application, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT corporate_action_id, portfolio_id, user_id, status, applied_at, error_code, retry_count, created_at, updated_at
		FROM corporate_action_applications WHERE user_id=$1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Application, 0)
	for rows.Next() {
		var a Application
		var status string
		if err := rows.Scan(&a.CorporateActionID, &a.PortfolioID, &a.UserID, &status,
			&a.AppliedAt, &a.ErrorCode, &a.RetryCount, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		a.Status = ApplicationStatus(status)
		out = append(out, a)
	}
	return out, rows.Err()
}
