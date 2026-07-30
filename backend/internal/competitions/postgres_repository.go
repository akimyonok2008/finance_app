package competitions

import (
	"context"
	"errors"
	"fmt"
	"time"

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

var (
	_ CompetitionRepository      = (*PostgresCompetitionRepository)(nil)
	_ EditionRepository          = (*PostgresCompetitionRepository)(nil)
	_ DefinitionMetadataProvider = (*PostgresCompetitionRepository)(nil)
)

// DefinitionMetadata resolves one definition's display metadata for the
// Arena catalogue (see ArenaCompetitionSummary). A missing definition (should
// not happen given the foreign key, but guards against a race with deletion)
// simply yields empty strings rather than an error.
func (r *PostgresCompetitionRepository) DefinitionMetadata(ctx context.Context, definitionID string) (description, category, iconKey string, err error) {
	err = r.pool.QueryRow(ctx, `
		SELECT description, category, icon_key FROM competition_definitions WHERE id = $1
	`, definitionID).Scan(&description, &category, &iconKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", "", nil
	}
	if err != nil {
		return "", "", "", fmt.Errorf("competition repository: definition metadata: %w", err)
	}
	return description, category, iconKey, nil
}

// competitionColumns is the full edition-aware column list. Legacy rows scan
// cleanly: their edition columns are NULL/default and land as zero values.
const competitionColumns = `id, name, type, starts_at, ends_at, status, created_at,
	definition_id, definition_version, join_opens_at, join_closes_at,
	lifecycle_status, rules_snapshot_json, scoring_scope,
	published_at, finalized_at, cancelled_at`

func scanCompetition(row pgx.Row) (*Competition, error) {
	var c Competition
	var definitionID *string
	var definitionVersion *int64
	var scoringScope *string
	var rules []byte
	if err := row.Scan(&c.ID, &c.Name, &c.Type, &c.StartsAt, &c.EndsAt, &c.Status, &c.CreatedAt,
		&definitionID, &definitionVersion, &c.JoinOpensAt, &c.JoinClosesAt,
		&c.LifecycleStatus, &rules, &scoringScope,
		&c.PublishedAt, &c.FinalizedAt, &c.CancelledAt); err != nil {
		return nil, err
	}
	if definitionID != nil {
		c.DefinitionID = *definitionID
	}
	if definitionVersion != nil {
		c.DefinitionVersion = *definitionVersion
	}
	if scoringScope != nil {
		c.ScoringScope = *scoringScope
	}
	c.RulesSnapshotJSON = rules
	return &c, nil
}

func (r *PostgresCompetitionRepository) ListCompetitions(ctx context.Context) ([]Competition, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+competitionColumns+` FROM competitions ORDER BY starts_at`)
	if err != nil {
		return nil, fmt.Errorf("competition repository: list: %w", err)
	}
	defer rows.Close()

	var out []Competition
	for rows.Next() {
		c, err := scanCompetition(rows)
		if err != nil {
			return nil, fmt.Errorf("competition repository: scan: %w", err)
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func (r *PostgresCompetitionRepository) GetCompetition(ctx context.Context, competitionID string) (*Competition, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT `+competitionColumns+` FROM competitions WHERE id = $1`, competitionID)
	c, err := scanCompetition(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCompetitionNotFound
		}
		return nil, fmt.Errorf("competition repository: get: %w", err)
	}
	return c, nil
}

// CreateEdition inserts an engine edition: stored lifecycle plus the
// immutable rules snapshot stamped from its definition version. The legacy
// status column mirrors the lifecycle so raw rows stay self-describing; the
// legacy read path never rewrites it for non-legacy rows.
func (r *PostgresCompetitionRepository) CreateEdition(ctx context.Context, e Competition) error {
	if e.LifecycleStatus == "" || e.LifecycleStatus == LifecycleLegacy {
		return fmt.Errorf("competition repository: edition requires a real lifecycle status")
	}
	if len(e.RulesSnapshotJSON) == 0 {
		return fmt.Errorf("competition repository: edition requires a rules snapshot")
	}
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO competitions
			(id, name, type, starts_at, ends_at, status, created_at,
			 definition_id, definition_version, join_opens_at, join_closes_at,
			 lifecycle_status, rules_snapshot_json, scoring_scope)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (id) DO NOTHING
	`, e.ID, e.Name, e.Type, e.StartsAt, e.EndsAt, e.LifecycleStatus, e.CreatedAt,
		e.DefinitionID, e.DefinitionVersion, e.JoinOpensAt, e.JoinClosesAt,
		e.LifecycleStatus, e.RulesSnapshotJSON, e.ScoringScope)
	if err != nil {
		return fmt.Errorf("competition repository: create edition: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrEditionExists
	}
	return nil
}

// lifecycleStampColumns maps a destination lifecycle state to the timestamp
// column that transition stamps. Fixed strings only — never interpolate
// caller input into SQL.
var lifecycleStampColumns = map[string]string{
	LifecyclePublished: "published_at",
	LifecycleCompleted: "finalized_at",
	LifecycleCancelled: "cancelled_at",
}

// TransitionLifecycle applies from→to with the optimistic guard described on
// EditionRepository.
func (r *PostgresCompetitionRepository) TransitionLifecycle(ctx context.Context, competitionID, from, to string, now time.Time) error {
	if err := ValidateLifecycleTransition(from, to); err != nil {
		return err
	}
	query := `UPDATE competitions SET lifecycle_status = $1, status = $1`
	if col, ok := lifecycleStampColumns[to]; ok {
		query += `, ` + col + ` = $4`
	}
	query += ` WHERE id = $2 AND lifecycle_status = $3`

	args := []any{to, competitionID, from}
	if _, ok := lifecycleStampColumns[to]; ok {
		args = append(args, now.UTC())
	}
	tag, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("competition repository: transition lifecycle: %w", err)
	}
	if tag.RowsAffected() == 0 {
		if _, getErr := r.GetCompetition(ctx, competitionID); getErr != nil {
			return getErr
		}
		return ErrLifecycleConflict
	}
	return nil
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
