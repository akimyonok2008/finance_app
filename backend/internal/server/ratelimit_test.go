package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"

	"github.com/ardakimyonok/finance_app/internal/auth"
)

func TestIPRateLimiter_AllowsUpToLimitThenBlocks(t *testing.T) {
	l := newIPRateLimiter(3, time.Minute)
	ctx := context.Background()

	assert.True(t, l.allow(ctx, "1.2.3.4"))
	assert.True(t, l.allow(ctx, "1.2.3.4"))
	assert.True(t, l.allow(ctx, "1.2.3.4"))
	assert.False(t, l.allow(ctx, "1.2.3.4"), "the 4th request within the window must be blocked")
}

func TestIPRateLimiter_KeysAreIndependent(t *testing.T) {
	l := newIPRateLimiter(1, time.Minute)
	ctx := context.Background()

	assert.True(t, l.allow(ctx, "1.2.3.4"))
	assert.True(t, l.allow(ctx, "5.6.7.8"), "a different key must have its own budget")
}

func TestAuthenticatedRateLimitUsesUserIDNotSharedIP(t *testing.T) {
	limiter := newIPRateLimiter(1, time.Minute)
	tokens := auth.NewTokenManager("rate-limit-test-secret", time.Hour)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := auth.RequireAuth(tokens)(
		rateLimitMiddlewareByKey(limiter, authenticatedUserOrIPKey)(next),
	)
	tokenFor := func(userID string) string {
		token, err := tokens.Generate(userID, userID+"@example.com", 1)
		assert.NoError(t, err)
		return token
	}
	request := func(token string) int {
		req := httptest.NewRequest(http.MethodPost, "/dm/messages", nil)
		req.RemoteAddr = "203.0.113.5:54321"
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Code
	}

	userOne := tokenFor("user-one")
	assert.Equal(t, http.StatusNoContent, request(userOne))
	assert.Equal(t, http.StatusTooManyRequests, request(userOne))
	assert.Equal(t, http.StatusNoContent, request(tokenFor("user-two")),
		"a second authenticated user behind the same IP must have an independent budget")
}

func newTestRedisLimiter(t *testing.T, name string, limit int, window time.Duration) (*redisRateLimiter, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return newRedisRateLimiter(client, name, limit, window), mr
}

func TestRedisRateLimiter_AllowsUpToLimitThenBlocks(t *testing.T) {
	l, _ := newTestRedisLimiter(t, "auth", 3, time.Minute)
	ctx := context.Background()

	assert.True(t, l.allow(ctx, "1.2.3.4"))
	assert.True(t, l.allow(ctx, "1.2.3.4"))
	assert.True(t, l.allow(ctx, "1.2.3.4"))
	assert.False(t, l.allow(ctx, "1.2.3.4"), "the 4th request within the window must be blocked")
}

func TestRedisRateLimiter_DifferentNamesDoNotCollide(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	authLimiter := newRedisRateLimiter(client, "auth", 1, time.Minute)
	accountLimiter := newRedisRateLimiter(client, "account", 1, time.Minute)
	ctx := context.Background()

	// Same IP, but two differently-named limiters sharing one Redis instance
	// — each must have its own budget rather than colliding on one key.
	assert.True(t, authLimiter.allow(ctx, "1.2.3.4"))
	assert.True(t, accountLimiter.allow(ctx, "1.2.3.4"), "a different limiter name must not share the other's budget")
}

func TestRedisRateLimiter_ResetsAfterWindowExpires(t *testing.T) {
	l, mr := newTestRedisLimiter(t, "auth", 1, time.Minute)
	ctx := context.Background()

	assert.True(t, l.allow(ctx, "1.2.3.4"))
	assert.False(t, l.allow(ctx, "1.2.3.4"))

	mr.FastForward(2 * time.Minute)
	assert.True(t, l.allow(ctx, "1.2.3.4"), "a new window must reset the budget")
}

func TestRedisRateLimiter_FallsBackToInMemoryOnRedisError(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(), MaxRetries: -1, DialTimeout: 50 * time.Millisecond,
	})
	l := newRedisRateLimiter(client, "auth", 2, time.Minute)
	ctx := context.Background()

	// Kill Redis: the limiter must not fail open (unlimited) or fail closed
	// (block everything) — it degrades to its local in-memory fallback.
	mr.Close()

	assert.True(t, l.allow(ctx, "1.2.3.4"))
	assert.True(t, l.allow(ctx, "1.2.3.4"))
	assert.False(t, l.allow(ctx, "1.2.3.4"), "the fallback limiter must still enforce a bound during a Redis outage")
}

func TestNewRateLimiter_PicksRedisWhenClientProvided(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	l := newRateLimiter(client, "auth", 5, time.Minute)
	_, ok := l.(*redisRateLimiter)
	assert.True(t, ok, "a Redis client must select the Redis-backed limiter")
}

func TestNewRateLimiter_PicksInMemoryWhenNoClient(t *testing.T) {
	l := newRateLimiter(nil, "auth", 5, time.Minute)
	_, ok := l.(*ipRateLimiter)
	assert.True(t, ok, "no Redis client must select the in-memory limiter")
}

func TestClientIP_StripsPort(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.5:54321"
	assert.Equal(t, "203.0.113.5", clientIP(req))
}

func TestClientIP_FallsBackToRawRemoteAddrWithoutPort(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "not-a-host-port"
	assert.Equal(t, "not-a-host-port", clientIP(req))
}
