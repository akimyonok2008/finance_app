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
	r.With(RequireAuth(tm)).Get("/me", h.Me)
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
