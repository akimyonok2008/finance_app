// Package ttlcache is a tiny, generic, per-key, in-process TTL cache. It
// exists to memoize expensive, frequently-repeated per-user computations
// (leaderboard enrichment, Explore's caller-comparison summary) without
// pulling in Redis or another external dependency for what is purely an
// in-process read optimization — the underlying data is never the source of
// truth, so a bounded staleness window is an acceptable, deliberate tradeoff.
package ttlcache

import (
	"sync"
	"time"
)

// Cache maps string keys to values of type T, each expiring TTL after it was
// set. It is safe for concurrent use.
type Cache[T any] struct {
	mu      sync.Mutex
	ttl     time.Duration
	entries map[string]entry[T]
}

type entry[T any] struct {
	value     T
	expiresAt time.Time
}

// New returns an empty cache whose entries live for ttl.
func New[T any](ttl time.Duration) *Cache[T] {
	return &Cache[T]{ttl: ttl, entries: make(map[string]entry[T])}
}

// Get returns the cached value for key and true, or the zero value and false
// if the key is absent or has expired.
func (c *Cache[T]) Get(key string) (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok || time.Now().After(e.expiresAt) {
		var zero T
		return zero, false
	}
	return e.value, true
}

// Set stores value under key, expiring after the cache's configured TTL.
func (c *Cache[T]) Set(key string, value T) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = entry[T]{value: value, expiresAt: time.Now().Add(c.ttl)}
}

// Invalidate removes key immediately, e.g. right after a mutation that
// changed the value a caller doesn't want to wait TTL to see.
func (c *Cache[T]) Invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}
