package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func newTestService() *Service {
	repo := NewInMemoryUserRepository()
	tm := NewTokenManager("test-secret", time.Hour)
	return NewService(repo, tm)
}

func validInput() RegisterInput {
	return RegisterInput{
		Email:       "User@Example.com",
		Password:    "StrongPassword123",
		DisplayName: "AlphaWolf_91",
	}
}

func TestRegister_ValidUserCreatesUserAndReturnsToken(t *testing.T) {
	svc := newTestService()

	user, token, err := svc.Register(validInput())

	require.NoError(t, err)
	require.NotNil(t, user)
	assert.NotEmpty(t, user.ID)
	assert.Equal(t, "AlphaWolf_91", user.DisplayName)
	assert.NotEmpty(t, token, "a JWT token must be returned")
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

func TestRepository_FindByIDUnknownReturnsNotFound(t *testing.T) {
	repo := NewInMemoryUserRepository()

	_, err := repo.FindByID("does-not-exist")

	assert.True(t, errors.Is(err, ErrUserNotFound))
}
