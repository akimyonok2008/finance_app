package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ardakimyonok/finance_app/internal/server"
)

// These exercise the CORS policy through the exported router rather than the
// unexported middleware directly, so they double as a regression test for how
// Deps.AppEnv/CORSAllowedOrigins are actually wired.

func doCORSRequest(t *testing.T, appEnv string, allowed []string, origin string) *httptest.ResponseRecorder {
	t.Helper()
	handler := server.New(server.Deps{AppEnv: appEnv, CORSAllowedOrigins: allowed})
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestCORS_DevelopmentWithNoAllowListIsWildcard(t *testing.T) {
	rec := doCORSRequest(t, "development", nil, "https://evil.example.com")
	assert.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_ProductionWithNoAllowListSetsNoOriginHeader(t *testing.T) {
	// A wildcard default in production would let any third-party site read an
	// authenticated response as long as it could smuggle out a bearer token.
	rec := doCORSRequest(t, "production", nil, "https://evil.example.com")
	assert.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_ProductionCheckIsCaseInsensitive(t *testing.T) {
	rec := doCORSRequest(t, "Production", nil, "https://evil.example.com")
	assert.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_AllowListReflectsMatchingOriginOnly(t *testing.T) {
	allowed := []string{"https://app.example.com"}

	match := doCORSRequest(t, "production", allowed, "https://app.example.com")
	assert.Equal(t, "https://app.example.com", match.Header().Get("Access-Control-Allow-Origin"))
	assert.Contains(t, match.Header().Values("Vary"), "Origin")

	mismatch := doCORSRequest(t, "production", allowed, "https://evil.example.com")
	assert.Empty(t, mismatch.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_AllowListAppliesEvenOutsideProduction(t *testing.T) {
	// An operator who configures an explicit allow-list means it, regardless
	// of environment — it must not be silently widened to "*" in dev/test.
	allowed := []string{"https://app.example.com"}

	rec := doCORSRequest(t, "development", allowed, "https://evil.example.com")
	assert.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_PreflightStillReturnsMethodAndHeaderAllowLists(t *testing.T) {
	handler := server.New(server.Deps{AppEnv: "development"})
	req := httptest.NewRequest(http.MethodOptions, "/portfolio/buys", nil)
	req.Header.Set("Origin", "https://app.example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("Access-Control-Allow-Methods"))
	assert.Contains(t, rec.Header().Get("Access-Control-Allow-Headers"), "Idempotency-Key")
}
