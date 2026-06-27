package profile

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/auth"
)

func TestHandlersRequireAuthenticatedContext(t *testing.T) {
	handler := NewHandler(testService())
	for _, test := range []struct {
		method string
		call   http.HandlerFunc
	}{
		{http.MethodGet, handler.GetMe},
		{http.MethodPatch, handler.UpdateMe},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(test.method, "/profiles/me", nil)
		test.call(rec, req)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	}
}

type authUserProvider struct{ svc *auth.Service }

func (p authUserProvider) GetUserByID(_ context.Context, id string) (*auth.User, error) {
	return p.svc.UserByID(id)
}

func TestProfileRoutesAuthenticatedUpdateAndPrivacy(t *testing.T) {
	tokens := auth.NewTokenManager("test-secret", time.Hour)
	authSvc := auth.NewService(auth.NewInMemoryUserRepository(), tokens)
	user, token, err := authSvc.Register(auth.RegisterInput{
		Email: "owner@example.com", Password: "StrongPassword123", DisplayName: "Route Owner",
	})
	require.NoError(t, err)

	profileSvc := NewService(NewInMemoryRepository(), authUserProvider{authSvc}, testSummaries{})
	router := chi.NewRouter()
	router.Use(auth.RequireAuthWithUser(tokens, authSvc))
	handler := NewHandler(profileSvc)
	router.Get("/profiles/me", handler.GetMe)
	router.Patch("/profiles/me", handler.UpdateMe)
	router.Get("/profiles/{handle}", handler.GetPublic)

	do := func(method, path, body, bearer string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	assert.Equal(t, http.StatusUnauthorized, do(http.MethodGet, "/profiles/me", "", "").Code)

	me := do(http.MethodGet, "/profiles/me", "", token)
	require.Equal(t, http.StatusOK, me.Code)
	var owner OwnerProfile
	require.NoError(t, json.Unmarshal(me.Body.Bytes(), &owner))
	assert.False(t, owner.IsPublic)
	assert.Equal(t, "route_owner", owner.Handle)

	assert.Equal(t, http.StatusNotFound, do(http.MethodGet, "/profiles/"+owner.Handle, "", token).Code)
	patch := do(http.MethodPatch, "/profiles/me", `{"is_public":true,"bio":"Long-term learner"}`, token)
	require.Equal(t, http.StatusOK, patch.Code)

	public := do(http.MethodGet, "/profiles/"+owner.Handle, "", token)
	require.Equal(t, http.StatusOK, public.Code)
	assert.NotContains(t, public.Body.String(), user.ID)
	assert.NotContains(t, public.Body.String(), user.Email)
	assert.NotContains(t, public.Body.String(), "password")
}
