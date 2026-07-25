package portfolio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ardakimyonok/finance_app/internal/performance"
)

// DBTX is the query surface shared by *pgxpool.Pool and pgx.Tx, so the same SQL
// runs either through the pool (reads) or inside the aggregate transaction
// (mutations) without duplication. Every method takes a context.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// postgresTx is the transaction-scoped view of one locked portfolio aggregate.
// Every write goes through tx, so nothing can escape the transaction.
type postgresTx struct {
	tx        pgx.Tx
	portfolio *Portfolio
	positions []*Position
	cash      []CashBalance
}

func (t *postgresTx) Portfolio() *Portfolio  { return t.portfolio }
func (t *postgresTx) Positions() []*Position { return t.positions }
func (t *postgresTx) CashBalances() []CashBalance {
	return append([]CashBalance(nil), t.cash...)
}

func (t *postgresTx) PutCashBalance(ctx context.Context, balance CashBalance) error {
	_, err := t.tx.Exec(ctx, `
		INSERT INTO portfolio_cash_balances (
			portfolio_id, currency, amount, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (portfolio_id,currency) DO UPDATE
		SET amount=EXCLUDED.amount, updated_at=EXCLUDED.updated_at`,
		balance.PortfolioID, balance.Currency, balance.Amount,
		balance.CreatedAt, balance.UpdatedAt)
	if err != nil {
		return fmt.Errorf("portfolio: put cash balance: %w", err)
	}
	return nil
}

func (t *postgresTx) RecordActivity(ctx context.Context, activity Activity) error {
	metadata, err := json.Marshal(activity.Metadata)
	if err != nil {
		return fmt.Errorf("portfolio: marshal activity metadata: %w", err)
	}
	_, err = t.tx.Exec(ctx, `
		INSERT INTO portfolio_activities (
			id, request_id, portfolio_id, user_id, activity_type, symbol,
			asset_type, currency, quantity, unit_price, gross_amount,
			cost_basis_allocated, realized_gain_loss_base,
			realized_gain_loss_percentage, occurred_at, portfolio_version,
			metadata_json, created_at, position_episode_id
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`,
		activity.ID, nullIfEmpty(activity.RequestID), activity.PortfolioID,
		activity.UserID, string(activity.Type), nullIfEmpty(activity.Symbol),
		nullIfEmpty(activity.AssetType), activity.Currency, activity.Quantity,
		activity.UnitPrice, activity.GrossAmount, activity.CostBasisAllocated,
		activity.RealizedGainLossBase, activity.RealizedGainLossPercentage,
		activity.OccurredAt, activity.PortfolioVersion, metadata, activity.CreatedAt,
		nullIfEmpty(activity.PositionEpisodeID))
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23505" {
			return ErrDuplicateActivity
		}
		return fmt.Errorf("portfolio: record activity: %w", err)
	}
	return nil
}

func (t *postgresTx) FindActivityByRequestID(ctx context.Context, requestID string) (Activity, bool, error) {
	var activity Activity
	var activityType string
	var metadata []byte
	var episodeID *string
	err := t.tx.QueryRow(ctx, `
		SELECT id, COALESCE(request_id,''), portfolio_id, user_id, activity_type,
		       COALESCE(symbol,''), COALESCE(asset_type,''), currency, quantity,
		       unit_price, gross_amount, cost_basis_allocated,
		       realized_gain_loss_base, realized_gain_loss_percentage,
		       occurred_at, portfolio_version, metadata_json, created_at,
		       position_episode_id
		FROM portfolio_activities
		WHERE portfolio_id=$1 AND request_id=$2`,
		t.portfolio.ID, requestID).Scan(
		&activity.ID, &activity.RequestID, &activity.PortfolioID, &activity.UserID,
		&activityType, &activity.Symbol, &activity.AssetType, &activity.Currency,
		&activity.Quantity, &activity.UnitPrice, &activity.GrossAmount,
		&activity.CostBasisAllocated, &activity.RealizedGainLossBase,
		&activity.RealizedGainLossPercentage, &activity.OccurredAt,
		&activity.PortfolioVersion, &metadata, &activity.CreatedAt, &episodeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Activity{}, false, nil
	}
	if err != nil {
		return Activity{}, false, fmt.Errorf("portfolio: find activity: %w", err)
	}
	activity.Type = ActivityType(activityType)
	_ = json.Unmarshal(metadata, &activity.Metadata)
	if episodeID != nil {
		activity.PositionEpisodeID = *episodeID
	}
	return activity, true, nil
}

func (t *postgresTx) CreatePosition(ctx context.Context, p *Position) error {
	_, err := t.tx.Exec(ctx,
		`INSERT INTO positions (
			id, user_id, portfolio_id, symbol, asset_type, quantity, average_buy_price,
			currency, status, closed_at, close_price, close_price_currency,
			realized_gain_loss_base, realized_gain_loss_percentage, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
		p.ID, p.UserID, p.PortfolioID, p.Symbol, p.AssetType, p.Quantity,
		p.AverageBuyPrice, p.Currency, firstNonEmptyStatus(p.Status), p.ClosedAt,
		p.ClosePrice, p.CloseCurrency, p.RealizedGainLossBase,
		p.RealizedGainLossPercentage, p.CreatedAt, p.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("portfolio: create position: %w", err)
	}
	return nil
}

func (t *postgresTx) UpdatePosition(ctx context.Context, p *Position) error {
	tag, err := t.tx.Exec(ctx,
		`UPDATE positions
		 SET symbol=$2, asset_type=$3, quantity=$4, average_buy_price=$5, currency=$6,
		     status=$7, closed_at=$8, close_price=$9, close_price_currency=$10,
		     realized_gain_loss_base=$11, realized_gain_loss_percentage=$12, updated_at=$13
		 WHERE id=$1`,
		p.ID, p.Symbol, p.AssetType, p.Quantity, p.AverageBuyPrice, p.Currency,
		firstNonEmptyStatus(p.Status), p.ClosedAt, p.ClosePrice, p.CloseCurrency,
		p.RealizedGainLossBase, p.RealizedGainLossPercentage, p.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("portfolio: update position: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrPositionNotFound
	}
	return nil
}

func (t *postgresTx) DeletePosition(ctx context.Context, id string) error {
	tag, err := t.tx.Exec(ctx, `DELETE FROM positions WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("portfolio: delete position: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrPositionNotFound
	}
	return nil
}

// ReplaceOpenPositions swaps only the OPEN positions, preserving closed history.
func (t *postgresTx) ReplaceOpenPositions(ctx context.Context, newPositions []*Position) error {
	if _, err := t.tx.Exec(ctx,
		`DELETE FROM positions WHERE portfolio_id=$1 AND COALESCE(status,'open')='open'`,
		t.portfolio.ID); err != nil {
		return fmt.Errorf("portfolio: replace delete: %w", err)
	}
	for _, p := range newPositions {
		if err := t.CreatePosition(ctx, p); err != nil {
			return err
		}
	}
	return nil
}

func (t *postgresTx) RankedState(ctx context.Context) (performance.State, bool, error) {
	st, err := performance.ScanState(ctx, t.tx, t.portfolio.ID)
	if errors.Is(err, performance.ErrStateNotFound) {
		return performance.State{}, false, nil
	}
	if err != nil {
		return performance.State{}, false, err
	}
	return *st, true, nil
}

func (t *postgresTx) PutRankedState(ctx context.Context, state performance.State, isNew bool, expectedVersion int64) error {
	if err := performance.ValidateState(state); err != nil {
		return err
	}
	if isNew {
		// No ON CONFLICT DO NOTHING here: under the aggregate row lock only one
		// writer can reach this point, and a genuine conflict must surface as an
		// error that rolls the transaction back rather than being mistaken for a
		// successful create.
		_, err := t.tx.Exec(ctx, `
			INSERT INTO ranked_performance_state (
				portfolio_id, user_id, checkpoint_index, segment_start_value_base,
				status, tracking_started_at, segment_started_at, updated_at, version
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			state.PortfolioID, state.UserID, state.CheckpointIndex,
			state.SegmentStartValueBase, string(state.Status), state.TrackingStartedAt,
			state.SegmentStartedAt, state.UpdatedAt, state.Version)
		if err != nil {
			return fmt.Errorf("portfolio: insert ranked state: %w", err)
		}
		return nil
	}
	tag, err := t.tx.Exec(ctx, `
		UPDATE ranked_performance_state
		SET checkpoint_index=$3, segment_start_value_base=$4, status=$5,
		    segment_started_at=$6, updated_at=$7, version=$8
		WHERE portfolio_id=$1 AND version=$2`,
		state.PortfolioID, expectedVersion, state.CheckpointIndex,
		state.SegmentStartValueBase, string(state.Status), state.SegmentStartedAt,
		state.UpdatedAt, state.Version)
	if err != nil {
		return fmt.Errorf("portfolio: update ranked state: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return performance.ErrVersionConflict
	}
	return nil
}

func (t *postgresTx) SetPortfolioVersion(ctx context.Context, version int64) error {
	tag, err := t.tx.Exec(ctx,
		`UPDATE portfolios SET version=$2, updated_at=NOW() WHERE id=$1 AND version=$3`,
		t.portfolio.ID, version, t.portfolio.Version)
	if err != nil {
		return fmt.Errorf("portfolio: bump version: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrMutationConflict
	}
	return nil
}

func (t *postgresTx) AppendOutbox(ctx context.Context, ev OutboxEvent) error {
	_, err := t.tx.Exec(ctx, `
		INSERT INTO portfolio_outbox (
			id, event_type, aggregate_type, aggregate_id, aggregate_version,
			user_id, ranked_index, ranking_status, tracking_started_at,
			valuation_as_of, data_quality_status, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		ev.ID, string(ev.EventType), ev.AggregateType, ev.AggregateID,
		ev.AggregateVersion, ev.UserID, ev.RankedIndex, ev.RankingStatus,
		ev.TrackingStartedAt, ev.ValuationAsOf, ev.DataQualityStatus, ev.CreatedAt)
	if err != nil {
		return fmt.Errorf("portfolio: append outbox: %w", err)
	}
	return nil
}

func (t *postgresTx) RecordAudit(ctx context.Context, a MutationAudit) error {
	// A UNIQUE index on request_id makes a duplicate submission fail the
	// transaction rather than applying the mutation twice.
	_, err := t.tx.Exec(ctx, `
		INSERT INTO portfolio_mutation_audit (
			id, request_id, portfolio_id, user_id, mutation_type,
			portfolio_version_before, portfolio_version_after,
			performance_version_before, performance_version_after,
			ranked_index_before, ranked_index_after, result_position_id, occurred_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		a.ID, nullIfEmpty(a.RequestID), a.PortfolioID, a.UserID, a.MutationType,
		a.PortfolioVersionBefore, a.PortfolioVersionAfter,
		a.PerformanceVersionBefore, a.PerformanceVersionAfter,
		a.RankedIndexBefore, a.RankedIndexAfter, nullIfEmpty(a.ResultPositionID),
		a.OccurredAt)
	if err != nil {
		return fmt.Errorf("portfolio: record audit: %w", err)
	}
	return nil
}

func (t *postgresTx) FindAuditByRequestID(ctx context.Context, requestID string) (MutationAudit, bool, error) {
	var (
		a        MutationAudit
		resultID *string
	)
	err := t.tx.QueryRow(ctx, `
		SELECT id, request_id, portfolio_id, user_id, mutation_type,
		       portfolio_version_before, portfolio_version_after,
		       performance_version_before, performance_version_after,
		       ranked_index_before, ranked_index_after, result_position_id, occurred_at
		FROM portfolio_mutation_audit WHERE portfolio_id=$1 AND request_id=$2`,
		t.portfolio.ID, requestID).Scan(
		&a.ID, &a.RequestID, &a.PortfolioID, &a.UserID, &a.MutationType,
		&a.PortfolioVersionBefore, &a.PortfolioVersionAfter,
		&a.PerformanceVersionBefore, &a.PerformanceVersionAfter,
		&a.RankedIndexBefore, &a.RankedIndexAfter, &resultID, &a.OccurredAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return MutationAudit{}, false, nil
	}
	if err != nil {
		return MutationAudit{}, false, fmt.Errorf("portfolio: find audit: %w", err)
	}
	if resultID != nil {
		a.ResultPositionID = *resultID
	}
	return a, true, nil
}

func nullIfEmpty(v string) any {
	if v == "" {
		return nil
	}
	return v
}

// --- AggregateStore on the Postgres repository --------------------------------

// WithLockedPortfolio opens the mutation transaction boundary.
//
// Isolation: PostgreSQL default READ COMMITTED. Serializability for a single
// portfolio comes from the explicit row lock below, not the isolation level, so
// no stale application-level input can survive a concurrent change. Different
// portfolios never contend.
func (r *PostgresRepository) WithLockedPortfolio(ctx context.Context, userID string, fn func(context.Context, AggregateTx) error) error {
	// Ensure the aggregate exists before locking (race-safe via UNIQUE (user_id)).
	if _, err := r.EnsureDefaultPortfolio(ctx, userID); err != nil {
		return err
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("portfolio: begin mutation tx: %w", err)
	}
	defer tx.Rollback(context.WithoutCancel(ctx)) //nolint:errcheck // rollback is best-effort

	var pf Portfolio
	err = tx.QueryRow(ctx, `
		SELECT id, user_id, name, currency, COALESCE(version,1), created_at, updated_at
		FROM portfolios WHERE user_id=$1
		ORDER BY created_at
		FOR UPDATE`, userID).Scan(
		&pf.ID, &pf.UserID, &pf.Name, &pf.Currency, &pf.Version, &pf.CreatedAt, &pf.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrPortfolioNotFound
		}
		return fmt.Errorf("portfolio: lock aggregate: %w", err)
	}

	positions, err := scanPositions(ctx, tx,
		`SELECT `+positionColumns+` FROM positions WHERE portfolio_id=$1 ORDER BY created_at`, pf.ID)
	if err != nil {
		return err
	}
	cashRows, err := tx.Query(ctx, `
		SELECT portfolio_id, currency, amount, created_at, updated_at
		FROM portfolio_cash_balances WHERE portfolio_id=$1 ORDER BY currency
		FOR UPDATE`, pf.ID)
	if err != nil {
		return fmt.Errorf("portfolio: lock cash balances: %w", err)
	}
	cash := make([]CashBalance, 0)
	for cashRows.Next() {
		var balance CashBalance
		if err := cashRows.Scan(&balance.PortfolioID, &balance.Currency,
			&balance.Amount, &balance.CreatedAt, &balance.UpdatedAt); err != nil {
			cashRows.Close()
			return fmt.Errorf("portfolio: scan locked cash: %w", err)
		}
		cash = append(cash, balance)
	}
	err = cashRows.Err()
	cashRows.Close()
	if err != nil {
		return err
	}

	atx := &postgresTx{tx: tx, portfolio: &pf, positions: positions, cash: cash}
	if err := fn(ctx, atx); err != nil {
		return err // deferred rollback discards every write
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("portfolio: commit mutation tx: %w", err)
	}
	return nil
}

// outboxLeaseTTL is how long a claimed-but-unsettled event stays invisible to
// other workers. A worker that crashes mid-processing releases its events after
// this window, so no work is lost.
const outboxLeaseTTL = 5 * time.Minute

// ClaimOutboxEvents claims unprocessed events exclusively.
//
// Two mechanisms combine: FOR UPDATE SKIP LOCKED prevents two workers colliding
// on the same row *within* overlapping transactions, and stamping claimed_at
// makes the claim durable AFTER this transaction commits — without it, the row
// becomes visible again the moment the claim commits and a second worker would
// re-claim the same event.
func (r *PostgresRepository) ClaimOutboxEvents(ctx context.Context, limit int) ([]OutboxEvent, error) {
	rows, err := r.pool.Query(ctx, `
		UPDATE portfolio_outbox
		   SET attempt_count = attempt_count + 1, claimed_at = NOW()
		WHERE id IN (
			SELECT id FROM portfolio_outbox
			WHERE processed_at IS NULL
			  AND (claimed_at IS NULL OR claimed_at < NOW() - $2::interval)
			ORDER BY created_at
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, event_type, aggregate_type, aggregate_id, aggregate_version,
		          user_id, ranked_index, ranking_status, tracking_started_at,
		          valuation_as_of, data_quality_status, created_at, attempt_count`,
		limit, outboxLeaseTTL.String())
	if err != nil {
		return nil, fmt.Errorf("portfolio: claim outbox: %w", err)
	}
	defer rows.Close()

	out := make([]OutboxEvent, 0, limit)
	for rows.Next() {
		var (
			ev        OutboxEvent
			eventType string
		)
		if err := rows.Scan(&ev.ID, &eventType, &ev.AggregateType, &ev.AggregateID,
			&ev.AggregateVersion, &ev.UserID, &ev.RankedIndex, &ev.RankingStatus,
			&ev.TrackingStartedAt, &ev.ValuationAsOf, &ev.DataQualityStatus,
			&ev.CreatedAt, &ev.AttemptCount); err != nil {
			return nil, fmt.Errorf("portfolio: scan outbox: %w", err)
		}
		ev.EventType = OutboxEventType(eventType)
		out = append(out, ev)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) MarkOutboxProcessed(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE portfolio_outbox SET processed_at=$2, last_error=NULL WHERE id=$1 AND processed_at IS NULL`,
		id, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("portfolio: mark outbox processed: %w", err)
	}
	return nil
}

// MarkOutboxFailed records the failure and RELEASES the lease so the event is
// retried promptly rather than waiting for the lease to expire.
func (r *PostgresRepository) MarkOutboxFailed(ctx context.Context, id, cause string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE portfolio_outbox SET last_error=$2, claimed_at=NULL WHERE id=$1`, id, cause)
	if err != nil {
		return fmt.Errorf("portfolio: mark outbox failed: %w", err)
	}
	return nil
}

// newAuditID is used by callers that need a stable audit identifier.
func newAuditID() string { return uuid.NewString() }
