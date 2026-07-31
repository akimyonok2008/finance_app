package competitions

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ardakimyonok/finance_app/internal/money"
)

// Postgres implementation of EngineEntryRepository on the existing
// PostgresCompetitionRepository (same tables, engine columns from migration
// 0046).

var _ EngineEntryRepository = (*PostgresCompetitionRepository)(nil)

// CreateEngineEntry writes the entry, its position snapshots (with identity,
// classification and inclusion flags) and its cash snapshots in ONE
// transaction — a partial engine entry can never exist.
func (r *PostgresCompetitionRepository) CreateEngineEntry(ctx context.Context, entry CompetitionEntry) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("competition repository: begin engine entry tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx, `
		INSERT INTO competition_entries
			(id, competition_id, user_id, starting_value_base, starting_index, joined_at,
			 entry_status, portfolio_version, snapshot_captured_at, eligibility_evidence_json,
			 scoring_scope, eligible_starting_value_base, baseline_status,
			 idempotency_key, request_fingerprint)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`, entry.ID, entry.CompetitionID, entry.UserID, entry.StartingValue, entry.StartingIndex,
		entry.JoinedAt, entry.EntryStatus, entry.PortfolioVersion, entry.SnapshotCapturedAt,
		entry.EligibilityEvidenceJSON, entry.ScoringScope, entry.EligibleStartingValueBase,
		entry.BaselineStatus, nullableString(entry.IdempotencyKey), nullableString(entry.RequestFingerprint))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrEntryExists
		}
		return fmt.Errorf("competition repository: create engine entry: %w", err)
	}

	for _, s := range entry.Snapshots {
		if _, err := tx.Exec(ctx, `
			INSERT INTO competition_entry_snapshot_positions
				(id, competition_entry_id, symbol, asset_type, quantity, currency,
				 starting_price, starting_price_currency, starting_value_base,
				 instrument_id, venue_mic, classification_snapshot_json, included_in_score)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		`, s.ID, s.CompetitionEntryID, s.Symbol, s.AssetType, s.Quantity, s.Currency,
			s.StartingPrice, s.StartingPriceCurrency, s.StartingValueBase,
			nullableUUID(s.InstrumentID), s.VenueMIC, s.ClassificationSnapshotJSON, s.IncludedInScore); err != nil {
			return fmt.Errorf("competition repository: create engine snapshot: %w", err)
		}
	}
	for _, c := range entry.CashSnapshots {
		if _, err := tx.Exec(ctx, `
			INSERT INTO competition_entry_snapshot_cash
				(id, competition_entry_id, currency, amount, value_base, included_in_score)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, c.ID, c.CompetitionEntryID, c.Currency, c.Amount, c.ValueBase, c.IncludedInScore); err != nil {
			return fmt.Errorf("competition repository: create cash snapshot: %w", err)
		}
	}
	return tx.Commit(ctx)
}

// nullableUUID maps "" to NULL for optional UUID columns.
func nullableUUID(v string) any {
	if v == "" {
		return nil
	}
	return v
}

// nullableString maps "" to NULL for optional text columns.
func nullableString(v string) any {
	if v == "" {
		return nil
	}
	return v
}

// FindEntryByIdempotencyKey looks up an entry this user previously created
// with the given Idempotency-Key, regardless of competition — used to catch
// a key reused across two different join requests.
func (r *PostgresCompetitionRepository) FindEntryByIdempotencyKey(ctx context.Context, userID, idempotencyKey string) (*CompetitionEntry, bool, error) {
	if idempotencyKey == "" {
		return nil, false, nil
	}
	var e CompetitionEntry
	err := r.pool.QueryRow(ctx, `
		SELECT id, competition_id, user_id, starting_value_base, starting_index, joined_at,
		       entry_status, eligibility_evidence_json, idempotency_key, request_fingerprint
		FROM competition_entries WHERE user_id = $1 AND idempotency_key = $2
	`, userID, idempotencyKey).Scan(&e.ID, &e.CompetitionID, &e.UserID, &e.StartingValue, &e.StartingIndex,
		&e.JoinedAt, &e.EntryStatus, &e.EligibilityEvidenceJSON, &e.IdempotencyKey, &e.RequestFingerprint)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("competition repository: find entry by idempotency key: %w", err)
	}
	return &e, true, nil
}

func (r *PostgresCompetitionRepository) UpdateEntryStatus(ctx context.Context, entryID, from, to string, now time.Time) error {
	query := `UPDATE competition_entries SET entry_status = $1`
	if to == EntryWithdrawn {
		query += `, withdrawn_at = $4`
	}
	query += ` WHERE id = $2 AND entry_status = $3`
	args := []any{to, entryID, from}
	if to == EntryWithdrawn {
		args = append(args, now.UTC())
	}
	tag, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("competition repository: update entry status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrEntryConflict
	}
	return nil
}

func (r *PostgresCompetitionRepository) ListEditionsByLifecycle(ctx context.Context, lifecycle string) ([]Competition, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+competitionColumns+` FROM competitions
		 WHERE lifecycle_status = $1 AND definition_id IS NOT NULL
		 ORDER BY starts_at`, lifecycle)
	if err != nil {
		return nil, fmt.Errorf("competition repository: list by lifecycle: %w", err)
	}
	defer rows.Close()
	var out []Competition
	for rows.Next() {
		c, err := scanCompetition(rows)
		if err != nil {
			return nil, fmt.Errorf("competition repository: scan edition: %w", err)
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func (r *PostgresCompetitionRepository) ListBaselineDueEditions(ctx context.Context, now time.Time) ([]Competition, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+competitionColumns+` FROM competitions
		 WHERE lifecycle_status IN ($1, $2) AND starts_at <= $3 AND definition_id IS NOT NULL
		 ORDER BY starts_at`, LifecycleRegistrationClosed, LifecycleActive, now.UTC())
	if err != nil {
		return nil, fmt.Errorf("competition repository: list baseline due: %w", err)
	}
	defer rows.Close()
	var out []Competition
	for rows.Next() {
		c, err := scanCompetition(rows)
		if err != nil {
			return nil, fmt.Errorf("competition repository: scan edition: %w", err)
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func (r *PostgresCompetitionRepository) ListPendingBaselineEntries(ctx context.Context, competitionID string, limit int) ([]CompetitionEntry, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, competition_id, user_id, starting_value_base, starting_index, joined_at
		FROM competition_entries
		WHERE competition_id = $1 AND entry_status = $2 AND baseline_status = $3
		ORDER BY joined_at
		LIMIT $4
	`, competitionID, EntryAdmitted, BaselinePending, limit)
	if err != nil {
		return nil, fmt.Errorf("competition repository: list pending baselines: %w", err)
	}
	defer rows.Close()

	var out []CompetitionEntry
	for rows.Next() {
		var e CompetitionEntry
		if err := rows.Scan(&e.ID, &e.CompetitionID, &e.UserID, &e.StartingValue, &e.StartingIndex, &e.JoinedAt); err != nil {
			return nil, fmt.Errorf("competition repository: scan pending entry: %w", err)
		}
		e.EntryStatus = EntryAdmitted
		e.BaselineStatus = BaselinePending
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if err := r.loadEngineSnapshots(ctx, &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// HasPendingBaselineEntries reports whether any entry is still admitted
// (awaiting baseline) for this competition — see the interface doc.
func (r *PostgresCompetitionRepository) HasPendingBaselineEntries(ctx context.Context, competitionID string) (bool, error) {
	var exists bool
	if err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM competition_entries WHERE competition_id = $1 AND entry_status = $2)
	`, competitionID, EntryAdmitted).Scan(&exists); err != nil {
		return false, fmt.Errorf("competition repository: has pending baseline entries: %w", err)
	}
	return exists, nil
}

// loadEngineSnapshots loads position snapshots (with inclusion flags) and
// cash snapshots for one entry.
func (r *PostgresCompetitionRepository) loadEngineSnapshots(ctx context.Context, e *CompetitionEntry) error {
	rows, err := r.pool.Query(ctx, `
		SELECT id, competition_entry_id, symbol, asset_type, quantity, currency,
		       starting_price, starting_price_currency, starting_value_base,
		       COALESCE(instrument_id::text, ''), COALESCE(venue_mic, ''), included_in_score
		FROM competition_entry_snapshot_positions
		WHERE competition_entry_id = $1 ORDER BY created_at
	`, e.ID)
	if err != nil {
		return fmt.Errorf("competition repository: load engine snapshots: %w", err)
	}
	defer rows.Close()
	e.Snapshots = nil
	for rows.Next() {
		var s CompetitionEntrySnapshotPosition
		if err := rows.Scan(&s.ID, &s.CompetitionEntryID, &s.Symbol, &s.AssetType, &s.Quantity,
			&s.Currency, &s.StartingPrice, &s.StartingPriceCurrency, &s.StartingValueBase,
			&s.InstrumentID, &s.VenueMIC, &s.IncludedInScore); err != nil {
			return fmt.Errorf("competition repository: scan engine snapshot: %w", err)
		}
		e.Snapshots = append(e.Snapshots, s)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	cashRows, err := r.pool.Query(ctx, `
		SELECT id, competition_entry_id, currency, amount, value_base, included_in_score
		FROM competition_entry_snapshot_cash
		WHERE competition_entry_id = $1 ORDER BY created_at
	`, e.ID)
	if err != nil {
		return fmt.Errorf("competition repository: load cash snapshots: %w", err)
	}
	defer cashRows.Close()
	e.CashSnapshots = nil
	for cashRows.Next() {
		var c CompetitionEntrySnapshotCash
		if err := cashRows.Scan(&c.ID, &c.CompetitionEntryID, &c.Currency, &c.Amount, &c.ValueBase, &c.IncludedInScore); err != nil {
			return fmt.Errorf("competition repository: scan cash snapshot: %w", err)
		}
		e.CashSnapshots = append(e.CashSnapshots, c)
	}
	return cashRows.Err()
}

// CompleteBaseline writes the official baseline atomically, guarded on the
// entry still awaiting it — replaying workers and concurrent replicas hit
// zero rows and back off with ErrEntryConflict.
func (r *PostgresCompetitionRepository) CompleteBaseline(ctx context.Context, entryID string, eligibleStartingValue money.Amount, baselines []PositionBaseline, now time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("competition repository: begin baseline tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		UPDATE competition_entries
		SET entry_status = $1, baseline_status = $2, baseline_completed_at = $3,
		    eligible_starting_value_base = $4, starting_value_base = $4
		WHERE id = $5 AND entry_status = $6 AND baseline_status = $7
	`, EntryActive, BaselineCompleted, now.UTC(), eligibleStartingValue,
		entryID, EntryAdmitted, BaselinePending)
	if err != nil {
		return fmt.Errorf("competition repository: complete baseline: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrEntryConflict
	}

	for _, b := range baselines {
		if _, err := tx.Exec(ctx, `
			UPDATE competition_entry_snapshot_positions
			SET symbol = $1, quantity = $2, starting_price = $3, starting_price_currency = $4,
			    starting_value_base = $5, starting_weight = $6, baseline_price_observed_at = $7
			WHERE id = $8 AND competition_entry_id = $9
		`, b.Symbol, b.Quantity, b.Price, b.PriceCurrency, b.ValueBase, b.Weight,
			b.ObservedAt.UTC(), b.SnapshotID, entryID); err != nil {
			return fmt.Errorf("competition repository: write position baseline: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func (r *PostgresCompetitionRepository) FailBaseline(ctx context.Context, entryID, reason string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE competition_entries
		SET entry_status = $1, baseline_status = $2, disqualification_reason = $3
		WHERE id = $4 AND entry_status = $5 AND baseline_status = $6
	`, EntryBaselineFailed, BaselineFailed, reason, entryID, EntryAdmitted, BaselinePending)
	if err != nil {
		return fmt.Errorf("competition repository: fail baseline: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrEntryConflict
	}
	return nil
}
