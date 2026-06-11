package competitions

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresCompetitionRepository is the durable implementation of
// CompetitionRepository. Entries and their snapshot positions are written in a
// single transaction so a partial snapshot can never exist.
type PostgresCompetitionRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresCompetitionRepository wires a Postgres-backed competition repository.
func NewPostgresCompetitionRepository(pool *pgxpool.Pool) *PostgresCompetitionRepository {
	return &PostgresCompetitionRepository{pool: pool}
}

func (r *PostgresCompetitionRepository) ListCompetitions(ctx context.Context) ([]Competition, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, type, starts_at, ends_at, status, created_at
		 FROM competitions ORDER BY starts_at`)
	if err != nil {
		return nil, fmt.Errorf("competition repository: list: %w", err)
	}
	defer rows.Close()

	var out []Competition
	for rows.Next() {
		var c Competition
		if err := rows.Scan(&c.ID, &c.Name, &c.Type, &c.StartsAt, &c.EndsAt, &c.Status, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("competition repository: scan: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *PostgresCompetitionRepository) GetCompetition(ctx context.Context, competitionID string) (*Competition, error) {
	var c Competition
	err := r.pool.QueryRow(ctx,
		`SELECT id, name, type, starts_at, ends_at, status, created_at
		 FROM competitions WHERE id = $1`, competitionID,
	).Scan(&c.ID, &c.Name, &c.Type, &c.StartsAt, &c.EndsAt, &c.Status, &c.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCompetitionNotFound
		}
		return nil, fmt.Errorf("competition repository: get: %w", err)
	}
	return &c, nil
}

func (r *PostgresCompetitionRepository) CreateCompetition(ctx context.Context, competition Competition) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO competitions (id, name, type, starts_at, ends_at, status, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (id) DO NOTHING`,
		competition.ID, competition.Name, competition.Type,
		competition.StartsAt, competition.EndsAt, competition.Status, competition.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("competition repository: create: %w", err)
	}
	return nil
}

// CreateEntry stores the entry and all of its snapshot positions atomically.
func (r *PostgresCompetitionRepository) CreateEntry(ctx context.Context, entry CompetitionEntry) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("competition repository: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx,
		`INSERT INTO competition_entries
		 (id, competition_id, user_id, starting_value_base, starting_index, joined_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		entry.ID, entry.CompetitionID, entry.UserID,
		entry.StartingValue, entry.StartingIndex, entry.JoinedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // duplicate (competition_id, user_id)
			return nil // idempotent: the existing entry stands
		}
		return fmt.Errorf("competition repository: create entry: %w", err)
	}

	for _, s := range entry.Snapshots {
		_, err = tx.Exec(ctx,
			`INSERT INTO competition_entry_snapshot_positions
			 (id, competition_entry_id, symbol, asset_type, quantity, currency,
			  starting_price, starting_price_currency, starting_value_base)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			s.ID, s.CompetitionEntryID, s.Symbol, s.AssetType, s.Quantity, s.Currency,
			s.StartingPrice, s.StartingPriceCurrency, s.StartingValueBase,
		)
		if err != nil {
			return fmt.Errorf("competition repository: create snapshot: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func (r *PostgresCompetitionRepository) GetEntry(ctx context.Context, competitionID, userID string) (*CompetitionEntry, error) {
	var e CompetitionEntry
	err := r.pool.QueryRow(ctx,
		`SELECT id, competition_id, user_id, starting_value_base, starting_index, joined_at
		 FROM competition_entries WHERE competition_id = $1 AND user_id = $2`,
		competitionID, userID,
	).Scan(&e.ID, &e.CompetitionID, &e.UserID, &e.StartingValue, &e.StartingIndex, &e.JoinedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrEntryNotFound
		}
		return nil, fmt.Errorf("competition repository: get entry: %w", err)
	}
	snaps, err := r.loadSnapshots(ctx, e.ID)
	if err != nil {
		return nil, err
	}
	e.Snapshots = snaps
	return &e, nil
}

func (r *PostgresCompetitionRepository) ListEntries(ctx context.Context, competitionID string) ([]CompetitionEntry, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, competition_id, user_id, starting_value_base, starting_index, joined_at
		 FROM competition_entries WHERE competition_id = $1 ORDER BY joined_at`, competitionID)
	if err != nil {
		return nil, fmt.Errorf("competition repository: list entries: %w", err)
	}
	defer rows.Close()

	var out []CompetitionEntry
	for rows.Next() {
		var e CompetitionEntry
		if err := rows.Scan(&e.ID, &e.CompetitionID, &e.UserID, &e.StartingValue, &e.StartingIndex, &e.JoinedAt); err != nil {
			return nil, fmt.Errorf("competition repository: scan entry: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range out {
		snaps, err := r.loadSnapshots(ctx, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Snapshots = snaps
	}
	return out, nil
}

func (r *PostgresCompetitionRepository) loadSnapshots(ctx context.Context, entryID string) ([]CompetitionEntrySnapshotPosition, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, competition_entry_id, symbol, asset_type, quantity, currency,
		        starting_price, starting_price_currency, starting_value_base
		 FROM competition_entry_snapshot_positions
		 WHERE competition_entry_id = $1 ORDER BY created_at`, entryID)
	if err != nil {
		return nil, fmt.Errorf("competition repository: load snapshots: %w", err)
	}
	defer rows.Close()

	var out []CompetitionEntrySnapshotPosition
	for rows.Next() {
		var s CompetitionEntrySnapshotPosition
		if err := rows.Scan(&s.ID, &s.CompetitionEntryID, &s.Symbol, &s.AssetType, &s.Quantity,
			&s.Currency, &s.StartingPrice, &s.StartingPriceCurrency, &s.StartingValueBase); err != nil {
			return nil, fmt.Errorf("competition repository: scan snapshot: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
