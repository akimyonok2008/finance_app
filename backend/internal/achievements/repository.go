package achievements

import (
	"context"
	"sync"
	"time"
)

// AchievementRepository is the persistence boundary for badge definitions and
// per-user unlocks. The service depends only on this interface.
type AchievementRepository interface {
	ListAchievements(ctx context.Context) ([]Achievement, error)
	GetAchievementByKey(ctx context.Context, key string) (*Achievement, error)
	UnlockAchievement(ctx context.Context, userID, achievementID string) error
	HasAchievement(ctx context.Context, userID, achievementID string) (bool, error)
	ListUserAchievements(ctx context.Context, userID string) ([]UserAchievement, error)
}

// InMemoryAchievementRepository seeds the badge catalogue and tracks unlocks.
type InMemoryAchievementRepository struct {
	mu           sync.RWMutex
	achievements []Achievement
	byKey        map[string]Achievement
	// unlocked: userID -> achievementID -> unlockedAt
	unlocked map[string]map[string]time.Time
}

// NewInMemoryAchievementRepository returns a repository seeded with the default
// badge catalogue.
func NewInMemoryAchievementRepository() *InMemoryAchievementRepository {
	now := time.Now().UTC()
	defs := seedDefinitions(now)
	byKey := make(map[string]Achievement, len(defs))
	for _, a := range defs {
		byKey[a.Key] = a
	}
	return &InMemoryAchievementRepository{
		achievements: defs,
		byKey:        byKey,
		unlocked:     make(map[string]map[string]time.Time),
	}
}

func (r *InMemoryAchievementRepository) ListAchievements(_ context.Context) ([]Achievement, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Achievement, len(r.achievements))
	copy(out, r.achievements)
	return out, nil
}

func (r *InMemoryAchievementRepository) GetAchievementByKey(_ context.Context, key string) (*Achievement, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.byKey[key]
	if !ok {
		return nil, ErrAchievementNotFound
	}
	return &a, nil
}

// UnlockAchievement is idempotent: unlocking an already-unlocked badge keeps the
// original unlock time and does not duplicate.
func (r *InMemoryAchievementRepository) UnlockAchievement(_ context.Context, userID, achievementID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.unlocked[userID] == nil {
		r.unlocked[userID] = make(map[string]time.Time)
	}
	if _, exists := r.unlocked[userID][achievementID]; exists {
		return nil
	}
	r.unlocked[userID][achievementID] = time.Now().UTC()
	return nil
}

func (r *InMemoryAchievementRepository) HasAchievement(_ context.Context, userID, achievementID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.unlocked[userID][achievementID]
	return ok, nil
}

func (r *InMemoryAchievementRepository) ListUserAchievements(_ context.Context, userID string) ([]UserAchievement, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]UserAchievement, 0, len(r.unlocked[userID]))
	for achID, at := range r.unlocked[userID] {
		out = append(out, UserAchievement{UserID: userID, AchievementID: achID, UnlockedAt: at})
	}
	return out, nil
}
