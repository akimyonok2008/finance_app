package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const minPasswordLength = 8

// maxAvatarKeyLength mirrors the profiles table's avatar_key length CHECK
// (migration 0002) — profiles enforce it, but the users table (migration
// 0001) never has, so a registration/OAuth-provisioned avatar_key could
// otherwise be stored and rendered (as raw text, not an image — there is no
// avatar upload/image system) unbounded.
const maxAvatarKeyLength = 40

func truncateRunes(value string, max int) string {
	runes := []rune(value)
	if len(runes) > max {
		runes = runes[:max]
	}
	return string(runes)
}

// Service holds the authentication business logic: validation, hashing,
// uniqueness checks, and token issuance. It depends only on the repository
// interface and the token manager.
type Service struct {
	repo            UserRepository
	tokens          *TokenManager
	googleEnabled   bool
	appleEnabled    bool
	googleVerifier  ProviderVerifier
	appleVerifier   ProviderVerifier
	deletionHooks   []AccountDeletionHook
	creationHooks   []AccountCreationHook
	txCreationHooks []TxInitHook
	emailSender     EmailSender
	publicAppURL    string
	verificationTTL time.Duration
	resetTTL        time.Duration
	reauthTTL       time.Duration
	signupIPPepper  string
}

// AccountDeletionHook lets non-database stores erase user-owned data before
// the authoritative auth row is deleted. A failure stops the operation so the
// API never reports successful deletion while a required erasure is incomplete.
type AccountDeletionHook interface {
	OnAccountDeleted(ctx context.Context, userID string) error
}

// AccountCreationHook initializes required user-owned aggregates immediately
// after account persistence, before any read endpoint can be reached.
type AccountCreationHook interface {
	OnAccountCreated(ctx context.Context, userID string) error
}

func (s *Service) RegisterCreationHook(h AccountCreationHook) {
	s.creationHooks = append(s.creationHooks, h)
}

// RegisterTxCreationHook attaches a required initialization step that runs
// inside the same database transaction as user creation (see
// PostgresUserRepository.CreateWithVerification). A failure rolls back the
// user row too, so registration can never leave behind a user with a
// permanently missing required aggregate.
func (s *Service) RegisterTxCreationHook(h TxInitHook) {
	s.txCreationHooks = append(s.txCreationHooks, h)
}

func (s *Service) initializeAccount(ctx context.Context, userID string) error {
	for _, hook := range s.creationHooks {
		if err := hook.OnAccountCreated(ctx, userID); err != nil {
			return err
		}
	}
	return nil
}

// RegisterDeletionHook attaches a required erasure step. May be called
// multiple times to cover every non-database user-data store.
func (s *Service) RegisterDeletionHook(h AccountDeletionHook) {
	s.deletionHooks = append(s.deletionHooks, h)
}

// NewService wires a Service with its repository and token manager.
func NewService(repo UserRepository, tokens *TokenManager) *Service {
	return &Service{
		repo: repo, tokens: tokens,
		emailSender:     DevelopmentEmailSender{},
		publicAppURL:    "http://localhost:5173",
		verificationTTL: 24 * time.Hour,
		resetTTL:        time.Hour,
		reauthTTL:       5 * time.Minute,
	}
}

type LifecycleConfig struct {
	EmailSender     EmailSender
	PublicAppURL    string
	VerificationTTL time.Duration
	ResetTTL        time.Duration
	ReauthTTL       time.Duration
}

func (s *Service) ConfigureLifecycle(cfg LifecycleConfig) {
	if cfg.EmailSender != nil {
		s.emailSender = cfg.EmailSender
	}
	if strings.TrimSpace(cfg.PublicAppURL) != "" {
		s.publicAppURL = strings.TrimRight(cfg.PublicAppURL, "/")
	}
	if cfg.VerificationTTL > 0 {
		s.verificationTTL = cfg.VerificationTTL
	}
	if cfg.ResetTTL > 0 {
		s.resetTTL = cfg.ResetTTL
	}
	if cfg.ReauthTTL > 0 {
		s.reauthTTL = cfg.ReauthTTL
	}
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

// ConfigureSignupIPHashing sets the HMAC key used to derive SignupIPHash from
// a registering client's IP. It never stores the raw IP — only a keyed hash,
// so the value is useless for anything except comparing whether two accounts
// were created from the same address. Passing an empty pepper disables the
// hash (SignupIPHash is left blank on every new registration).
func (s *Service) ConfigureSignupIPHashing(pepper string) {
	s.signupIPPepper = pepper
}

// hashSignupIP derives SignupIPHash for a registering client. Returns "" if
// hashing is unconfigured or no IP was supplied, so this never blocks
// registration or requires a schema backfill for existing rows.
func (s *Service) hashSignupIP(ip string) string {
	if s.signupIPPepper == "" || ip == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(s.signupIPPepper))
	mac.Write([]byte(ip))
	return hex.EncodeToString(mac.Sum(nil))
}

// Register validates the input, persists an unverified password account, and
// sends a single-use verification link. It intentionally returns no JWT.
func (s *Service) Register(in RegisterInput) (*User, string, error) {
	return s.RegisterContext(context.Background(), in)
}

func (s *Service) RegisterContext(ctx context.Context, in RegisterInput) (*User, string, error) {
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

	avatarKey := truncateRunes(strings.TrimSpace(in.AvatarKey), maxAvatarKeyLength)
	if avatarKey == "" {
		avatarKey = "default"
	}

	user := &User{
		ID:           uuid.NewString(),
		Email:        email,
		DisplayName:  in.DisplayName,
		AvatarKey:    avatarKey,
		PasswordHash: string(hash),
		HasPassword:  true,
		AuthVersion:  1,
		Role:         RoleUser,
		SignupIPHash: s.hashSignupIP(in.SignupIP),
	}
	rawToken, record, err := newLifecycleToken(user.ID, s.verificationTTL)
	if err != nil {
		return nil, "", err
	}
	now := time.Now().UTC()
	message := EmailOutboxMessage{
		ID: uuid.NewString(), UserID: user.ID, Kind: EmailKindVerification,
		Recipient:       user.Email,
		VerificationURL: s.publicAppURL + "/verify-email?token=" + rawToken,
		CreatedAt:       now, AvailableAt: now,
	}
	if err := s.repo.CreateWithVerification(ctx, user, record, message, s.txCreationHooks...); err != nil {
		return nil, "", err
	}
	if err := s.initializeAccount(ctx, user.ID); err != nil {
		return nil, "", err
	}
	// Low-latency best effort after the atomic commit. Failure is deliberately
	// not returned to the client: the durable outbox retains the message and
	// the mandatory processor retries it, so registration cannot be stranded
	// or misleadingly invite a duplicate registration attempt.
	_ = s.deliverEmailOutboxByID(ctx, message.ID)
	return user, "", nil
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
	if !user.HasPassword {
		return nil, "", ErrInvalidCredentials
	}
	if user.EmailVerifiedAt == nil {
		return nil, "", ErrEmailVerificationRequired
	}

	return s.issue(user)
}

// ChangePassword verifies currentPassword, replaces the password hash, and
// increments auth_version so all existing JWTs are invalidated.
func (s *Service) ChangePassword(userID, currentPassword, newPassword string) error {
	_, err := s.ChangePasswordAndRevoke(userID, currentPassword, newPassword)
	return err
}

func (s *Service) ChangePasswordAndRevoke(userID, currentPassword, newPassword string) (string, error) {
	user, err := s.repo.FindByID(userID)
	if err != nil {
		return "", err
	}
	if !user.HasPassword {
		return "", ErrReauthenticationRequired
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(currentPassword)); err != nil {
		return "", ErrInvalidCredentials
	}
	if len(newPassword) < minPasswordLength {
		return "", ErrPasswordTooShort
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	updated, err := s.repo.UpdatePasswordAndRevoke(userID, string(hash))
	if err != nil {
		return "", err
	}
	return s.tokens.Generate(updated.ID, updated.Email, updated.AuthVersion)
}

// DeleteAccount consumes a short-lived, single-use reauthentication token,
// completes all required external-store erasures, and then hard-deletes the
// auth row and database-owned user data transactionally.
func (s *Service) DeleteAccount(ctx context.Context, userID, reauthToken string) error {
	if err := s.consumeReauthentication(ctx, userID, reauthToken); err != nil {
		return err
	}
	for _, hook := range s.deletionHooks {
		if err := hook.OnAccountDeleted(ctx, userID); err != nil {
			return err
		}
	}
	return s.repo.DeleteAccount(ctx, userID)
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
	return s.loginWithProviderClaims(ctx, claims)
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
	return s.loginWithProviderClaims(ctx, claims)
}

func (s *Service) loginWithProviderClaims(ctx context.Context, claims ProviderClaims) (*User, string, error) {
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
		return s.registerProviderUser(ctx, email, claims)
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

// registerProviderUser atomically persists a brand-new OAuth account, its
// provider identity, and required aggregate initialization (see
// PostgresUserRepository.CreateProviderUser). A failure rolls back the user
// row too, so OAuth registration can never leave behind a user with a
// permanently missing required aggregate, matching the guarantee
// CreateWithVerification gives password registration.
func (s *Service) registerProviderUser(ctx context.Context, email string, claims ProviderClaims) (*User, string, error) {
	displayName := strings.TrimSpace(claims.DisplayName)
	if displayName == "" {
		displayName = strings.Split(email, "@")[0]
	}
	if len([]rune(displayName)) < 2 {
		displayName = "Investor"
	}
	avatarKey := truncateRunes(strings.TrimSpace(claims.AvatarKey), maxAvatarKeyLength)
	if avatarKey == "" {
		avatarKey = "default"
	}
	now := time.Now().UTC()
	user := &User{
		ID: uuid.NewString(), Email: email, DisplayName: displayName,
		AvatarKey: avatarKey, HasPassword: false, EmailVerifiedAt: &now,
		AuthVersion: 1, Role: RoleUser,
	}
	identity := &AuthIdentity{
		ID: uuid.NewString(), UserID: user.ID, Provider: claims.Provider,
		ProviderSubject: claims.Subject, Email: email, EmailVerified: claims.EmailVerified,
	}
	if err := s.repo.CreateProviderUser(ctx, user, identity, s.txCreationHooks...); err != nil {
		return nil, "", err
	}
	return s.issue(user)
}

func (s *Service) issue(user *User) (*User, string, error) {
	// Token issuance is the common final boundary for password, Google, Apple,
	// and email-verification authentication. Keep the ban check here so no
	// authentication path can accidentally mint a JWT for a banned account.
	if user.IsBanned() {
		return nil, "", ErrAccountBanned
	}
	token, err := s.tokens.Generate(user.ID, user.Email, user.AuthVersion)
	if err != nil {
		return nil, "", err
	}
	return user, token, nil
}

func newLifecycleToken(userID string, ttl time.Duration) (string, LifecycleToken, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", LifecycleToken{}, err
	}
	value := base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(value))
	now := time.Now().UTC()
	return value, LifecycleToken{
		ID: uuid.NewString(), UserID: userID, TokenHash: hex.EncodeToString(sum[:]),
		CreatedAt: now, ExpiresAt: now.Add(ttl),
	}, nil
}

func hashLifecycleToken(raw string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(raw)))
	return hex.EncodeToString(sum[:])
}

func (s *Service) VerifyEmail(ctx context.Context, rawToken string) (*User, string, error) {
	user, err := s.repo.VerifyEmailToken(ctx, hashLifecycleToken(rawToken), time.Now().UTC())
	if err != nil {
		return nil, "", err
	}
	return s.issue(user)
}

// ResendVerification intentionally returns no account-specific result. Unknown,
// verified, and provider-only addresses all follow the same external response.
func (s *Service) ResendVerification(ctx context.Context, email string) {
	user, err := s.repo.FindByEmail(email)
	if err != nil || !user.HasPassword || user.EmailVerifiedAt != nil {
		return
	}
	raw, record, err := newLifecycleToken(user.ID, s.verificationTTL)
	if err != nil {
		return
	}
	if s.repo.SaveEmailVerificationToken(ctx, record) != nil {
		return
	}
	_ = s.emailSender.SendVerification(ctx, user.Email, s.publicAppURL+"/verify-email?token="+raw)
}

// ForgotPassword is deliberately enumeration-safe: it performs work only for
// eligible accounts but exposes no existence or delivery result to the caller.
func (s *Service) ForgotPassword(ctx context.Context, email string) {
	user, err := s.repo.FindByEmail(email)
	if err != nil || !user.HasPassword {
		return
	}
	raw, record, err := newLifecycleToken(user.ID, s.resetTTL)
	if err != nil {
		return
	}
	if s.repo.SavePasswordResetToken(ctx, record) != nil {
		return
	}
	_ = s.emailSender.SendPasswordReset(ctx, user.Email, s.publicAppURL+"/reset-password?token="+raw)
}

func (s *Service) ResetPassword(ctx context.Context, rawToken, newPassword string) error {
	if len(newPassword) < minPasswordLength {
		return ErrPasswordTooShort
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.repo.ResetPasswordToken(ctx, hashLifecycleToken(rawToken), string(hash), time.Now().UTC())
	return err
}

func (s *Service) RevokeSessions(userID string) error {
	_, err := s.repo.IncrementAuthVersion(userID)
	return err
}

func (s *Service) ReauthenticatePassword(ctx context.Context, userID, password string) (string, error) {
	user, err := s.repo.FindByID(userID)
	if err != nil {
		return "", err
	}
	if !user.HasPassword || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return "", ErrInvalidCredentials
	}
	return s.createReauthentication(ctx, userID)
}

func (s *Service) ReauthenticateProvider(
	ctx context.Context,
	userID string,
	provider AuthProvider,
	credential string,
) (string, error) {
	var verifier ProviderVerifier
	switch provider {
	case ProviderGoogle:
		if !s.googleEnabled {
			return "", ErrProviderDisabled
		}
		verifier = s.googleVerifier
	case ProviderApple:
		if !s.appleEnabled {
			return "", ErrProviderDisabled
		}
		verifier = s.appleVerifier
	default:
		return "", ErrInvalidProviderToken
	}
	if verifier == nil {
		return "", ErrProviderNotConfigured
	}
	claims, err := verifier.Verify(ctx, credential)
	if err != nil || strings.TrimSpace(claims.Subject) == "" {
		return "", ErrInvalidProviderToken
	}
	identity, err := s.repo.FindIdentity(provider, claims.Subject)
	if err != nil || identity.UserID != userID {
		return "", ErrInvalidProviderToken
	}
	return s.createReauthentication(ctx, userID)
}

func (s *Service) createReauthentication(ctx context.Context, userID string) (string, error) {
	raw, record, err := newLifecycleToken(userID, s.reauthTTL)
	if err != nil {
		return "", err
	}
	if err := s.repo.SaveReauthenticationToken(ctx, record); err != nil {
		return "", err
	}
	return raw, nil
}

func (s *Service) consumeReauthentication(ctx context.Context, userID, rawToken string) error {
	if strings.TrimSpace(rawToken) == "" {
		return ErrReauthenticationRequired
	}
	return s.repo.ConsumeReauthenticationToken(ctx, userID, hashLifecycleToken(rawToken), time.Now().UTC())
}

func (s *Service) SetFirstPassword(ctx context.Context, userID, rawToken, newPassword string) (string, error) {
	user, err := s.repo.FindByID(userID)
	if err != nil {
		return "", err
	}
	if user.HasPassword {
		return "", ErrPasswordAlreadySet
	}
	if len(newPassword) < minPasswordLength {
		return "", ErrPasswordTooShort
	}
	if err := s.consumeReauthentication(ctx, userID, rawToken); err != nil {
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	updated, err := s.repo.UpdatePasswordAndRevoke(userID, string(hash))
	if err != nil {
		return "", err
	}
	return s.tokens.Generate(updated.ID, updated.Email, updated.AuthVersion)
}

// EnsureBootstrapAdmin promotes the account with the given email to
// RoleAdmin if it exists and isn't already an admin. It is a no-op when
// email is empty or the account doesn't exist yet (e.g. it will register
// later) — a safe, idempotent bootstrap mechanism that never runs through a
// public API.
func (s *Service) EnsureBootstrapAdmin(ctx context.Context, email string) error {
	email = strings.TrimSpace(email)
	if email == "" {
		return nil
	}
	user, err := s.repo.FindByEmail(email)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil
		}
		return err
	}
	if user.Role == RoleAdmin {
		return nil
	}
	_, err = s.repo.SetRole(ctx, user.ID, RoleAdmin)
	return err
}

// UserByID fetches a user by id, used by the /me handler after auth.
func (s *Service) UserByID(id string) (*User, error) {
	return s.repo.FindByID(id)
}

// SetRole assigns a moderation role. Called only from the moderation admin
// bootstrap path and RequireModerator-gated endpoints — never a public API.
func (s *Service) SetRole(ctx context.Context, userID, role string) error {
	_, err := s.repo.SetRole(ctx, userID, role)
	return err
}

// Suspend applies (or, with a nil until, clears) a temporary suspension.
func (s *Service) Suspend(ctx context.Context, userID string, until *time.Time, reason string) error {
	_, err := s.repo.Suspend(ctx, userID, until, reason)
	return err
}

// Ban permanently bans the account.
func (s *Service) Ban(ctx context.Context, userID string, reason string) error {
	_, err := s.repo.Ban(ctx, userID, reason)
	return err
}

// ListUsers returns all users, including sensitive internal fields like the
// password hash. It exists for callers that genuinely need full account state
// (e.g. batch worker adapters that only read IDs but sit inside the auth
// module's own trust boundary). Modules outside that boundary — like the
// leaderboard — must use ListRankableUsers instead.
func (s *Service) ListUsers(ctx context.Context) ([]User, error) {
	return s.repo.ListUsers(ctx)
}

// ListRankableUsers returns the narrow, safe-to-share projection of every
// user for ranking display. It is the only user enumeration the leaderboard
// module (or any other module outside auth) should depend on — the password
// hash and other sensitive fields never leave this boundary.
func (s *Service) ListRankableUsers(ctx context.Context) ([]RankableUser, error) {
	users, err := s.repo.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]RankableUser, 0, len(users))
	for _, u := range users {
		out = append(out, RankableUser{ID: u.ID, DisplayName: u.DisplayName, AvatarKey: u.AvatarKey})
	}
	return out, nil
}

// pagedUserLister is the optional repository capability behind
// ListRankableUsersPage: keyset-paginated enumeration ordered by user ID.
// Implemented by PostgresUserRepository; the in-memory repository doesn't
// need it (the service falls back to sorting the full list, which is fine at
// in-memory scale).
type pagedUserLister interface {
	ListUsersPage(ctx context.Context, afterID string, limit int) ([]User, error)
}

// ListRankableUsersPage is the keyset-paginated counterpart of
// ListRankableUsers: up to limit users with ID > afterID, ordered by ID. It
// lets population-scale background workers (the leaderboard refresh) select
// one bounded batch per tick without loading every user into memory. Same
// trust boundary as ListRankableUsers: only the narrow rankable projection
// ever leaves the auth module.
func (s *Service) ListRankableUsersPage(ctx context.Context, afterID string, limit int) ([]RankableUser, error) {
	if limit <= 0 {
		return nil, nil
	}
	if paged, ok := s.repo.(pagedUserLister); ok {
		users, err := paged.ListUsersPage(ctx, afterID, limit)
		if err != nil {
			return nil, err
		}
		out := make([]RankableUser, 0, len(users))
		for _, u := range users {
			out = append(out, RankableUser{ID: u.ID, DisplayName: u.DisplayName, AvatarKey: u.AvatarKey})
		}
		return out, nil
	}
	all, err := s.ListRankableUsers(ctx)
	if err != nil {
		return nil, err
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	out := make([]RankableUser, 0, limit)
	for _, u := range all {
		if u.ID <= afterID {
			continue
		}
		out = append(out, u)
		if len(out) == limit {
			break
		}
	}
	return out, nil
}
