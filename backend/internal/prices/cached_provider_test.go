package prices

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingProvider counts how often the underlying provider is hit.
type countingProvider struct {
	mu    sync.Mutex
	inner PriceProvider
	calls int
}

func (c *countingProvider) GetLatestPrice(ctx context.Context, symbol string) (*Price, error) {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	return c.inner.GetLatestPrice(ctx, symbol)
}

func (c *countingProvider) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func TestCachedProvider_SecondCallHitsCache(t *testing.T) {
	counting := &countingProvider{inner: NewMockPriceProvider()}
	cached := NewCachedPriceProvider(counting, NewInMemoryPriceCache(), 5*time.Minute)
	ctx := context.Background()

	p1, err := cached.GetLatestPrice(ctx, "AAPL")
	require.NoError(t, err)
	assert.Equal(t, 195.0, p1.Price)

	p2, err := cached.GetLatestPrice(ctx, "AAPL")
	require.NoError(t, err)
	assert.Equal(t, 195.0, p2.Price)

	assert.Equal(t, 1, counting.count(), "second lookup must be served from cache")
}

func TestCachedProvider_NormalizesSymbolForCacheKey(t *testing.T) {
	counting := &countingProvider{inner: NewMockPriceProvider()}
	cached := NewCachedPriceProvider(counting, NewInMemoryPriceCache(), 5*time.Minute)
	ctx := context.Background()

	_, err := cached.GetLatestPrice(ctx, "AAPL")
	require.NoError(t, err)
	_, err = cached.GetLatestPrice(ctx, " aapl ")
	require.NoError(t, err)

	assert.Equal(t, 1, counting.count(), "differently-cased symbols must share a cache entry")
}

func TestCachedProvider_ExpiredEntryRefetches(t *testing.T) {
	counting := &countingProvider{inner: NewMockPriceProvider()}
	cached := NewCachedPriceProvider(counting, NewInMemoryPriceCache(), 1*time.Millisecond)
	ctx := context.Background()

	_, err := cached.GetLatestPrice(ctx, "AAPL")
	require.NoError(t, err)
	time.Sleep(5 * time.Millisecond)
	_, err = cached.GetLatestPrice(ctx, "AAPL")
	require.NoError(t, err)

	assert.Equal(t, 2, counting.count(), "expired cache entry must trigger a refetch")
}

func TestCachedProvider_ProviderErrorPassesThrough(t *testing.T) {
	counting := &countingProvider{inner: NewMockPriceProvider()}
	cached := NewCachedPriceProvider(counting, NewInMemoryPriceCache(), 5*time.Minute)

	_, err := cached.GetLatestPrice(context.Background(), "ZZZZ")
	assert.ErrorIs(t, err, ErrPriceUnavailable)
}

func TestRedisPriceCache_SetGetAndTTL(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := NewRedisPriceCache(client)
	ctx := context.Background()

	price := Price{Symbol: "AAPL", Price: 195, Currency: "USD", Timestamp: time.Now().UTC(), Source: "mock"}
	require.NoError(t, cache.Set(ctx, price, time.Minute))

	got, ok, err := cache.Get(ctx, "AAPL")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, 195.0, got.Price)
	assert.Equal(t, "USD", got.Currency)

	// After the TTL passes, the entry is gone.
	mr.FastForward(2 * time.Minute)
	_, ok, err = cache.Get(ctx, "AAPL")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestRedisPriceCache_MissIsNotAnError(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := NewRedisPriceCache(client)

	_, ok, err := cache.Get(context.Background(), "NOPE")
	require.NoError(t, err)
	assert.False(t, ok)
}
