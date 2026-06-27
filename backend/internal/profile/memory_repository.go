package profile

import (
	"context"
	"sync"
)

type InMemoryRepository struct {
	mu       sync.RWMutex
	byUserID map[string]Profile
	byHandle map[string]string
}

func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		byUserID: make(map[string]Profile),
		byHandle: make(map[string]string),
	}
}

func (r *InMemoryRepository) Create(_ context.Context, p Profile) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.byUserID[p.UserID]; exists {
		return ErrHandleExists
	}
	if _, exists := r.byHandle[p.Handle]; exists {
		return ErrHandleExists
	}
	r.byUserID[p.UserID] = p
	r.byHandle[p.Handle] = p.UserID
	return nil
}

func (r *InMemoryRepository) GetByUserID(_ context.Context, userID string) (Profile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.byUserID[userID]
	if !ok {
		return Profile{}, ErrNotFound
	}
	return p, nil
}

func (r *InMemoryRepository) GetByHandle(_ context.Context, handle string) (Profile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	userID, ok := r.byHandle[handle]
	if !ok {
		return Profile{}, ErrNotFound
	}
	return r.byUserID[userID], nil
}

func (r *InMemoryRepository) ListPublicProfiles(_ context.Context) ([]Profile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Profile, 0)
	for _, p := range r.byUserID {
		if p.IsPublic {
			out = append(out, p)
		}
	}
	return out, nil
}

func (r *InMemoryRepository) Update(_ context.Context, p Profile) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	current, exists := r.byUserID[p.UserID]
	if !exists {
		return ErrNotFound
	}
	if owner, exists := r.byHandle[p.Handle]; exists && owner != p.UserID {
		return ErrHandleExists
	}
	delete(r.byHandle, current.Handle)
	r.byUserID[p.UserID] = p
	r.byHandle[p.Handle] = p.UserID
	return nil
}
