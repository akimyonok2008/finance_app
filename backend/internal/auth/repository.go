package auth

import (
	"context"
	"crypto/subtle"
	"strings"
	"sync"
	"time"
)

// UserRepository is the persistence boundary for accounts. The service depends
// only on this interface, so InMemoryUserRepository can later be swapped for a
// PostgresUserRepository without touching business logic or handlers.
type UserRepository interface {
	// Create persists a new user. It returns ErrEmailExists if the (normalized)
	// email is already taken.
	Create(user *User) error
	// CreateWithVerification atomically persists a password user, its initial
	// verification token, and the corresponding durable email-outbox message.
	// A failure must leave none of the three records behind.
	CreateWithVerification(ctx context.Context, user *User, token LifecycleToken, email EmailOutboxMessage) error
	// FindByEmail looks up a user by email. The email is normalized internally,
	// so callers may pass any casing. Returns ErrUserNotFound on a miss.
	FindByEmail(email string) (*User, error)
	// FindByID looks up a user by id. Returns ErrUserNotFound on a miss.
	FindByID(id string) (*User, error)
	// ListUsers returns all users. Order is unspecified; callers that need a
	// deterministic order must sort themselves.
	ListUsers(ctx context.Context) ([]User, error)
	FindIdentity(provider AuthProvider, subject string) (*AuthIdentity, error)
	CreateIdentity(identity *AuthIdentity) error
	// UpdatePassword sets a new password hash for an existing, non-deleted
	// user. Returns ErrUserNotFound if the user doesn't exist (or is deleted).
	UpdatePassword(userID, passwordHash string) error
	UpdatePasswordAndRevoke(userID, passwordHash string) (*User, error)
	IncrementAuthVersion(userID string) (*User, error)
	SaveEmailVerificationToken(ctx context.Context, token LifecycleToken) error
	VerifyEmailToken(ctx context.Context, tokenHash string, now time.Time) (*User, error)
	SavePasswordResetToken(ctx context.Context, token LifecycleToken) error
	ResetPasswordToken(ctx context.Context, tokenHash, passwordHash string, now time.Time) (*User, error)
	SaveReauthenticationToken(ctx context.Context, token LifecycleToken) error
	ConsumeReauthenticationToken(ctx context.Context, userID, tokenHash string, now time.Time) error
	ClaimEmailOutboxByID(ctx context.Context, id string, now time.Time) (EmailOutboxMessage, bool, error)
	ClaimEmailOutbox(ctx context.Context, limit int, now time.Time) ([]EmailOutboxMessage, error)
	MarkEmailOutboxDelivered(ctx context.Context, id string, deliveredAt time.Time) error
	MarkEmailOutboxFailed(ctx context.Context, id, failure string, retryAt time.Time) error
	// SoftDelete marks the user deleted. From that point on FindByEmail,
	// FindByID, and ListUsers must all treat the account as gone — which also
	// means any outstanding JWT stops working, since RequireAuthWithUser
	// re-checks FindByID on every request. Returns ErrUserNotFound if the user
	// doesn't exist (or is already deleted).
	SoftDelete(userID string) error
	DeleteAccount(ctx context.Context, userID string) error

	// SetRole assigns a moderation role (RoleUser/RoleModerator/RoleAdmin).
	SetRole(ctx context.Context, userID, role string) (*User, error)
	// Suspend applies a temporary suspension until the given time. A nil
	// until clears any existing suspension.
	Suspend(ctx context.Context, userID string, until *time.Time, reason string) (*User, error)
	// Ban permanently bans the account, preventing further authentication.
	Ban(ctx context.Context, userID string, reason string) (*User, error)
}

// InMemoryUserRepository is a goroutine-safe, process-local store used for the
// prototype milestone. It is indexed by both id and normalized email.
type InMemoryUserRepository struct {
	mu           sync.RWMutex
	byID         map[string]*User
	byEmail      map[string]*User
	identities   map[string]*AuthIdentity
	emailTokens  []memoryLifecycleToken
	resetTokens  []memoryLifecycleToken
	reauthTokens []memoryLifecycleToken
	emailOutbox  []EmailOutboxMessage
}

type memoryLifecycleToken struct {
	LifecycleToken
	ConsumedAt *time.Time
}

// NewInMemoryUserRepository returns an empty in-memory repository.
func NewInMemoryUserRepository() *InMemoryUserRepository {
	return &InMemoryUserRepository{
		byID:       make(map[string]*User),
		byEmail:    make(map[string]*User),
		identities: make(map[string]*AuthIdentity),
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
	if stored.AuthVersion == 0 {
		stored.AuthVersion = 1
	}
	if stored.PasswordHash != "" {
		stored.HasPassword = true
	}
	if stored.Role == "" {
		stored.Role = RoleUser
	}
	r.byID[stored.ID] = &stored
	r.byEmail[key] = &stored
	return nil
}

func (r *InMemoryUserRepository) CreateWithVerification(
	_ context.Context,
	user *User,
	token LifecycleToken,
	email EmailOutboxMessage,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := normalizeEmail(user.Email)
	if _, exists := r.byEmail[key]; exists {
		return ErrEmailExists
	}
	stored := *user
	if stored.AuthVersion == 0 {
		stored.AuthVersion = 1
	}
	if stored.PasswordHash != "" {
		stored.HasPassword = true
	}
	if stored.Role == "" {
		stored.Role = RoleUser
	}
	r.byID[stored.ID] = &stored
	r.byEmail[key] = &stored
	r.emailTokens = append(r.emailTokens, memoryLifecycleToken{LifecycleToken: token})
	r.emailOutbox = append(r.emailOutbox, email)
	return nil
}

func (r *InMemoryUserRepository) ClaimEmailOutbox(
	_ context.Context,
	limit int,
	now time.Time,
) ([]EmailOutboxMessage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if limit <= 0 {
		limit = 10
	}
	out := make([]EmailOutboxMessage, 0, limit)
	for i := range r.emailOutbox {
		message := &r.emailOutbox[i]
		leaseExpired := message.ClaimedAt != nil && message.ClaimedAt.Add(emailOutboxLeaseTTL).Before(now)
		if message.DeliveredAt != nil || message.AvailableAt.After(now) ||
			(message.ClaimedAt != nil && !leaseExpired) {
			continue
		}
		claimed := now
		message.ClaimedAt = &claimed
		message.Attempts++
		out = append(out, *message)
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func (r *InMemoryUserRepository) ClaimEmailOutboxByID(
	_ context.Context,
	id string,
	now time.Time,
) (EmailOutboxMessage, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.emailOutbox {
		message := &r.emailOutbox[i]
		if message.ID != id || message.DeliveredAt != nil || message.AvailableAt.After(now) {
			continue
		}
		leaseExpired := message.ClaimedAt != nil && message.ClaimedAt.Add(emailOutboxLeaseTTL).Before(now)
		if message.ClaimedAt != nil && !leaseExpired {
			return EmailOutboxMessage{}, false, nil
		}
		claimed := now
		message.ClaimedAt = &claimed
		message.Attempts++
		return *message, true, nil
	}
	return EmailOutboxMessage{}, false, nil
}

func (r *InMemoryUserRepository) MarkEmailOutboxDelivered(
	_ context.Context,
	id string,
	deliveredAt time.Time,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.emailOutbox {
		if r.emailOutbox[i].ID == id {
			r.emailOutbox[i].DeliveredAt = &deliveredAt
			r.emailOutbox[i].ClaimedAt = nil
			r.emailOutbox[i].LastError = ""
			r.emailOutbox[i].VerificationURL = ""
			return nil
		}
	}
	return ErrEmailOutboxNotFound
}

func (r *InMemoryUserRepository) MarkEmailOutboxFailed(
	_ context.Context,
	id, failure string,
	retryAt time.Time,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.emailOutbox {
		if r.emailOutbox[i].ID == id {
			r.emailOutbox[i].ClaimedAt = nil
			r.emailOutbox[i].AvailableAt = retryAt
			r.emailOutbox[i].LastError = failure
			return nil
		}
	}
	return ErrEmailOutboxNotFound
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

// UpdatePassword overwrites the stored password hash for userID.
func (r *InMemoryUserRepository) UpdatePassword(userID, passwordHash string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	u, ok := r.byID[userID]
	if !ok {
		return ErrUserNotFound
	}
	u.PasswordHash = passwordHash
	u.HasPassword = true
	return nil
}

func (r *InMemoryUserRepository) UpdatePasswordAndRevoke(userID, passwordHash string) (*User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byID[userID]
	if !ok {
		return nil, ErrUserNotFound
	}
	u.PasswordHash = passwordHash
	u.HasPassword = true
	u.AuthVersion++
	copied := *u
	return &copied, nil
}

func (r *InMemoryUserRepository) IncrementAuthVersion(userID string) (*User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byID[userID]
	if !ok {
		return nil, ErrUserNotFound
	}
	u.AuthVersion++
	copied := *u
	return &copied, nil
}

func (r *InMemoryUserRepository) SaveEmailVerificationToken(_ context.Context, token LifecycleToken) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	for i := range r.emailTokens {
		if r.emailTokens[i].UserID == token.UserID && r.emailTokens[i].ConsumedAt == nil {
			r.emailTokens[i].ConsumedAt = &now
		}
	}
	r.emailTokens = append(r.emailTokens, memoryLifecycleToken{LifecycleToken: token})
	return nil
}

func (r *InMemoryUserRepository) VerifyEmailToken(_ context.Context, tokenHash string, now time.Time) (*User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.emailTokens {
		t := &r.emailTokens[i]
		if subtle.ConstantTimeCompare([]byte(t.TokenHash), []byte(tokenHash)) != 1 {
			continue
		}
		if t.ConsumedAt != nil || !now.Before(t.ExpiresAt) {
			return nil, ErrInvalidLifecycleToken
		}
		t.ConsumedAt = &now
		u, ok := r.byID[t.UserID]
		if !ok {
			return nil, ErrInvalidLifecycleToken
		}
		verified := now.UTC()
		u.EmailVerifiedAt = &verified
		copied := *u
		return &copied, nil
	}
	return nil, ErrInvalidLifecycleToken
}

func (r *InMemoryUserRepository) SavePasswordResetToken(_ context.Context, token LifecycleToken) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	for i := range r.resetTokens {
		if r.resetTokens[i].UserID == token.UserID && r.resetTokens[i].ConsumedAt == nil {
			r.resetTokens[i].ConsumedAt = &now
		}
	}
	r.resetTokens = append(r.resetTokens, memoryLifecycleToken{LifecycleToken: token})
	return nil
}

func (r *InMemoryUserRepository) ResetPasswordToken(_ context.Context, tokenHash, passwordHash string, now time.Time) (*User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.resetTokens {
		t := &r.resetTokens[i]
		if subtle.ConstantTimeCompare([]byte(t.TokenHash), []byte(tokenHash)) != 1 {
			continue
		}
		if t.ConsumedAt != nil || !now.Before(t.ExpiresAt) {
			return nil, ErrInvalidLifecycleToken
		}
		t.ConsumedAt = &now
		u, ok := r.byID[t.UserID]
		if !ok || !u.HasPassword {
			return nil, ErrInvalidLifecycleToken
		}
		u.PasswordHash = passwordHash
		u.AuthVersion++
		for j := range r.resetTokens {
			if r.resetTokens[j].UserID == u.ID && r.resetTokens[j].ConsumedAt == nil {
				r.resetTokens[j].ConsumedAt = &now
			}
		}
		copied := *u
		return &copied, nil
	}
	return nil, ErrInvalidLifecycleToken
}

func (r *InMemoryUserRepository) SaveReauthenticationToken(_ context.Context, token LifecycleToken) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reauthTokens = append(r.reauthTokens, memoryLifecycleToken{LifecycleToken: token})
	return nil
}

func (r *InMemoryUserRepository) ConsumeReauthenticationToken(_ context.Context, userID, tokenHash string, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.reauthTokens {
		t := &r.reauthTokens[i]
		if t.UserID != userID || subtle.ConstantTimeCompare([]byte(t.TokenHash), []byte(tokenHash)) != 1 {
			continue
		}
		if t.ConsumedAt != nil || !now.Before(t.ExpiresAt) {
			return ErrReauthenticationRequired
		}
		t.ConsumedAt = &now
		return nil
	}
	return ErrReauthenticationRequired
}

// SoftDelete removes the user from both lookup indexes, so FindByEmail,
// FindByID, and ListUsers all treat the account as gone immediately.
func (r *InMemoryUserRepository) SoftDelete(userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	u, ok := r.byID[userID]
	if !ok {
		return ErrUserNotFound
	}
	u.AuthVersion++
	delete(r.byID, userID)
	delete(r.byEmail, normalizeEmail(u.Email))
	return nil
}

func (r *InMemoryUserRepository) DeleteAccount(_ context.Context, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byID[userID]
	if !ok {
		return ErrUserNotFound
	}
	delete(r.byID, userID)
	delete(r.byEmail, normalizeEmail(u.Email))
	for key, identity := range r.identities {
		if identity.UserID == userID {
			delete(r.identities, key)
		}
	}
	r.emailTokens = filterMemoryTokens(r.emailTokens, userID)
	r.resetTokens = filterMemoryTokens(r.resetTokens, userID)
	r.reauthTokens = filterMemoryTokens(r.reauthTokens, userID)
	return nil
}

func filterMemoryTokens(tokens []memoryLifecycleToken, userID string) []memoryLifecycleToken {
	out := tokens[:0]
	for _, token := range tokens {
		if token.UserID != userID {
			out = append(out, token)
		}
	}
	return out
}

// SetRole assigns a moderation role in place.
func (r *InMemoryUserRepository) SetRole(_ context.Context, userID, role string) (*User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byID[userID]
	if !ok {
		return nil, ErrUserNotFound
	}
	u.Role = role
	copied := *u
	return &copied, nil
}

// Suspend sets or clears a temporary suspension window.
func (r *InMemoryUserRepository) Suspend(_ context.Context, userID string, until *time.Time, reason string) (*User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byID[userID]
	if !ok {
		return nil, ErrUserNotFound
	}
	u.SuspendedUntil = until
	if until == nil {
		u.SuspensionReason = ""
	} else {
		u.SuspensionReason = reason
	}
	copied := *u
	return &copied, nil
}

// Ban permanently bans the account.
func (r *InMemoryUserRepository) Ban(_ context.Context, userID string, reason string) (*User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byID[userID]
	if !ok {
		return nil, ErrUserNotFound
	}
	now := time.Now().UTC()
	u.BannedAt = &now
	u.BanReason = reason
	copied := *u
	return &copied, nil
}

func (r *InMemoryUserRepository) FindIdentity(provider AuthProvider, subject string) (*AuthIdentity, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if identity, ok := r.identities[identityKey(provider, subject)]; ok {
		copied := *identity
		return &copied, nil
	}
	return nil, ErrIdentityNotFound
}

func (r *InMemoryUserRepository) CreateIdentity(identity *AuthIdentity) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := identityKey(identity.Provider, identity.ProviderSubject)
	if _, exists := r.identities[key]; exists {
		return ErrEmailExists
	}
	stored := *identity
	r.identities[key] = &stored
	return nil
}

func identityKey(provider AuthProvider, subject string) string {
	return string(provider) + ":" + strings.TrimSpace(subject)
}
