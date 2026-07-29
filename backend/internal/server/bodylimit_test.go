package server

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMaxBodyBytesRejectsKnownOversizeBeforeHandler(t *testing.T) {
	called := false
	handler := maxBodyBytes(8)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("123456789"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	assert.False(t, called)
}

func TestMaxBodyBytesStopsStreamedBodyAtLimit(t *testing.T) {
	handler := maxBodyBytes(8)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		var tooLarge *http.MaxBytesError
		require.True(t, errors.As(err, &tooLarge))
	}))
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("123456789"))
	req.ContentLength = -1

	handler.ServeHTTP(httptest.NewRecorder(), req)
}

func TestAuthRouteUsesSmallerBodyLimitThanGlobalLimit(t *testing.T) {
	handler := New(Deps{AppEnv: "development"})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(strings.Repeat("x", int(authBodyLimit+1))))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}
