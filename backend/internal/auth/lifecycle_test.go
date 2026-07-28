package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type captureEmailSender struct {
	verificationURL string
	resetURL        string
}

type failingVerificationSender struct{ err error }

func (s failingVerificationSender) SendVerification(context.Context, string, string) error {
	return s.err
}

func (failingVerificationSender) SendPasswordReset(context.Context, string, string) error {
	return nil
}

func (s *captureEmailSender) SendVerification(_ context.Context, _ string, value string) error {
	s.verificationURL = value
	return nil
}

func (s *captureEmailSender) SendPasswordReset(_ context.Context, _ string, value string) error {
	s.resetURL = value
	return nil
}

func tokenFromURL(value string) string {
	if index := strings.LastIndex(value, "="); index >= 0 {
		return value[index+1:]
	}
	return ""
}

func lifecycleHarness() (*Service, *InMemoryUserRepository, *captureEmailSender, *TokenManager) {
	repo := NewInMemoryUserRepository()
	tokens := NewTokenManager("lifecycle-test-secret", time.Hour)
	sender := &captureEmailSender{}
	svc := NewService(repo, tokens)
	svc.ConfigureLifecycle(LifecycleConfig{EmailSender: sender})
	return svc, repo, sender, tokens
}

func registerAndVerifyService(t *testing.T, svc *Service, sender *captureEmailSender) (*User, string) {
	t.Helper()
	user, registrationToken, err := svc.Register(validInput())
	require.NoError(t, err)
	require.Empty(t, registrationToken)
	verified, jwt, err := svc.VerifyEmail(context.Background(), tokenFromURL(sender.verificationURL))
	require.NoError(t, err)
	require.Equal(t, user.ID, verified.ID)
	return verified, jwt
}

func TestLifecycle_UnverifiedPasswordUserCannotLogin(t *testing.T) {
	svc, _, _, _ := lifecycleHarness()
	_, _, err := svc.Register(validInput())
	require.NoError(t, err)

	_, _, err = svc.Login("user@example.com", "StrongPassword123")
	assert.ErrorIs(t, err, ErrEmailVerificationRequired)
}

func TestLifecycle_RegistrationEmailFailureRemainsRetryable(t *testing.T) {
	repo := NewInMemoryUserRepository()
	svc := NewService(repo, NewTokenManager("lifecycle-test-secret", time.Hour))
	svc.ConfigureLifecycle(LifecycleConfig{
		EmailSender: failingVerificationSender{err: errors.New("smtp unavailable")},
	})

	user, token, err := svc.Register(validInput())
	require.NoError(t, err, "durably queued delivery must make registration successful")
	assert.Empty(t, token)
	require.NotNil(t, user)

	stored, err := repo.FindByEmail(user.Email)
	require.NoError(t, err)
	assert.Equal(t, user.ID, stored.ID)

	repo.mu.Lock()
	require.Len(t, repo.emailOutbox, 1)
	assert.Nil(t, repo.emailOutbox[0].DeliveredAt)
	assert.Contains(t, repo.emailOutbox[0].LastError, "smtp unavailable")
	repo.emailOutbox[0].AvailableAt = time.Now().Add(-time.Second)
	repo.mu.Unlock()

	capture := &captureEmailSender{}
	svc.ConfigureLifecycle(LifecycleConfig{EmailSender: capture})
	delivered, err := svc.ProcessEmailOutboxOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, delivered)
	assert.NotEmpty(t, capture.verificationURL)

	verified, jwt, err := svc.VerifyEmail(
		context.Background(), tokenFromURL(capture.verificationURL),
	)
	require.NoError(t, err)
	assert.Equal(t, user.ID, verified.ID)
	assert.NotEmpty(t, jwt)
}

func TestLifecycle_VerificationTokenSucceedsOnce(t *testing.T) {
	svc, _, sender, _ := lifecycleHarness()
	_, _, err := svc.Register(validInput())
	require.NoError(t, err)
	raw := tokenFromURL(sender.verificationURL)

	_, jwt, err := svc.VerifyEmail(context.Background(), raw)
	require.NoError(t, err)
	assert.NotEmpty(t, jwt)

	_, _, err = svc.VerifyEmail(context.Background(), raw)
	assert.ErrorIs(t, err, ErrInvalidLifecycleToken)
}

func TestLifecycle_ExpiredVerificationTokenFails(t *testing.T) {
	svc, repo, _, _ := lifecycleHarness()
	user, _, err := svc.Register(validInput())
	require.NoError(t, err)
	raw := "expired-verification-token"
	require.NoError(t, repo.SaveEmailVerificationToken(context.Background(), LifecycleToken{
		ID: "expired", UserID: user.ID, TokenHash: hashLifecycleToken(raw),
		CreatedAt: time.Now().Add(-2 * time.Hour), ExpiresAt: time.Now().Add(-time.Hour),
	}))

	_, _, err = svc.VerifyEmail(context.Background(), raw)
	assert.ErrorIs(t, err, ErrInvalidLifecycleToken)
}

func TestLifecycle_ResetTokenSucceedsOnceAndRevokesPreviousJWT(t *testing.T) {
	svc, _, sender, tokens := lifecycleHarness()
	user, oldJWT := registerAndVerifyService(t, svc, sender)
	svc.ForgotPassword(context.Background(), user.Email)
	raw := tokenFromURL(sender.resetURL)

	require.NoError(t, svc.ResetPassword(context.Background(), raw, "ReplacementPassword123"))
	assert.ErrorIs(t, svc.ResetPassword(context.Background(), raw, "AnotherPassword123"), ErrInvalidLifecycleToken)

	protected := RequireAuthWithUser(tokens, svc)(http.HandlerFunc(echoUserHandler))
	request := func(jwt string) int {
		req, _ := http.NewRequest(http.MethodGet, "/me", nil)
		req.Header.Set("Authorization", "Bearer "+jwt)
		rec := &statusRecorder{header: http.Header{}}
		protected.ServeHTTP(rec, req)
		return rec.status
	}
	assert.Equal(t, http.StatusUnauthorized, request(oldJWT))
	_, newJWT, err := svc.Login(user.Email, "ReplacementPassword123")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, request(newJWT))
}

type statusRecorder struct {
	header http.Header
	status int
}

func (r *statusRecorder) Header() http.Header       { return r.header }
func (r *statusRecorder) Write([]byte) (int, error) { return 0, nil }
func (r *statusRecorder) WriteHeader(status int)    { r.status = status }

func TestLifecycle_PasswordChangeReturnsReplacementAndRevokesOldJWT(t *testing.T) {
	svc, _, sender, tokens := lifecycleHarness()
	user, oldJWT := registerAndVerifyService(t, svc, sender)

	replacement, err := svc.ChangePasswordAndRevoke(user.ID, "StrongPassword123", "NewStrongPassword123")
	require.NoError(t, err)
	assert.NotEmpty(t, replacement)

	oldClaims, err := tokens.Parse(oldJWT)
	require.NoError(t, err)
	newClaims, err := tokens.Parse(replacement)
	require.NoError(t, err)
	assert.Equal(t, oldClaims.AuthVersion+1, newClaims.AuthVersion)
}

func TestLifecycle_ProviderOnlyReauthenticationRequiresMatchingSubject(t *testing.T) {
	svc, _, _, _ := lifecycleHarness()
	svc.ConfigureProviderAuth(ProviderAuthConfig{
		GoogleEnabled: true,
		GoogleVerifier: fakeVerifier{claims: ProviderClaims{
			Subject: "subject-one", Email: "provider@example.com",
			EmailVerified: true,
		}},
	})
	user, _, err := svc.LoginWithGoogle(context.Background(), "credential")
	require.NoError(t, err)
	assert.False(t, user.HasPassword)
	_, err = svc.ReauthenticatePassword(context.Background(), user.ID, "unknown")
	assert.ErrorIs(t, err, ErrInvalidCredentials)

	svc.googleVerifier = fakeVerifier{claims: ProviderClaims{
		Subject: "different-subject", Email: user.Email, EmailVerified: true,
	}}
	_, err = svc.ReauthenticateProvider(context.Background(), user.ID, ProviderGoogle, "credential")
	assert.ErrorIs(t, err, ErrInvalidProviderToken)

	svc.googleVerifier = fakeVerifier{claims: ProviderClaims{
		Subject: "subject-one", Email: user.Email, EmailVerified: true,
	}}
	reauth, err := svc.ReauthenticateProvider(context.Background(), user.ID, ProviderGoogle, "credential")
	require.NoError(t, err)
	replacement, err := svc.SetFirstPassword(context.Background(), user.ID, reauth, "FirstPassword123")
	require.NoError(t, err)
	assert.NotEmpty(t, replacement)
}

func TestLifecycle_ReauthenticationTokenIsSingleUseAndExpires(t *testing.T) {
	svc, repo, sender, _ := lifecycleHarness()
	user, _ := registerAndVerifyService(t, svc, sender)
	raw, err := svc.ReauthenticatePassword(context.Background(), user.ID, "StrongPassword123")
	require.NoError(t, err)
	require.NoError(t, repo.ConsumeReauthenticationToken(
		context.Background(), user.ID, hashLifecycleToken(raw), time.Now().UTC(),
	))
	assert.ErrorIs(t, repo.ConsumeReauthenticationToken(
		context.Background(), user.ID, hashLifecycleToken(raw), time.Now().UTC(),
	), ErrReauthenticationRequired)

	expired := "expired-reauth"
	require.NoError(t, repo.SaveReauthenticationToken(context.Background(), LifecycleToken{
		ID: "expired-reauth", UserID: user.ID, TokenHash: hashLifecycleToken(expired),
		CreatedAt: time.Now().Add(-time.Hour), ExpiresAt: time.Now().Add(-time.Minute),
	}))
	assert.ErrorIs(t, repo.ConsumeReauthenticationToken(
		context.Background(), user.ID, hashLifecycleToken(expired), time.Now().UTC(),
	), ErrReauthenticationRequired)
}
