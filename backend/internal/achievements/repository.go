package achievements

import (
	"context"
	"sync"
	"time"
)

// AchievementRepository is the persistence boundary for awarded benchmark
// badges. Badge definitions themselves live in code (benchmark.Badges), so the
// repository only tracks per-user unlocks and their evidence.
type AchievementRepository interface {
	// ListAwarded returns the user's awarded badges keyed by badge key.
	ListAwarded(ctx context.Context, userID string) (map[string]AwardedAchievement, error)
	// Award persists an unlock idempotently: awarding an already-awarded badge
	// keeps the original unlock time and evidence.
	Award(ctx context.Context, a AwardedAchievement) error
}

// InMemoryAchievementRepository tracks awarded badges in memory.
type InMemoryAchievementRepository struct {
	mu sync.RWMutex
	// awarded: userID -> badgeKey -> record
	awarded map[string]map[string]AwardedAchievement
}

// NewInMemoryAchievementRepository returns an empty in-memory repository.
func NewInMemoryAchievementRepository() *InMemoryAchievementRepository {
	return &InMemoryAchievementRepository{awarded: make(map[string]map[string]AwardedAchievement)}
}

func (r *InMemoryAchievementRepository) ListAwarded(_ context.Context, userID string) (map[string]AwardedAchievement, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]AwardedAchievement, len(r.awarded[userID]))
	for k, v := range r.awarded[userID] {
		out[k] = v
	}
	return out, nil
}

func (r *InMemoryAchievementRepository) Award(_ context.Context, a AwardedAchievement) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.awarded[a.UserID] == nil {
		r.awarded[a.UserID] = make(map[string]AwardedAchievement)
	}
	if _, exists := r.awarded[a.UserID][a.BadgeKey]; exists {
		return nil // idempotent
	}
	if a.UnlockedAt.IsZero() {
		a.UnlockedAt = time.Now().UTC()
	}
	r.awarded[a.UserID][a.BadgeKey] = a
	return nil
}

func (r *InMemoryAchievementRepository) OnAccountDeleted(_ context.Context, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.awarded, userID)
	return nil
}
