package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const minPasswordLength = 8

// Service holds the authentication business logic: validation, hashing,
// uniqueness checks, and token issuance. It depends only on the repository
// interface and the token manager.
type Service struct {
	repo           UserRepository
	tokens         *TokenManager
	googleEnabled  bool
	appleEnabled   bool
	googleVerifier ProviderVerifier
	appleVerifier  ProviderVerifier
	deletionHooks  []AccountDeletionHook
}

// AccountDeletionHook lets other domains react to account deletion, e.g.
// unpublishing a public profile so it stops appearing on Explore or a direct
// handle lookup once the account is gone. Hooks run best-effort: a hook
// failure is logged by the caller but never blocks the deletion itself, since
// the account row is already gone by the time hooks run.
type AccountDeletionHook interface {
	OnAccountDeleted(ctx context.Context, userID string) error
}

// RegisterDeletionHook attaches an additional best-effort side effect to run
// after DeleteAccount soft-deletes the auth row. May be called multiple times
// to wire more than one domain (profile takedown, etc.).
func (s *Service) RegisterDeletionHook(h AccountDeletionHook) {
	s.deletionHooks = append(s.deletionHooks, h)
}

// NewService wires a Service with its repository and token manager.
func NewService(repo UserRepository, tokens *TokenManager) *Service {
	return &Service{repo: repo, tokens: tokens}
}

type ProviderAuthConfig struct {
	GoogleEnabled  bool
	AppleEnabled   bool
	GoogleVerifier ProviderVerifier
	AppleVerifier  ProviderVerifier
}

func (s *Service) ConfigureProviderAuth(cfg ProviderAuthConfig) {
	s.googleEnabled = cfg.GoogleEnabled
	s.appleEnabled = cfg.AppleEnabled
	s.googleVerifier = cfg.GoogleVerifier
	s.appleVerifier = cfg.AppleVerifier
}

// Register validates the input, hashes the password, persists the user, and
// returns the created user together with a freshly minted JWT.
func (s *Service) Register(in RegisterInput) (*User, string, error) {
	email := normalizeEmail(in.Email)
	if email == "" {
		return nil, "", ErrEmailRequired
	}
	if in.Password == "" {
		return nil, "", ErrPasswordRequired
	}
	if len(in.Password) < minPasswordLength {
		return nil, "", ErrPasswordTooShort
	}
	if in.DisplayName == "" {
		return nil, "", ErrDisplayNameRequired
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, "", err
	}

	avatarKey := strings.TrimSpace(in.AvatarKey)
	if avatarKey == "" {
		avatarKey = "default"
	}

	user := &User{
		ID:           uuid.NewString(),
		Email:        email,
		DisplayName:  in.DisplayName,
		AvatarKey:    avatarKey,
		PasswordHash: string(hash),
	}
	if err := s.repo.Create(user); err != nil {
		return nil, "", err // ErrEmailExists bubbles up unchanged
	}

	token, err := s.tokens.Generate(user.ID, user.Email)
	if err != nil {
		return nil, "", err
	}
	return user, token, nil
}

// Login verifies credentials and returns the user plus a new JWT. Both an
// unknown email and a wrong password yield ErrInvalidCredentials so the API
// does not reveal which accounts exist.
func (s *Service) Login(email, password string) (*User, string, error) {
	user, err := s.repo.FindByEmail(email)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil, "", ErrInvalidCredentials
		}
		return nil, "", err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, "", ErrInvalidCredentials
	}

	token, err := s.tokens.Generate(user.ID, user.Email)
	if err != nil {
		return nil, "", err
	}
	return user, token, nil
}

// ChangePassword verifies currentPassword against the stored hash and, if it
// matches, replaces it with a hash of newPassword. Existing JWTs are
// unaffected (they carry no password state), so a compromised session is not
// automatically invalidated by a password change alone.
func (s *Service) ChangePassword(userID, currentPassword, newPassword string) error {
	user, err := s.repo.FindByID(userID)
	if err != nil {
		return err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(currentPassword)); err != nil {
		return ErrInvalidCredentials
	}
	if len(newPassword) < minPasswordLength {
		return ErrPasswordTooShort
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.repo.UpdatePassword(userID, string(hash))
}

// DeleteAccount verifies password (a destructive action requires
// re-confirming the credential, not just holding a live token) and then
// soft-deletes the account. From that instant, FindByEmail/FindByID/ListUsers
// all exclude the user, so RequireAuthWithUser rejects the caller's own token
// on its very next request — there is no separate revocation step. Registered
// deletion hooks then run best-effort so other domains (a public profile) stop
// surfacing the account too; a hook failure does not undo the deletion, since
// the safety property that matters (login and API access are gone) already
// holds by the time hooks run.
func (s *Service) DeleteAccount(ctx context.Context, userID, password string) error {
	user, err := s.repo.FindByID(userID)
	if err != nil {
		return err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return ErrInvalidCredentials
	}
	if err := s.repo.SoftDelete(userID); err != nil {
		return err
	}
	for _, hook := range s.deletionHooks {
		if err := hook.OnAccountDeleted(ctx, userID); err != nil {
			slog.Warn("account_deletion_hook_failed", "user_id", userID, "error", err)
		}
	}
	return nil
}

func (s *Service) LoginWithGoogle(ctx context.Context, credential string) (*User, string, error) {
	if !s.googleEnabled {
		return nil, "", ErrProviderDisabled
	}
	if s.googleVerifier == nil {
		return nil, "", ErrProviderNotConfigured
	}
	claims, err := s.googleVerifier.Verify(ctx, credential)
	if err != nil {
		return nil, "", err
	}
	claims.Provider = ProviderGoogle
	return s.loginWithProviderClaims(claims)
}

func (s *Service) LoginWithApple(ctx context.Context, identityToken string, fallback ProviderClaims) (*User, string, error) {
	if !s.appleEnabled {
		return nil, "", ErrProviderDisabled
	}
	if s.appleVerifier == nil {
		return nil, "", ErrProviderNotConfigured
	}
	claims, err := s.appleVerifier.Verify(ctx, identityToken)
	if err != nil {
		return nil, "", err
	}
	claims.Provider = ProviderApple
	if claims.Email == "" && fallback.Email != "" {
		claims.Email = fallback.Email
		claims.EmailVerified = true
	}
	if claims.DisplayName == "" {
		claims.DisplayName = fallback.DisplayName
	}
	return s.loginWithProviderClaims(claims)
}

func (s *Service) loginWithProviderClaims(claims ProviderClaims) (*User, string, error) {
	if claims.Subject == "" {
		return nil, "", ErrInvalidProviderToken
	}
	if identity, err := s.repo.FindIdentity(claims.Provider, claims.Subject); err == nil {
		user, err := s.repo.FindByID(identity.UserID)
		if err != nil {
			return nil, "", err
		}
		return s.issue(user)
	} else if !errors.Is(err, ErrIdentityNotFound) {
		return nil, "", err
	}

	email := normalizeEmail(claims.Email)
	if email == "" {
		return nil, "", ErrInvalidProviderToken
	}
	if !claims.EmailVerified {
		return nil, "", ErrProviderEmailUnverified
	}

	user, err := s.repo.FindByEmail(email)
	if errors.Is(err, ErrUserNotFound) {
		user, err = s.createProviderUser(email, claims.DisplayName, claims.AvatarKey)
	}
	if err != nil {
		return nil, "", err
	}
	identity := &AuthIdentity{
		ID: uuid.NewString(), UserID: user.ID, Provider: claims.Provider,
		ProviderSubject: claims.Subject, Email: email, EmailVerified: claims.EmailVerified,
	}
	if err := s.repo.CreateIdentity(identity); err != nil && !errors.Is(err, ErrEmailExists) {
		return nil, "", err
	}
	return s.issue(user)
}

func (s *Service) createProviderUser(email, displayName, avatarKey string) (*User, error) {
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		displayName = strings.Split(email, "@")[0]
	}
	if len([]rune(displayName)) < 2 {
		displayName = "Investor"
	}
	if avatarKey = strings.TrimSpace(avatarKey); avatarKey == "" {
		avatarKey = "default"
	}
	randomPassword := make([]byte, 32)
	if _, err := rand.Read(randomPassword); err != nil {
		return nil, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(base64.RawURLEncoding.EncodeToString(randomPassword)), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	user := &User{
		ID: uuid.NewString(), Email: email, DisplayName: displayName,
		AvatarKey: avatarKey, PasswordHash: string(hash),
	}
	if err := s.repo.Create(user); err != nil {
		return nil, err
	}
	return user, nil
}

func (s *Service) issue(user *User) (*User, string, error) {
	token, err := s.tokens.Generate(user.ID, user.Email)
	if err != nil {
		return nil, "", err
	}
	return user, token, nil
}

// UserByID fetches a user by id, used by the /me handler after auth.
func (s *Service) UserByID(id string) (*User, error) {
	return s.repo.FindByID(id)
}

// ListUsers returns all users. It is consumed by the leaderboard module via the
// UserProvider interface. Callers receive full User values (including the
// password hash) and are responsible for projecting to a safe response shape —
// the hash must never be serialized to clients.
func (s *Service) ListUsers(ctx context.Context) ([]User, error) {
	return s.repo.ListUsers(ctx)
}
