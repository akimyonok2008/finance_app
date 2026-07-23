package achievements

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ardakimyonok/finance_app/internal/benchmark"
)

// PostgresAchievementRepository is the durable implementation of
// AchievementRepository. Badge definitions live in code (benchmark.Badges); this
// table stores only per-user unlocks and their evidence.
type PostgresAchievementRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresAchievementRepository wires the repository. Definitions are not
// seeded — the catalogue is code-defined.
func NewPostgresAchievementRepository(_ context.Context, pool *pgxpool.Pool) (*PostgresAchievementRepository, error) {
	return &PostgresAchievementRepository{pool: pool}, nil
}

func (r *PostgresAchievementRepository) ListAwarded(ctx context.Context, userID string) (map[string]AwardedAchievement, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT user_id, badge_key, unlocked_at, evidence
		 FROM user_benchmark_achievements
		 WHERE user_id = $1`, userID)
	if err != nil {
		return nil, fmt.Errorf("achievement repository: list awarded: %w", err)
	}
	defer rows.Close()

	out := make(map[string]AwardedAchievement)
	for rows.Next() {
		var (
			a           AwardedAchievement
			evidenceRaw []byte
		)
		if err := rows.Scan(&a.UserID, &a.BadgeKey, &a.UnlockedAt, &evidenceRaw); err != nil {
			return nil, fmt.Errorf("achievement repository: scan awarded: %w", err)
		}
		if len(evidenceRaw) > 0 {
			var ev benchmark.AchievementEvidence
			if err := json.Unmarshal(evidenceRaw, &ev); err != nil {
				return nil, fmt.Errorf("achievement repository: decode evidence: %w", err)
			}
			a.Evidence = ev
		}
		out[a.BadgeKey] = a
	}
	return out, rows.Err()
}

// Award is idempotent via ON CONFLICT DO NOTHING, preserving the original unlock
// time and evidence.
func (r *PostgresAchievementRepository) Award(ctx context.Context, a AwardedAchievement) error {
	evidenceRaw, err := json.Marshal(a.Evidence)
	if err != nil {
		return fmt.Errorf("achievement repository: encode evidence: %w", err)
	}
	_, err = r.pool.Exec(ctx,
		`INSERT INTO user_benchmark_achievements (user_id, badge_key, unlocked_at, evidence)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (user_id, badge_key) DO NOTHING`,
		a.UserID, a.BadgeKey, a.UnlockedAt, evidenceRaw,
	)
	if err != nil {
		return fmt.Errorf("achievement repository: award: %w", err)
	}
	return nil
}
