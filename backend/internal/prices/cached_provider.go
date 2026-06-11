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
}

// --- in-memory implementation --------------------------------------------------

type cacheEntry struct {
	price     Price
	expiresAt time.Time
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
	c.entries[normalizeSymbol(price.Symbol)] = cacheEntry{price: price, expiresAt: time.Now().Add(ttl)}
	return nil
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

func (c *RedisPriceCache) Get(ctx context.Context, symbol string) (*Price, bool, error) {
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

func (c *RedisPriceCache) Set(ctx context.Context, price Price, ttl time.Duration) error {
	raw, err := json.Marshal(price)
	if err != nil {
		return fmt.Errorf("price cache: marshal: %w", err)
	}
	if err := c.client.Set(ctx, priceCacheKey(price.Symbol), raw, ttl).Err(); err != nil {
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

// GetLatestPrice serves a fresh cached quote when available, otherwise asks the
// underlying provider and caches the result. Cache failures are non-fatal: the
// provider is always the fallback.
func (c *CachedPriceProvider) GetLatestPrice(ctx context.Context, symbol string) (*Price, error) {
	sym := normalizeSymbol(symbol)

	if cached, ok, err := c.cache.Get(ctx, sym); err == nil && ok {
		return cached, nil
	}

	price, err := c.provider.GetLatestPrice(ctx, sym)
	if err != nil {
		return nil, err
	}
	_ = c.cache.Set(ctx, *price, c.ttl) // best-effort
	return price, nil
}
