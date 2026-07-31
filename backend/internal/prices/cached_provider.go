package prices

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// PriceCache stores recent quotes so repeated lookups don't hammer the
// underlying provider.
type PriceCache interface {
	// Get returns the cached price and ok=true on a fresh hit.
	Get(ctx context.Context, symbol string) (*Price, bool, error)
	// Set stores a price with a time-to-live.
	Set(ctx context.Context, price Price, ttl time.Duration) error
	// GetStale returns the last observed price for symbol regardless of
	// freshness, bounded only by staleRetention. It exists so a provider
	// outage or a symbol the provider has stopped covering (a rate limit, a
	// delisting, a coverage gap) can fall back to the last known value instead
	// of making every quote lookup for that symbol fail.
	GetStale(ctx context.Context, symbol string) (*Price, bool, error)
}

// staleRetention bounds how long a last-known price remains usable as a
// fallback once it stops being fresh. It is deliberately long (weeks, not
// minutes): the goal is to keep a holding valued at its last observed price
// through a provider outage or permanent coverage loss, not to manufacture a
// fresh-looking quote. A quote older than this is treated as never cached.
const staleRetention = 30 * 24 * time.Hour

// --- in-memory implementation --------------------------------------------------

type cacheEntry struct {
	price     Price
	expiresAt time.Time
	storedAt  time.Time
}

// InMemoryPriceCache is a process-local TTL cache, the default when Redis is
// not configured.
type InMemoryPriceCache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
}

// NewInMemoryPriceCache returns an empty in-memory price cache.
func NewInMemoryPriceCache() *InMemoryPriceCache {
	return &InMemoryPriceCache{entries: make(map[string]cacheEntry)}
}

func (c *InMemoryPriceCache) Get(_ context.Context, symbol string) (*Price, bool, error) {
	c.mu.RLock()
	e, ok := c.entries[normalizeSymbol(symbol)]
	c.mu.RUnlock()
	if !ok || time.Now().After(e.expiresAt) {
		return nil, false, nil
	}
	copied := e.price
	return &copied, true, nil
}

func (c *InMemoryPriceCache) Set(_ context.Context, price Price, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	c.entries[normalizeSymbol(price.Symbol)] = cacheEntry{price: price, expiresAt: now.Add(ttl), storedAt: now}
	return nil
}

func (c *InMemoryPriceCache) GetStale(_ context.Context, symbol string) (*Price, bool, error) {
	c.mu.RLock()
	e, ok := c.entries[normalizeSymbol(symbol)]
	c.mu.RUnlock()
	if !ok || time.Since(e.storedAt) > staleRetention {
		return nil, false, nil
	}
	copied := e.price
	return &copied, true, nil
}

// --- Redis implementation -------------------------------------------------------

// RedisPriceCache stores quotes as JSON values with Redis-native TTL expiry.
type RedisPriceCache struct {
	client *redis.Client
}

// NewRedisPriceCache wraps a connected Redis client.
func NewRedisPriceCache(client *redis.Client) *RedisPriceCache {
	return &RedisPriceCache{client: client}
}

func priceCacheKey(symbol string) string {
	return "price:" + normalizeSymbol(symbol)
}

// Get returns the cached price only while it is still fresh. Freshness is
// judged from the embedded ExpiresAt stamp set by Set, NOT from whether the
// Redis key still exists: the key itself is kept alive for staleRetention (far
// longer than any freshness ttl) so GetStale can still recover it later.
func (c *RedisPriceCache) Get(ctx context.Context, symbol string) (*Price, bool, error) {
	p, ok, err := c.get(ctx, symbol)
	if err != nil || !ok {
		return nil, false, err
	}
	if !p.ExpiresAt.IsZero() && time.Now().After(p.ExpiresAt) {
		return nil, false, nil
	}
	return p, true, nil
}

// GetStale returns the last observed price regardless of its ExpiresAt stamp,
// bounded only by the Redis key's own staleRetention TTL.
func (c *RedisPriceCache) GetStale(ctx context.Context, symbol string) (*Price, bool, error) {
	return c.get(ctx, symbol)
}

func (c *RedisPriceCache) get(ctx context.Context, symbol string) (*Price, bool, error) {
	raw, err := c.client.Get(ctx, priceCacheKey(symbol)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("price cache: get: %w", err)
	}
	var p Price
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, false, nil // treat corrupt entries as a miss
	}
	return &p, true, nil
}

// Set stores price under a physical Redis TTL of staleRetention (not ttl), and
// stamps ExpiresAt = now + ttl so Get can still judge freshness precisely. This
// keeps the entry available to GetStale long after it stops being "fresh".
func (c *RedisPriceCache) Set(ctx context.Context, price Price, ttl time.Duration) error {
	price.ExpiresAt = time.Now().Add(ttl)
	raw, err := json.Marshal(price)
	if err != nil {
		return fmt.Errorf("price cache: marshal: %w", err)
	}
	retention := staleRetention
	if ttl > retention {
		retention = ttl
	}
	if err := c.client.Set(ctx, priceCacheKey(price.Symbol), raw, retention).Err(); err != nil {
		return fmt.Errorf("price cache: set: %w", err)
	}
	return nil
}

// --- caching decorator ----------------------------------------------------------

// CachedPriceProvider decorates any PriceProvider with a TTL cache. It is
// itself a PriceProvider, so it slots transparently into every consumer
// (portfolio summary, sprint snapshots, /prices).
type CachedPriceProvider struct {
	provider PriceProvider
	cache    PriceCache
	ttl      time.Duration
}

// NewCachedPriceProvider wraps provider with the given cache and TTL.
func NewCachedPriceProvider(provider PriceProvider, cache PriceCache, ttl time.Duration) *CachedPriceProvider {
	return &CachedPriceProvider{provider: provider, cache: cache, ttl: ttl}
}

// ProviderStatusStaleFallback marks a price served from GetStale after the
// live provider failed: it is a real, previously observed price, not an
// invented one, but it may be well behind the market.
const ProviderStatusStaleFallback = "provider_unavailable"

// GetLatestPrice serves a fresh cached quote when available, otherwise asks the
// underlying provider and caches the result. Cache failures are non-fatal: the
// provider is always the fallback.
//
// If the live provider errors (outage, rate limit, or a symbol it has stopped
// covering, e.g. a delisting), the last observed price is served instead of
// propagating the error — so a single broken symbol degrades to a stale
// valuation for that holding rather than making every quote lookup, and every
// portfolio mutation that revalues it, fail outright. A symbol that was never
// successfully priced still returns the provider's error: there is no
// last-known value to fall back to.
// PriceAtOrBefore delegates directly to the wrapped provider when it
// supports historical lookups. Unlike GetLatestPrice this is not cached: a
// point-in-time historical price never goes stale, and callers (competition
// boundary observation capture) already dedupe/persist the result themselves.
func (c *CachedPriceProvider) PriceAtOrBefore(ctx context.Context, symbol string, at time.Time) (*HistoricalPrice, error) {
	hp, ok := c.provider.(HistoricalPriceProvider)
	if !ok {
		return nil, ErrPriceUnavailable
	}
	return hp.PriceAtOrBefore(ctx, symbol, at)
}

func (c *CachedPriceProvider) GetLatestPrice(ctx context.Context, symbol string) (*Price, error) {
	sym := normalizeSymbol(symbol)

	if cached, ok, err := c.cache.Get(ctx, sym); err == nil && ok {
		return cached, nil
	}

	price, err := c.provider.GetLatestPrice(ctx, sym)
	if err != nil {
		if stale, ok, serr := c.cache.GetStale(ctx, sym); serr == nil && ok {
			frozen := *stale
			frozen.IsStale = true
			frozen.ProviderStatus = ProviderStatusStaleFallback
			return &frozen, nil
		}
		return nil, err
	}
	now := time.Now().UTC()
	if price.FetchedAt.IsZero() {
		price.FetchedAt = now
	}
	price.ExpiresAt = now.Add(c.ttl)
	_ = c.cache.Set(ctx, *price, c.ttl) // best-effort
	return price, nil
}
