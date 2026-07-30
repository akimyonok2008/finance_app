package competitions

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ardakimyonok/finance_app/internal/competitions/rules"
)

// DefinitionRepository is the persistence boundary for competition templates
// and their immutable rule versions. Definitions are administrative data:
// created by protected admin flows or the deterministic seed loader, never by
// end-user requests.
type DefinitionRepository interface {
	CreateDefinition(ctx context.Context, def Definition) error
	GetDefinition(ctx context.Context, id string) (*Definition, error)
	GetDefinitionBySlug(ctx context.Context, slug string) (*Definition, error)
	ListDefinitions(ctx context.Context, enabledOnly bool) ([]Definition, error)
	// CreateDefinitionVersion appends the next immutable version. The version
	// number must be exactly current_version+1 (append-only, no gaps, no
	// rewrites); anything else fails without touching stored versions.
	CreateDefinitionVersion(ctx context.Context, v DefinitionVersion) error
	GetDefinitionVersion(ctx context.Context, definitionID string, version int64) (*DefinitionVersion, error)
}

// PostgresDefinitionRepository is the durable DefinitionRepository.
type PostgresDefinitionRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresDefinitionRepository(pool *pgxpool.Pool) *PostgresDefinitionRepository {
	return &PostgresDefinitionRepository{pool: pool}
}

var _ DefinitionRepository = (*PostgresDefinitionRepository)(nil)

const definitionColumns = `id, slug, name, description, category, icon_key,
	presentation_config_json, is_enabled, current_version, created_at, updated_at`

func (r *PostgresDefinitionRepository) CreateDefinition(ctx context.Context, def Definition) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO competition_definitions
			(id, slug, name, description, category, icon_key, presentation_config_json, is_enabled)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, def.ID, def.Slug, def.Name, def.Description, def.Category, def.IconKey,
		def.PresentationConfigJSON, def.IsEnabled)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrDefinitionExists
		}
		return fmt.Errorf("definition repository: create: %w", err)
	}
	return nil
}

func (r *PostgresDefinitionRepository) GetDefinition(ctx context.Context, id string) (*Definition, error) {
	return r.getDefinitionWhere(ctx, `id = $1`, id)
}

func (r *PostgresDefinitionRepository) GetDefinitionBySlug(ctx context.Context, slug string) (*Definition, error) {
	return r.getDefinitionWhere(ctx, `slug = $1`, slug)
}

func (r *PostgresDefinitionRepository) getDefinitionWhere(ctx context.Context, where string, arg any) (*Definition, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT `+definitionColumns+` FROM competition_definitions WHERE `+where, arg)
	def, err := scanDefinition(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrDefinitionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("definition repository: get: %w", err)
	}
	return def, nil
}

func (r *PostgresDefinitionRepository) ListDefinitions(ctx context.Context, enabledOnly bool) ([]Definition, error) {
	query := `SELECT ` + definitionColumns + ` FROM competition_definitions`
	if enabledOnly {
		query += ` WHERE is_enabled`
	}
	query += ` ORDER BY slug`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("definition repository: list: %w", err)
	}
	defer rows.Close()

	var out []Definition
	for rows.Next() {
		def, err := scanDefinition(rows)
		if err != nil {
			return nil, fmt.Errorf("definition repository: scan: %w", err)
		}
		out = append(out, *def)
	}
	return out, rows.Err()
}

func scanDefinition(row pgx.Row) (*Definition, error) {
	var def Definition
	var presentation []byte
	if err := row.Scan(&def.ID, &def.Slug, &def.Name, &def.Description, &def.Category,
		&def.IconKey, &presentation, &def.IsEnabled, &def.CurrentVersion,
		&def.CreatedAt, &def.UpdatedAt); err != nil {
		return nil, err
	}
	def.PresentationConfigJSON = presentation
	return &def, nil
}

// CreateDefinitionVersion appends v as the definition's next version inside
// one transaction: the definition row is locked, the version number is
// verified to be exactly current_version+1, the immutable version row is
// inserted, and current_version advances. There is deliberately NO update
// path for existing versions anywhere in this repository.
func (r *PostgresDefinitionRepository) CreateDefinitionVersion(ctx context.Context, v DefinitionVersion) error {
	// Admission check: an invalid rule document must never be persisted —
	// every stored version is guaranteed parseable by the typed engine.
	if _, _, err := rules.ValidateDefinitionVersionPayloads(v.EligibilityRulesJSON, v.ScoringRulesJSON); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidRuleDocument, err)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("definition repository: begin version tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var current int64
	err = tx.QueryRow(ctx, `
		SELECT current_version FROM competition_definitions WHERE id = $1 FOR UPDATE
	`, v.DefinitionID).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrDefinitionNotFound
	}
	if err != nil {
		return fmt.Errorf("definition repository: lock definition: %w", err)
	}
	if v.Version <= current {
		return ErrDefinitionVersionExists
	}
	if v.Version != current+1 {
		return fmt.Errorf("definition repository: versions are append-only: got %d, next is %d", v.Version, current+1)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO competition_definition_versions
			(definition_id, version, eligibility_rules_json, scoring_rules_json,
			 schedule_defaults_json, display_rules_json, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, v.DefinitionID, v.Version, v.EligibilityRulesJSON, v.ScoringRulesJSON,
		v.ScheduleDefaultsJSON, v.DisplayRulesJSON, v.CreatedBy); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrDefinitionVersionExists
		}
		return fmt.Errorf("definition repository: insert version: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE competition_definitions SET current_version = $1, updated_at = now() WHERE id = $2
	`, v.Version, v.DefinitionID); err != nil {
		return fmt.Errorf("definition repository: advance current_version: %w", err)
	}
	return tx.Commit(ctx)
}

func (r *PostgresDefinitionRepository) GetDefinitionVersion(ctx context.Context, definitionID string, version int64) (*DefinitionVersion, error) {
	var v DefinitionVersion
	var eligibility, scoring, schedule, display []byte
	var createdAt time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT definition_id, version, eligibility_rules_json, scoring_rules_json,
		       schedule_defaults_json, display_rules_json, created_at, created_by
		FROM competition_definition_versions
		WHERE definition_id = $1 AND version = $2
	`, definitionID, version).Scan(&v.DefinitionID, &v.Version, &eligibility, &scoring,
		&schedule, &display, &createdAt, &v.CreatedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrDefinitionVersionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("definition repository: get version: %w", err)
	}
	v.EligibilityRulesJSON = eligibility
	v.ScoringRulesJSON = scoring
	v.ScheduleDefaultsJSON = schedule
	v.DisplayRulesJSON = display
	v.CreatedAt = createdAt
	return &v, nil
}
