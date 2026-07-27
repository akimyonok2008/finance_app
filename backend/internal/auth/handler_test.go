package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestHandler wires a fresh service + handler + router for HTTP-level tests.
func newTestHandler() (*Handler, *Service, http.Handler) {
	repo := NewInMemoryUserRepository()
	tm := NewTokenManager("test-secret", time.Hour)
	svc := NewService(repo, tm)
	h := NewHandler(svc)

	r := chi.NewRouter()
	r.Post("/auth/register", h.Register)
	r.Post("/auth/login", h.Login)
	r.Post("/auth/google", h.Google)
	r.With(RequireAuth(tm)).Get("/me", h.Me)
	r.With(RequireAuth(tm)).Post("/auth/change-password", h.ChangePassword)
	r.With(RequireAuth(tm)).Post("/auth/delete-account", h.DeleteAccount)
	return h, svc, r
}

func doJSON(t *testing.T, router http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == "" {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader([]byte(body))
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

const registerBody = `{"email":"user@example.com","password":"StrongPassword123","display_name":"AlphaWolf_91"}`

func TestHandlerRegister_Returns201ForValidRequest(t *testing.T) {
	_, _, router := newTestHandler()

	rec := doJSON(t, router, http.MethodPost, "/auth/register", registerBody, "")

	assert.Equal(t, http.StatusCreated, rec.Code)

	var resp authResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.Token)
	assert.Equal(t, "user@example.com", resp.User.Email)
	assert.Equal(t, "AlphaWolf_91", resp.User.DisplayName)
	assert.NotEmpty(t, resp.User.ID)
}

func TestHandlerRegister_Returns400ForInvalidRequest(t *testing.T) {
	_, _, router := newTestHandler()

	// Password too short.
	body := `{"email":"user@example.com","password":"short","display_name":"X"}`
	rec := doJSON(t, router, http.MethodPost, "/auth/register", body, "")

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assertHasError(t, rec.Body.Bytes())
}

func TestHandlerRegister_Returns400ForMalformedJSON(t *testing.T) {
	_, _, router := newTestHandler()

	rec := doJSON(t, router, http.MethodPost, "/auth/register", `{not json`, "")

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandlerRegister_Returns409ForDuplicateEmail(t *testing.T) {
	_, _, router := newTestHandler()
	doJSON(t, router, http.MethodPost, "/auth/register", registerBody, "")

	rec := doJSON(t, router, http.MethodPost, "/auth/register", registerBody, "")

	assert.Equal(t, http.StatusConflict, rec.Code)
	assertHasError(t, rec.Body.Bytes())
}

func TestHandlerLogin_Returns200ForValidCredentials(t *testing.T) {
	_, _, router := newTestHandler()
	doJSON(t, router, http.MethodPost, "/auth/register", registerBody, "")

	body := `{"email":"user@example.com","password":"StrongPassword123"}`
	rec := doJSON(t, router, http.MethodPost, "/auth/login", body, "")

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp authResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.Token)
}

func TestHandlerLogin_Returns401ForWrongPassword(t *testing.T) {
	_, _, router := newTestHandler()
	doJSON(t, router, http.MethodPost, "/auth/register", registerBody, "")

	body := `{"email":"user@example.com","password":"WrongPassword"}`
	rec := doJSON(t, router, http.MethodPost, "/auth/login", body, "")

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assertHasError(t, rec.Body.Bytes())
}

func TestHandlerLogin_Returns401ForUnknownEmail(t *testing.T) {
	_, _, router := newTestHandler()

	body := `{"email":"ghost@example.com","password":"whatever123"}`
	rec := doJSON(t, router, http.MethodPost, "/auth/login", body, "")

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestHandlerGoogle_Returns503WhenDisabled(t *testing.T) {
	_, _, router := newTestHandler()

	rec := doJSON(t, router, http.MethodPost, "/auth/google", `{"credential":"id-token"}`, "")

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assertHasError(t, rec.Body.Bytes())
}

func TestHandlerGoogle_Returns400ForMissingCredential(t *testing.T) {
	_, svc, router := newTestHandler()
	svc.ConfigureProviderAuth(ProviderAuthConfig{
		GoogleEnabled:  true,
		GoogleVerifier: fakeVerifier{},
	})

	rec := doJSON(t, router, http.MethodPost, "/auth/google", `{"credential":"   "}`, "")

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assertHasError(t, rec.Body.Bytes())
}

func TestHandlerGoogle_Returns401ForInvalidCredential(t *testing.T) {
	_, svc, router := newTestHandler()
	svc.ConfigureProviderAuth(ProviderAuthConfig{
		GoogleEnabled:  true,
		GoogleVerifier: fakeVerifier{err: ErrInvalidProviderToken},
	})

	rec := doJSON(t, router, http.MethodPost, "/auth/google", `{"credential":"bad-token"}`, "")

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assertHasError(t, rec.Body.Bytes())
}

func TestHandlerGoogle_ReturnsJWTForVerifiedCredential(t *testing.T) {
	_, svc, router := newTestHandler()
	svc.ConfigureProviderAuth(ProviderAuthConfig{
		GoogleEnabled: true,
		GoogleVerifier: fakeVerifier{claims: ProviderClaims{
			Subject:       "google-sub-1",
			Email:         "GoogleUser@example.com",
			EmailVerified: true,
			DisplayName:   "Google User",
		}},
	})

	rec := doJSON(t, router, http.MethodPost, "/auth/google", `{"credential":"id-token"}`, "")

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp authResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.Token)
	assert.Equal(t, "googleuser@example.com", resp.User.Email)
	assert.Equal(t, "Google User", resp.User.DisplayName)
	assertNoPasswordLeak(t, rec.Body.String())
	assert.NotContains(t, rec.Body.String(), "google-sub-1")
}

func TestHandlerMe_Returns200WithValidToken(t *testing.T) {
	_, _, router := newTestHandler()
	regRec := doJSON(t, router, http.MethodPost, "/auth/register", registerBody, "")
	var reg authResponse
	require.NoError(t, json.Unmarshal(regRec.Body.Bytes(), &reg))

	rec := doJSON(t, router, http.MethodGet, "/me", "", reg.Token)

	assert.Equal(t, http.StatusOK, rec.Code)
	var pub PublicUser
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &pub))
	assert.Equal(t, "user@example.com", pub.Email)
	assert.Equal(t, reg.User.ID, pub.ID)
}

func TestHandlerMe_Returns401ForValidTokenOfMissingUser(t *testing.T) {
	// Simulates an in-memory restart: token is syntactically valid but the user
	// no longer exists in the repository.
	_, _, router := newTestHandler()
	tm := NewTokenManager("test-secret", time.Hour)
	ghost, err := tm.Generate("ghost-id", "ghost@example.com")
	require.NoError(t, err)

	rec := doJSON(t, router, http.MethodGet, "/me", "", ghost)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestHandlerMe_Returns401WithoutToken(t *testing.T) {
	_, _, router := newTestHandler()

	rec := doJSON(t, router, http.MethodGet, "/me", "", "")

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestHandlerMe_Returns401WithInvalidToken(t *testing.T) {
	_, _, router := newTestHandler()

	rec := doJSON(t, router, http.MethodGet, "/me", "", "this.is.not.a.valid.jwt")

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// --- Security / privacy tests -------------------------------------------------

func TestSecurity_RegisterResponseNeverExposesPassword(t *testing.T) {
	_, _, router := newTestHandler()

	rec := doJSON(t, router, http.MethodPost, "/auth/register", registerBody, "")

	assertNoPasswordLeak(t, rec.Body.String())
}

func TestSecurity_LoginResponseNeverExposesPassword(t *testing.T) {
	_, _, router := newTestHandler()
	doJSON(t, router, http.MethodPost, "/auth/register", registerBody, "")

	body := `{"email":"user@example.com","password":"StrongPassword123"}`
	rec := doJSON(t, router, http.MethodPost, "/auth/login", body, "")

	assertNoPasswordLeak(t, rec.Body.String())
}

func TestSecurity_MeResponseNeverExposesPassword(t *testing.T) {
	_, _, router := newTestHandler()
	regRec := doJSON(t, router, http.MethodPost, "/auth/register", registerBody, "")
	var reg authResponse
	require.NoError(t, json.Unmarshal(regRec.Body.Bytes(), &reg))

	rec := doJSON(t, router, http.MethodGet, "/me", "", reg.Token)

	assertNoPasswordLeak(t, rec.Body.String())
}

func TestHandlerChangePassword_Returns204AndRotatesCredential(t *testing.T) {
	_, _, router := newTestHandler()
	regRec := doJSON(t, router, http.MethodPost, "/auth/register", registerBody, "")
	var reg authResponse
	require.NoError(t, json.Unmarshal(regRec.Body.Bytes(), &reg))

	rec := doJSON(t, router, http.MethodPost, "/auth/change-password",
		`{"current_password":"StrongPassword123","new_password":"EvenStrongerPassword456"}`, reg.Token)
	assert.Equal(t, http.StatusNoContent, rec.Code)

	oldLogin := doJSON(t, router, http.MethodPost, "/auth/login",
		`{"email":"user@example.com","password":"StrongPassword123"}`, "")
	assert.Equal(t, http.StatusUnauthorized, oldLogin.Code, "the old password must stop working")

	newLogin := doJSON(t, router, http.MethodPost, "/auth/login",
		`{"email":"user@example.com","password":"EvenStrongerPassword456"}`, "")
	assert.Equal(t, http.StatusOK, newLogin.Code)
}

func TestHandlerChangePassword_Returns401ForWrongCurrentPassword(t *testing.T) {
	_, _, router := newTestHandler()
	regRec := doJSON(t, router, http.MethodPost, "/auth/register", registerBody, "")
	var reg authResponse
	require.NoError(t, json.Unmarshal(regRec.Body.Bytes(), &reg))

	rec := doJSON(t, router, http.MethodPost, "/auth/change-password",
		`{"current_password":"WrongPassword","new_password":"EvenStrongerPassword456"}`, reg.Token)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assertHasError(t, rec.Body.Bytes())
}

func TestHandlerChangePassword_Returns401WithoutToken(t *testing.T) {
	_, _, router := newTestHandler()

	rec := doJSON(t, router, http.MethodPost, "/auth/change-password",
		`{"current_password":"x","new_password":"EvenStrongerPassword456"}`, "")

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestHandlerDeleteAccount_Returns204ThenTokenStopsWorking(t *testing.T) {
	_, _, router := newTestHandler()
	regRec := doJSON(t, router, http.MethodPost, "/auth/register", registerBody, "")
	var reg authResponse
	require.NoError(t, json.Unmarshal(regRec.Body.Bytes(), &reg))

	rec := doJSON(t, router, http.MethodPost, "/auth/delete-account",
		`{"password":"StrongPassword123"}`, reg.Token)
	assert.Equal(t, http.StatusNoContent, rec.Code)

	loginRec := doJSON(t, router, http.MethodPost, "/auth/login",
		`{"email":"user@example.com","password":"StrongPassword123"}`, "")
	assert.Equal(t, http.StatusUnauthorized, loginRec.Code, "login must fail after deletion")
}

func TestHandlerDeleteAccount_Returns401ForWrongPassword(t *testing.T) {
	_, _, router := newTestHandler()
	regRec := doJSON(t, router, http.MethodPost, "/auth/register", registerBody, "")
	var reg authResponse
	require.NoError(t, json.Unmarshal(regRec.Body.Bytes(), &reg))

	rec := doJSON(t, router, http.MethodPost, "/auth/delete-account",
		`{"password":"WrongPassword"}`, reg.Token)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	loginRec := doJSON(t, router, http.MethodPost, "/auth/login",
		`{"email":"user@example.com","password":"StrongPassword123"}`, "")
	assert.Equal(t, http.StatusOK, loginRec.Code, "a rejected deletion must leave the account usable")
}

func assertHasError(t *testing.T, body []byte) {
	t.Helper()
	var e map[string]any
	require.NoError(t, json.Unmarshal(body, &e))
	assert.NotEmpty(t, e["error"], "error responses must use {\"error\": \"...\"} format")
}

func assertNoPasswordLeak(t *testing.T, body string) {
	t.Helper()
	lower := strings.ToLower(body)
	assert.NotContains(t, lower, "password_hash")
	assert.NotContains(t, lower, "passwordhash")
	assert.NotContains(t, lower, "\"password\"")
	assert.NotContains(t, body, "StrongPassword123")
}
