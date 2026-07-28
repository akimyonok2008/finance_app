package safety

import (
	"context"
	"sort"
	"sync"
	"time"
)

type InMemoryRepository struct {
	mu     sync.RWMutex
	blocks map[string]Block // key: blocker->blocked
}

func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{blocks: map[string]Block{}}
}

func blockKey(a, b string) string { return a + "->" + b }

func (r *InMemoryRepository) Block(_ context.Context, blockerUserID, blockedUserID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := blockKey(blockerUserID, blockedUserID)
	if _, ok := r.blocks[key]; ok {
		return nil
	}
	r.blocks[key] = Block{BlockerUserID: blockerUserID, BlockedUserID: blockedUserID, CreatedAt: time.Now().UTC()}
	return nil
}

func (r *InMemoryRepository) Unblock(_ context.Context, blockerUserID, blockedUserID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.blocks, blockKey(blockerUserID, blockedUserID))
	return nil
}

func (r *InMemoryRepository) IsBlocked(_ context.Context, blockerUserID, blockedUserID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.blocks[blockKey(blockerUserID, blockedUserID)]
	return ok, nil
}

func (r *InMemoryRepository) IsBlockedEitherDirection(_ context.Context, userA, userB string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ab := r.blocks[blockKey(userA, userB)]
	_, ba := r.blocks[blockKey(userB, userA)]
	return ab || ba, nil
}

func (r *InMemoryRepository) ListBlocked(_ context.Context, blockerUserID string) ([]Block, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Block, 0)
	for _, b := range r.blocks {
		if b.BlockerUserID == blockerUserID {
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (r *InMemoryRepository) BlockedPairUserIDs(_ context.Context, userID string) (map[string]bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := map[string]bool{}
	for _, b := range r.blocks {
		if b.BlockerUserID == userID {
			out[b.BlockedUserID] = true
		}
		if b.BlockedUserID == userID {
			out[b.BlockerUserID] = true
		}
	}
	return out, nil
}

// OnAccountDeleted implements auth.AccountDeletionHook: it removes every
// block relationship involving the deleted account.
func (r *InMemoryRepository) OnAccountDeleted(_ context.Context, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, b := range r.blocks {
		if b.BlockerUserID == userID || b.BlockedUserID == userID {
			delete(r.blocks, key)
		}
	}
	return nil
}
