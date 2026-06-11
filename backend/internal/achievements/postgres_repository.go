package achievements

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresAchievementRepository is the durable implementation of
// AchievementRepository. The badge catalogue is seeded idempotently on
// construction (ON CONFLICT key DO NOTHING), so restarts never duplicate it.
type PostgresAchievementRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresAchievementRepository wires the repository and seeds the catalogue.
func NewPostgresAchievementRepository(ctx context.Context, pool *pgxpool.Pool) (*PostgresAchievementRepository, error) {
	r := &PostgresAchievementRepository{pool: pool}
	for _, a := range seedDefinitions(time.Now().UTC()) {
		_, err := pool.Exec(ctx,
			`INSERT INTO achievements (id, key, name, description, icon_key)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (key) DO NOTHING`,
			uuid.NewString(), a.Key, a.Name, a.Description, a.IconKey,
		)
		if err != nil {
			return nil, fmt.Errorf("achievement repository: seed %s: %w", a.Key, err)
		}
	}
	return r, nil
}

func (r *PostgresAchievementRepository) ListAchievements(ctx context.Context) ([]Achievement, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, key, name, description, icon_key, created_at FROM achievements ORDER BY created_at, key`)
	if err != nil {
		return nil, fmt.Errorf("achievement repository: list: %w", err)
	}
	defer rows.Close()

	var out []Achievement
	for rows.Next() {
		var a Achievement
		if err := rows.Scan(&a.ID, &a.Key, &a.Name, &a.Description, &a.IconKey, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("achievement repository: scan: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *PostgresAchievementRepository) GetAchievementByKey(ctx context.Context, key string) (*Achievement, error) {
	var a Achievement
	err := r.pool.QueryRow(ctx,
		`SELECT id, key, name, description, icon_key, created_at FROM achievements WHERE key = $1`, key,
	).Scan(&a.ID, &a.Key, &a.Name, &a.Description, &a.IconKey, &a.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAchievementNotFound
		}
		return nil, fmt.Errorf("achievement repository: get by key: %w", err)
	}
	return &a, nil
}

// UnlockAchievement is idempotent via ON CONFLICT DO NOTHING, preserving the
// original unlock time.
func (r *PostgresAchievementRepository) UnlockAchievement(ctx context.Context, userID, achievementID string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO user_achievements (user_id, achievement_id)
		 VALUES ($1, $2)
		 ON CONFLICT (user_id, achievement_id) DO NOTHING`,
		userID, achievementID,
	)
	if err != nil {
		return fmt.Errorf("achievement repository: unlock: %w", err)
	}
	return nil
}

func (r *PostgresAchievementRepository) HasAchievement(ctx context.Context, userID, achievementID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM user_achievements WHERE user_id = $1 AND achievement_id = $2)`,
		userID, achievementID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("achievement repository: has: %w", err)
	}
	return exists, nil
}

func (r *PostgresAchievementRepository) ListUserAchievements(ctx context.Context, userID string) ([]UserAchievement, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT user_id, achievement_id, unlocked_at FROM user_achievements WHERE user_id = $1`, userID)
	if err != nil {
		return nil, fmt.Errorf("achievement repository: list user achievements: %w", err)
	}
	defer rows.Close()

	var out []UserAchievement
	for rows.Next() {
		var ua UserAchievement
		if err := rows.Scan(&ua.UserID, &ua.AchievementID, &ua.UnlockedAt); err != nil {
			return nil, fmt.Errorf("achievement repository: scan user achievement: %w", err)
		}
		out = append(out, ua)
	}
	return out, rows.Err()
}
