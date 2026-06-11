package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// echoUserHandler writes the authenticated user id pulled from the context so
// tests can confirm the middleware populated it.
func echoUserHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := UserIDFromContext(r.Context())
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(id))
}

func serveProtected(t *testing.T, tm *TokenManager, authHeader string) *httptest.ResponseRecorder {
	t.Helper()
	handler := RequireAuth(tm)(http.HandlerFunc(echoUserHandler))
	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestMiddleware_MissingTokenReturns401(t *testing.T) {
	tm := NewTokenManager("secret", time.Hour)
	rec := serveProtected(t, tm, "")
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestMiddleware_MalformedHeaderReturns401(t *testing.T) {
	tm := NewTokenManager("secret", time.Hour)
	// Missing the "Bearer " scheme prefix.
	rec := serveProtected(t, tm, "token-without-scheme")
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestMiddleware_InvalidTokenReturns401(t *testing.T) {
	tm := NewTokenManager("secret", time.Hour)
	rec := serveProtected(t, tm, "Bearer not.a.real.token")
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestMiddleware_TokenSignedWithWrongSecretReturns401(t *testing.T) {
	signer := NewTokenManager("other-secret", time.Hour)
	token, err := signer.Generate("user-1", "user@example.com")
	require.NoError(t, err)

	verifier := NewTokenManager("secret", time.Hour)
	rec := serveProtected(t, verifier, "Bearer "+token)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestMiddleware_ExpiredTokenReturns401(t *testing.T) {
	tm := NewTokenManager("secret", -time.Hour) // already expired
	token, err := tm.Generate("user-1", "user@example.com")
	require.NoError(t, err)

	rec := serveProtected(t, tm, "Bearer "+token)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestMiddleware_ValidTokenPassesAndSetsUserID(t *testing.T) {
	tm := NewTokenManager("secret", time.Hour)
	token, err := tm.Generate("user-42", "user@example.com")
	require.NoError(t, err)

	rec := serveProtected(t, tm, "Bearer "+token)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "user-42", rec.Body.String())
}

type fakeUserStore struct{ existing map[string]bool }

func (f fakeUserStore) UserByID(id string) (*User, error) {
	if f.existing[id] {
		return &User{ID: id}, nil
	}
	return nil, ErrUserNotFound
}

func TestRequireAuthWithUser_MissingUserReturns401(t *testing.T) {
	tm := NewTokenManager("secret", time.Hour)
	token, err := tm.Generate("ghost", "ghost@example.com")
	require.NoError(t, err)

	store := fakeUserStore{existing: map[string]bool{}} // ghost does not exist
	handler := RequireAuthWithUser(tm, store)(http.HandlerFunc(echoUserHandler))
	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestRequireAuthWithUser_ExistingUserPasses(t *testing.T) {
	tm := NewTokenManager("secret", time.Hour)
	token, err := tm.Generate("user-1", "u@example.com")
	require.NoError(t, err)

	store := fakeUserStore{existing: map[string]bool{"user-1": true}}
	handler := RequireAuthWithUser(tm, store)(http.HandlerFunc(echoUserHandler))
	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "user-1", rec.Body.String())
}

func TestTokenManager_GenerateAndParseRoundTrip(t *testing.T) {
	tm := NewTokenManager("secret", time.Hour)
	token, err := tm.Generate("user-7", "user@example.com")
	require.NoError(t, err)

	claims, err := tm.Parse(token)
	require.NoError(t, err)
	assert.Equal(t, "user-7", claims.UserID)
	assert.Equal(t, "user@example.com", claims.Email)
}
