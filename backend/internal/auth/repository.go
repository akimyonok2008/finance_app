package auth

import (
	"context"
	"strings"
	"sync"
)

// UserRepository is the persistence boundary for accounts. The service depends
// only on this interface, so InMemoryUserRepository can later be swapped for a
// PostgresUserRepository without touching business logic or handlers.
type UserRepository interface {
	// Create persists a new user. It returns ErrEmailExists if the (normalized)
	// email is already taken.
	Create(user *User) error
	// FindByEmail looks up a user by email. The email is normalized internally,
	// so callers may pass any casing. Returns ErrUserNotFound on a miss.
	FindByEmail(email string) (*User, error)
	// FindByID looks up a user by id. Returns ErrUserNotFound on a miss.
	FindByID(id string) (*User, error)
	// ListUsers returns all users. Order is unspecified; callers that need a
	// deterministic order must sort themselves.
	ListUsers(ctx context.Context) ([]User, error)
}

// InMemoryUserRepository is a goroutine-safe, process-local store used for the
// prototype milestone. It is indexed by both id and normalized email.
type InMemoryUserRepository struct {
	mu      sync.RWMutex
	byID    map[string]*User
	byEmail map[string]*User
}

// NewInMemoryUserRepository returns an empty in-memory repository.
func NewInMemoryUserRepository() *InMemoryUserRepository {
	return &InMemoryUserRepository{
		byID:    make(map[string]*User),
		byEmail: make(map[string]*User),
	}
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// Create stores a copy of the user, enforcing email uniqueness.
func (r *InMemoryUserRepository) Create(user *User) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := normalizeEmail(user.Email)
	if _, exists := r.byEmail[key]; exists {
		return ErrEmailExists
	}

	stored := *user // store a copy so callers can't mutate our state
	r.byID[stored.ID] = &stored
	r.byEmail[key] = &stored
	return nil
}

// FindByEmail returns a copy of the user with the given email.
func (r *InMemoryUserRepository) FindByEmail(email string) (*User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if u, ok := r.byEmail[normalizeEmail(email)]; ok {
		copied := *u
		return &copied, nil
	}
	return nil, ErrUserNotFound
}

// FindByID returns a copy of the user with the given id.
func (r *InMemoryUserRepository) FindByID(id string) (*User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if u, ok := r.byID[id]; ok {
		copied := *u
		return &copied, nil
	}
	return nil, ErrUserNotFound
}

// ListUsers returns copies of all stored users in unspecified order.
func (r *InMemoryUserRepository) ListUsers(_ context.Context) ([]User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]User, 0, len(r.byID))
	for _, u := range r.byID {
		out = append(out, *u)
	}
	return out, nil
}
