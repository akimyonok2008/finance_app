package server

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/ardakimyonok/finance_app/internal/httpx"
)

// rateLimiter is satisfied by both the in-memory and Redis-backed limiters, so
// rateLimitMiddleware doesn't care which one it's enforcing.
type rateLimiter interface {
	allow(ctx context.Context, key string) bool
}

// newRateLimiter picks the Redis-backed limiter when a client is available (so
// every replica behind a load balancer shares one budget instead of each
// enforcing its own), falling back to the in-memory limiter when Redis isn't
// configured — correct for a single instance, just not coordinated across
// replicas. name namespaces the Redis keys so two differently-configured
// limiters (e.g. auth vs. account) sharing one Redis instance never collide
// on the same key for the same client IP.
func newRateLimiter(redisClient *redis.Client, name string, limit int, window time.Duration) rateLimiter {
	if redisClient != nil {
		return newRedisRateLimiter(redisClient, name, limit, window)
	}
	return newIPRateLimiter(limit, window)
}

// ipRateLimiter is a simple fixed-window limiter keyed by client IP. Without
// it, /auth/login and /auth/register are an unbounded surface: each attempt
// costs the attacker nothing but forces the server to run bcrypt (deliberately
// slow, ~10 rounds) on every guess, and there is no other bound on
// credential-stuffing volume.
type ipRateLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	hits   map[string]*windowCount
}

type windowCount struct {
	count      int
	windowFrom time.Time
}

// newIPRateLimiter allows up to limit requests per key within window, and
// starts a background sweep so IPs that stop sending requests don't linger in
// memory forever.
func newIPRateLimiter(limit int, window time.Duration) *ipRateLimiter {
	l := &ipRateLimiter{limit: limit, window: window, hits: make(map[string]*windowCount)}
	go l.sweepLoop()
	return l
}

func (l *ipRateLimiter) allow(_ context.Context, key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	wc, ok := l.hits[key]
	if !ok || now.Sub(wc.windowFrom) >= l.window {
		l.hits[key] = &windowCount{count: 1, windowFrom: now}
		return true
	}
	if wc.count >= l.limit {
		return false
	}
	wc.count++
	return true
}

func (l *ipRateLimiter) sweepLoop() {
	ticker := time.NewTicker(l.window)
	defer ticker.Stop()
	for range ticker.C {
		l.sweep()
	}
}

func (l *ipRateLimiter) sweep() {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := time.Now().Add(-l.window)
	for k, wc := range l.hits {
		if wc.windowFrom.Before(cutoff) {
			delete(l.hits, k)
		}
	}
}

// redisRateLimiter is a fixed-window limiter backed by Redis INCR+EXPIRE, so
// multiple API replicas behind a load balancer share one limit instead of
// each independently allowing `limit` requests (an in-memory limiter's budget
// multiplies by however many replicas are running). It falls back to a local
// in-memory limiter for the duration of any Redis error, so a Redis outage
// degrades to per-instance limiting rather than either blocking every request
// or silently disabling the limiter altogether.
type redisRateLimiter struct {
	client   *redis.Client
	name     string
	limit    int
	window   time.Duration
	fallback *ipRateLimiter
}

func newRedisRateLimiter(client *redis.Client, name string, limit int, window time.Duration) *redisRateLimiter {
	return &redisRateLimiter{client: client, name: name, limit: limit, window: window, fallback: newIPRateLimiter(limit, window)}
}

func (l *redisRateLimiter) allow(ctx context.Context, key string) bool {
	redisKey := "ratelimit:" + l.name + ":" + key
	count, err := l.client.Incr(ctx, redisKey).Result()
	if err != nil {
		return l.fallback.allow(ctx, key)
	}
	if count == 1 {
		// First hit opens this key's window. Best-effort: if Expire fails the
		// key just never expires until cleaned up some other way, which only
		// makes the limit stricter (never bypassable), so it's safe to ignore.
		_ = l.client.Expire(ctx, redisKey, l.window).Err()
	}
	return count <= int64(l.limit)
}

// rateLimitMiddleware enforces limiter per client IP. It must run after
// middleware.RealIP so r.RemoteAddr reflects the real client behind a proxy.
func rateLimitMiddleware(limiter rateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiter.allow(r.Context(), clientIP(r)) {
				httpx.WriteError(w, http.StatusTooManyRequests, "too many requests; try again shortly")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
