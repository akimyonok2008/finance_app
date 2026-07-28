package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func newTestService() *Service {
	repo := NewInMemoryUserRepository()
	tm := NewTokenManager("test-secret", time.Hour)
	svc := NewService(repo, tm)
	svc.ConfigureLifecycle(LifecycleConfig{EmailSender: autoVerifySender{repo: repo}})
	return svc
}

type autoVerifySender struct{ repo UserRepository }

func (s autoVerifySender) SendVerification(ctx context.Context, _ string, verificationURL string) error {
	token := verificationURL[strings.LastIndex(verificationURL, "=")+1:]
	_, err := s.repo.VerifyEmailToken(ctx, hashLifecycleToken(token), time.Now().UTC())
	return err
}

func (autoVerifySender) SendPasswordReset(context.Context, string, string) error { return nil }

func validInput() RegisterInput {
	return RegisterInput{
		Email:       "User@Example.com",
		Password:    "StrongPassword123",
		DisplayName: "AlphaWolf_91",
	}
}

func TestRegister_ValidUserCreatesUnverifiedUserWithoutToken(t *testing.T) {
	svc := newTestService()

	user, token, err := svc.Register(validInput())

	require.NoError(t, err)
	require.NotNil(t, user)
	assert.NotEmpty(t, user.ID)
	assert.Equal(t, "AlphaWolf_91", user.DisplayName)
	assert.Empty(t, token, "registration must not create an authenticated session")
}

func TestRegister_DuplicateEmailFails(t *testing.T) {
	svc := newTestService()
	_, _, err := svc.Register(validInput())
	require.NoError(t, err)

	// Same email, different casing — must still be detected as duplicate.
	dup := validInput()
	dup.Email = "user@example.com"
	_, _, err = svc.Register(dup)

	assert.ErrorIs(t, err, ErrEmailExists)
}

func TestRegister_WeakPasswordFails(t *testing.T) {
	svc := newTestService()
	in := validInput()
	in.Password = "short"

	_, _, err := svc.Register(in)

	assert.ErrorIs(t, err, ErrPasswordTooShort)
}

func TestRegister_MissingDisplayNameFails(t *testing.T) {
	svc := newTestService()
	in := validInput()
	in.DisplayName = ""

	_, _, err := svc.Register(in)

	assert.ErrorIs(t, err, ErrDisplayNameRequired)
}

func TestRegister_MissingEmailFails(t *testing.T) {
	svc := newTestService()
	in := validInput()
	in.Email = ""

	_, _, err := svc.Register(in)

	assert.ErrorIs(t, err, ErrEmailRequired)
}

func TestRegister_NormalizesEmailToLowercase(t *testing.T) {
	svc := newTestService()

	user, _, err := svc.Register(validInput())

	require.NoError(t, err)
	assert.Equal(t, "user@example.com", user.Email)
}

func TestRegister_PasswordStoredAsBcryptHash(t *testing.T) {
	repo := NewInMemoryUserRepository()
	svc := NewService(repo, NewTokenManager("test-secret", time.Hour))

	_, _, err := svc.Register(validInput())
	require.NoError(t, err)

	stored, err := repo.FindByEmail("user@example.com")
	require.NoError(t, err)

	// The stored hash must validate against the raw password via bcrypt.
	err = bcrypt.CompareHashAndPassword([]byte(stored.PasswordHash), []byte("StrongPassword123"))
	assert.NoError(t, err, "stored hash must be a valid bcrypt hash of the password")
}

func TestRegister_RawPasswordNeverStored(t *testing.T) {
	repo := NewInMemoryUserRepository()
	svc := NewService(repo, NewTokenManager("test-secret", time.Hour))

	_, _, err := svc.Register(validInput())
	require.NoError(t, err)

	stored, err := repo.FindByEmail("user@example.com")
	require.NoError(t, err)

	assert.NotEqual(t, "StrongPassword123", stored.PasswordHash)
	assert.NotContains(t, stored.PasswordHash, "StrongPassword123")
}

// TestRegister_SignupIPHashedNotStoredRaw: SignupIPHash exists purely to let
// a later investigation find accounts created from the same network (no
// multi-account/identity check otherwise exists), so it must never be
// recoverable to the raw IP, and must be deterministic for the same IP.
func TestRegister_SignupIPHashedNotStoredRaw(t *testing.T) {
	repo := NewInMemoryUserRepository()
	svc := NewService(repo, NewTokenManager("test-secret", time.Hour))
	svc.ConfigureSignupIPHashing("test-pepper")

	in := validInput()
	in.SignupIP = "203.0.113.7"
	_, _, err := svc.Register(in)
	require.NoError(t, err)

	stored, err := repo.FindByEmail("user@example.com")
	require.NoError(t, err)
	require.NotEmpty(t, stored.SignupIPHash)
	assert.NotContains(t, stored.SignupIPHash, "203.0.113.7", "the raw IP must never appear in the stored hash")

	// Same IP, different account, must hash identically so shared-network
	// accounts can be correlated.
	repo2 := NewInMemoryUserRepository()
	svc2 := NewService(repo2, NewTokenManager("test-secret", time.Hour))
	svc2.ConfigureSignupIPHashing("test-pepper")
	other := validInput()
	other.Email = "second@example.com"
	other.SignupIP = "203.0.113.7"
	_, _, err = svc2.Register(other)
	require.NoError(t, err)
	stored2, err := repo2.FindByEmail("second@example.com")
	require.NoError(t, err)
	assert.Equal(t, stored.SignupIPHash, stored2.SignupIPHash, "the same IP must hash identically across accounts")
}

// TestRegister_SignupIPHashingUnconfiguredLeavesHashBlank: hashing must be
// opt-in — a deployment that never calls ConfigureSignupIPHashing (or the
// test harness) must not silently store anything, and registration must not
// fail just because no IP was available (e.g. an internal/test caller).
func TestRegister_SignupIPHashingUnconfiguredLeavesHashBlank(t *testing.T) {
	repo := NewInMemoryUserRepository()
	svc := NewService(repo, NewTokenManager("test-secret", time.Hour))

	in := validInput()
	in.SignupIP = "203.0.113.7"
	_, _, err := svc.Register(in)
	require.NoError(t, err)

	stored, err := repo.FindByEmail("user@example.com")
	require.NoError(t, err)
	assert.Empty(t, stored.SignupIPHash)
}

func TestLogin_ValidCredentialsReturnsUserAndToken(t *testing.T) {
	svc := newTestService()
	_, _, err := svc.Register(validInput())
	require.NoError(t, err)

	user, token, err := svc.Login("user@example.com", "StrongPassword123")

	require.NoError(t, err)
	require.NotNil(t, user)
	assert.Equal(t, "user@example.com", user.Email)
	assert.NotEmpty(t, token)
}

func TestLogin_NormalizesEmailCasing(t *testing.T) {
	svc := newTestService()
	_, _, err := svc.Register(validInput())
	require.NoError(t, err)

	_, _, err = svc.Login("USER@EXAMPLE.COM", "StrongPassword123")

	assert.NoError(t, err)
}

func TestLogin_WrongPasswordFails(t *testing.T) {
	svc := newTestService()
	_, _, err := svc.Register(validInput())
	require.NoError(t, err)

	_, _, err = svc.Login("user@example.com", "WrongPassword")

	assert.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestLogin_UnknownEmailFails(t *testing.T) {
	svc := newTestService()

	_, _, err := svc.Login("nobody@example.com", "whatever123")

	assert.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestLogin_BannedAccountCannotReceiveToken(t *testing.T) {
	svc := newTestService()
	user, _, err := svc.Register(validInput())
	require.NoError(t, err)
	require.NoError(t, svc.Ban(context.Background(), user.ID, "permanent ban"))

	loggedIn, token, err := svc.Login("user@example.com", "StrongPassword123")

	assert.ErrorIs(t, err, ErrAccountBanned)
	assert.Nil(t, loggedIn)
	assert.Empty(t, token)
}

func TestRepository_FindByIDUnknownReturnsNotFound(t *testing.T) {
	repo := NewInMemoryUserRepository()

	_, err := repo.FindByID("does-not-exist")

	assert.True(t, errors.Is(err, ErrUserNotFound))
}

func TestProviderLogin_NewGoogleIdentityCreatesUser(t *testing.T) {
	svc := newTestService()
	svc.ConfigureProviderAuth(ProviderAuthConfig{
		GoogleEnabled: true,
		GoogleVerifier: fakeVerifier{claims: ProviderClaims{
			Provider: ProviderGoogle, Subject: "google-sub-1",
			Email: "New@Example.com", EmailVerified: true, DisplayName: "New Investor",
		}},
	})

	user, token, err := svc.LoginWithGoogle(context.Background(), "id-token")

	require.NoError(t, err)
	assert.Equal(t, "new@example.com", user.Email)
	assert.Equal(t, "New Investor", user.DisplayName)
	assert.NotEmpty(t, token)
}

func TestProviderLogin_ExistingIdentityLogsInUser(t *testing.T) {
	svc := newTestService()
	user, _, err := svc.Register(validInput())
	require.NoError(t, err)
	require.NoError(t, svc.repo.CreateIdentity(&AuthIdentity{
		ID: "identity-1", UserID: user.ID, Provider: ProviderGoogle,
		ProviderSubject: "google-sub-1", Email: user.Email, EmailVerified: true,
	}))
	svc.ConfigureProviderAuth(ProviderAuthConfig{
		GoogleEnabled: true,
		GoogleVerifier: fakeVerifier{claims: ProviderClaims{
			Provider: ProviderGoogle, Subject: "google-sub-1",
			Email: "other@example.com", EmailVerified: true, DisplayName: "Ignored",
		}},
	})

	loggedIn, token, err := svc.LoginWithGoogle(context.Background(), "id-token")

	require.NoError(t, err)
	assert.Equal(t, user.ID, loggedIn.ID)
	assert.Equal(t, user.Email, loggedIn.Email)
	assert.NotEmpty(t, token)
}

func TestProviderLogin_BannedExistingIdentityCannotReceiveToken(t *testing.T) {
	svc := newTestService()
	user, _, err := svc.Register(validInput())
	require.NoError(t, err)
	require.NoError(t, svc.repo.CreateIdentity(&AuthIdentity{
		ID: "identity-banned", UserID: user.ID, Provider: ProviderGoogle,
		ProviderSubject: "google-sub-banned", Email: user.Email, EmailVerified: true,
	}))
	require.NoError(t, svc.Ban(context.Background(), user.ID, "permanent ban"))
	svc.ConfigureProviderAuth(ProviderAuthConfig{
		GoogleEnabled: true,
		GoogleVerifier: fakeVerifier{claims: ProviderClaims{
			Provider: ProviderGoogle, Subject: "google-sub-banned",
			Email: user.Email, EmailVerified: true,
		}},
	})

	loggedIn, token, err := svc.LoginWithGoogle(context.Background(), "id-token")

	assert.ErrorIs(t, err, ErrAccountBanned)
	assert.Nil(t, loggedIn)
	assert.Empty(t, token)
}

func TestProviderLogin_VerifiedEmailLinksExistingUser(t *testing.T) {
	svc := newTestService()
	user, _, err := svc.Register(validInput())
	require.NoError(t, err)
	svc.ConfigureProviderAuth(ProviderAuthConfig{
		GoogleEnabled: true,
		GoogleVerifier: fakeVerifier{claims: ProviderClaims{
			Provider: ProviderGoogle, Subject: "google-sub-2",
			Email: "USER@example.com", EmailVerified: true, DisplayName: "Ignored",
		}},
	})

	loggedIn, _, err := svc.LoginWithGoogle(context.Background(), "id-token")
	require.NoError(t, err)
	assert.Equal(t, user.ID, loggedIn.ID)

	identity, err := svc.repo.FindIdentity(ProviderGoogle, "google-sub-2")
	require.NoError(t, err)
	assert.Equal(t, user.ID, identity.UserID)
}

func TestProviderLogin_UnverifiedEmailDoesNotLink(t *testing.T) {
	svc := newTestService()
	_, _, err := svc.Register(validInput())
	require.NoError(t, err)
	svc.ConfigureProviderAuth(ProviderAuthConfig{
		GoogleEnabled: true,
		GoogleVerifier: fakeVerifier{claims: ProviderClaims{
			Provider: ProviderGoogle, Subject: "google-sub-3",
			Email: "user@example.com", EmailVerified: false,
		}},
	})

	_, _, err = svc.LoginWithGoogle(context.Background(), "id-token")
	assert.ErrorIs(t, err, ErrProviderEmailUnverified)
}

func TestProviderLogin_DisabledProviderFailsSafely(t *testing.T) {
	svc := newTestService()

	_, _, err := svc.LoginWithGoogle(context.Background(), "id-token")

	assert.ErrorIs(t, err, ErrProviderDisabled)
}

func TestProviderLogin_GoogleCreatesUserWithoutAllowlist(t *testing.T) {
	svc := newTestService()
	svc.ConfigureProviderAuth(ProviderAuthConfig{
		GoogleEnabled: true,
		GoogleVerifier: fakeVerifier{claims: ProviderClaims{
			Provider: ProviderGoogle, Subject: "google-sub-dev",
			Email: "owner@example.com", EmailVerified: true,
		}},
	})

	user, _, err := svc.LoginWithGoogle(context.Background(), "id-token")

	require.NoError(t, err)
	assert.Equal(t, "owner@example.com", user.Email)
}

func TestProviderLogin_AppleUsesFallbackFirstAuthorizationEmail(t *testing.T) {
	svc := newTestService()
	svc.ConfigureProviderAuth(ProviderAuthConfig{
		AppleEnabled: true,
		AppleVerifier: fakeVerifier{claims: ProviderClaims{
			Provider: ProviderApple, Subject: "apple-sub-1", DisplayName: "Apple User",
		}},
	})

	user, _, err := svc.LoginWithApple(context.Background(), "identity-token", ProviderClaims{
		Email: "relay@privaterelay.appleid.com",
	})

	require.NoError(t, err)
	assert.Equal(t, "relay@privaterelay.appleid.com", user.Email)
}

type fakeVerifier struct {
	claims ProviderClaims
	err    error
}

func (v fakeVerifier) Verify(context.Context, string) (ProviderClaims, error) {
	if v.err != nil {
		return ProviderClaims{}, v.err
	}
	return v.claims, nil
}

func TestChangePassword_WrongCurrentPasswordFails(t *testing.T) {
	svc := newTestService()
	user, _, err := svc.Register(validInput())
	require.NoError(t, err)

	err = svc.ChangePassword(user.ID, "NotTheRealPassword", "NewStrongPassword123")

	assert.ErrorIs(t, err, ErrInvalidCredentials)
	_, _, loginErr := svc.Login("user@example.com", "StrongPassword123")
	assert.NoError(t, loginErr, "the original password must still work after a rejected change")
}

func TestChangePassword_WeakNewPasswordFails(t *testing.T) {
	svc := newTestService()
	user, _, err := svc.Register(validInput())
	require.NoError(t, err)

	err = svc.ChangePassword(user.ID, "StrongPassword123", "short")

	assert.ErrorIs(t, err, ErrPasswordTooShort)
}

func TestChangePassword_CorrectCurrentPasswordUpdatesHash(t *testing.T) {
	svc := newTestService()
	user, _, err := svc.Register(validInput())
	require.NoError(t, err)

	err = svc.ChangePassword(user.ID, "StrongPassword123", "NewStrongPassword123")
	require.NoError(t, err)

	_, _, err = svc.Login("user@example.com", "StrongPassword123")
	assert.ErrorIs(t, err, ErrInvalidCredentials, "the old password must stop working")

	_, _, err = svc.Login("user@example.com", "NewStrongPassword123")
	assert.NoError(t, err, "the new password must work")
}

func TestDeleteAccount_WrongPasswordFails(t *testing.T) {
	svc := newTestService()
	user, _, err := svc.Register(validInput())
	require.NoError(t, err)

	_, err = svc.ReauthenticatePassword(context.Background(), user.ID, "NotTheRealPassword")

	assert.ErrorIs(t, err, ErrInvalidCredentials)
	_, err = svc.UserByID(user.ID)
	assert.NoError(t, err, "a rejected deletion must not remove the account")
}

// TestDeleteAccount_RemovesAccountFromEveryLookup is the property that matters
// most: once deleted, the account must disappear from login, from
// RequireAuthWithUser's existence check (via UserByID/FindByID), and from
// ListUsers (leaderboard/achievement enumeration) — all at once, with no
// separate revocation step.
func TestDeleteAccount_RemovesAccountFromEveryLookup(t *testing.T) {
	svc := newTestService()
	user, _, err := svc.Register(validInput())
	require.NoError(t, err)

	reauth, err := svc.ReauthenticatePassword(context.Background(), user.ID, "StrongPassword123")
	require.NoError(t, err)
	err = svc.DeleteAccount(context.Background(), user.ID, reauth)
	require.NoError(t, err)

	_, _, loginErr := svc.Login("user@example.com", "StrongPassword123")
	assert.ErrorIs(t, loginErr, ErrInvalidCredentials, "login must fail for a deleted account")

	_, err = svc.UserByID(user.ID)
	assert.ErrorIs(t, err, ErrUserNotFound, "UserByID (used by RequireAuthWithUser) must reject a deleted account")

	users, err := svc.ListUsers(context.Background())
	require.NoError(t, err)
	for _, u := range users {
		assert.NotEqual(t, user.ID, u.ID, "a deleted account must not appear in ListUsers")
	}
}

type recordingDeletionHook struct {
	called []string
	err    error
}

func (h *recordingDeletionHook) OnAccountDeleted(_ context.Context, userID string) error {
	h.called = append(h.called, userID)
	return h.err
}

func TestDeleteAccount_RunsRegisteredHooks(t *testing.T) {
	svc := newTestService()
	user, _, err := svc.Register(validInput())
	require.NoError(t, err)
	hook := &recordingDeletionHook{}
	svc.RegisterDeletionHook(hook)

	reauth, err := svc.ReauthenticatePassword(context.Background(), user.ID, "StrongPassword123")
	require.NoError(t, err)
	err = svc.DeleteAccount(context.Background(), user.ID, reauth)

	require.NoError(t, err)
	assert.Equal(t, []string{user.ID}, hook.called)
}

func TestDeleteAccount_HookFailureBlocksDeletion(t *testing.T) {
	svc := newTestService()
	user, _, err := svc.Register(validInput())
	require.NoError(t, err)
	svc.RegisterDeletionHook(&recordingDeletionHook{err: errors.New("downstream unavailable")})

	reauth, err := svc.ReauthenticatePassword(context.Background(), user.ID, "StrongPassword123")
	require.NoError(t, err)
	err = svc.DeleteAccount(context.Background(), user.ID, reauth)

	require.Error(t, err)
	_, err = svc.UserByID(user.ID)
	assert.NoError(t, err)
}
