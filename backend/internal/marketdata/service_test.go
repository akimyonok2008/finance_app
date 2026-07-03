package marketdata

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ardakimyonok/finance_app/internal/prices"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type countingProvider struct {
	quotes []Quote
	err    error
	calls  int
}

func (p *countingProvider) SearchInstruments(context.Context, string) ([]Instrument, error) {
	return nil, nil
}

func (p *countingProvider) GetQuote(ctx context.Context, symbol string) (Quote, error) {
	quotes, err := p.GetQuotes(ctx, []string{symbol})
	if err != nil {
		return Quote{}, err
	}
	return quotes[0], nil
}

func (p *countingProvider) GetQuotes(_ context.Context, symbols []string) ([]Quote, error) {
	p.calls++
	if p.err != nil {
		return nil, p.err
	}
	now := time.Now().UTC()
	out := make([]Quote, 0, len(symbols))
	for _, symbol := range symbols {
		out = append(out, Quote{
			Symbol: normalizeSymbol(symbol), Price: 100, Currency: "USD",
			Provider: "test", ProviderStatus: StatusOK,
			FetchedAt: now, ExpiresAt: now.Add(time.Minute),
		})
	}
	return out, nil
}

func newTestService(provider Provider) *Service {
	return NewService(NewInMemoryRepository(), provider, ServiceConfig{
		RealMarketDataEnabled: true,
		QuoteCacheTTL:         10 * time.Minute,
		QuoteStaleAfter:       15 * time.Minute,
		AllowStaleOnError:     true,
		SearchLimit:           10,
		MaxQuoteBatchSize:     25,
	})
}

func TestService_FreshCacheReturnsWithoutProviderCall(t *testing.T) {
	provider := &countingProvider{}
	svc := newTestService(provider)

	resp, err := svc.Quotes(context.Background(), []string{"AAPL"})
	require.NoError(t, err)
	require.Len(t, resp.Quotes, 1)
	assert.Equal(t, 1, provider.calls)

	resp, err = svc.Quotes(context.Background(), []string{"AAPL"})
	require.NoError(t, err)
	require.Len(t, resp.Quotes, 1)
	assert.Equal(t, 1, provider.calls)
	assert.Equal(t, "cache", resp.Source)
}

func TestService_StaleCacheReturnedIfProviderFails(t *testing.T) {
	provider := &countingProvider{}
	svc := newTestService(provider)
	now := time.Now().UTC().Add(-30 * time.Minute)
	require.NoError(t, svc.repo.UpsertQuotes(context.Background(), []Quote{{
		Symbol: "AAPL", Price: 90, Currency: "USD", Provider: "test",
		ProviderStatus: StatusOK, FetchedAt: now, ExpiresAt: now.Add(-time.Minute),
	}}))
	provider.err = ErrProviderUnavailable

	resp, err := svc.Quotes(context.Background(), []string{"AAPL"})
	require.NoError(t, err)
	require.Len(t, resp.Quotes, 1)
	assert.True(t, resp.Quotes[0].IsStale)
	assert.Equal(t, StatusProviderUnavailable, resp.Quotes[0].ProviderStatus)
	assert.Equal(t, 90.0, resp.Quotes[0].Price)
}

func TestService_NoCachedQuoteReturnsProviderError(t *testing.T) {
	svc := newTestService(&countingProvider{err: ErrProviderUnavailable})
	_, err := svc.Quotes(context.Background(), []string{"AAPL"})
	assert.ErrorIs(t, err, ErrProviderUnavailable)
}

func TestService_ProviderSearchPopulatesRegistryAndSorts(t *testing.T) {
	provider := &instrumentProvider{items: []Instrument{
		{Symbol: "AAPL", Name: "Apple Inc.", Country: "US", Currency: "USD", AssetType: "stock", Provider: "test"},
		{Symbol: "AAP", Name: "Advance Auto Parts", Country: "US", Currency: "USD", AssetType: "stock", Provider: "test"},
	}}
	svc := newTestService(provider)

	resp, err := svc.SearchInstruments(context.Background(), "AAP")
	require.NoError(t, err)
	require.Len(t, resp.Results, 2)
	assert.Equal(t, "AAP", resp.Results[0].Symbol)

	cached, err := svc.SearchInstruments(context.Background(), "AAP")
	require.NoError(t, err)
	assert.NotEmpty(t, cached.Results)
}

func TestService_RealModeSearchPrefersProviderOverLocalCache(t *testing.T) {
	provider := &instrumentProvider{items: []Instrument{
		{Symbol: "AAPL", Name: "Apple Inc.", Country: "US", Currency: "USD", AssetType: "stock", Provider: ProviderTwelveData},
	}}
	svc := newTestService(provider)
	require.NoError(t, svc.repo.UpsertInstruments(context.Background(), []Instrument{
		{Symbol: "AAPX", Name: "Cached Placeholder", Country: "US", Currency: "USD", AssetType: "stock", Provider: "cache"},
	}))

	resp, err := svc.SearchInstruments(context.Background(), "AAP")
	require.NoError(t, err)
	require.Len(t, resp.Results, 1)
	assert.Equal(t, "provider", resp.Source)
	assert.Equal(t, "AAPL", resp.Results[0].Symbol)
}

func TestService_RealModeSearchFallsBackToCacheOnProviderFailure(t *testing.T) {
	provider := &instrumentProvider{err: ErrProviderUnavailable}
	svc := newTestService(provider)
	require.NoError(t, svc.repo.UpsertInstruments(context.Background(), []Instrument{
		{Symbol: "AAPL", Name: "Apple Inc.", Country: "US", Currency: "USD", AssetType: "stock", Provider: "cache"},
	}))

	resp, err := svc.SearchInstruments(context.Background(), "AAP")
	require.NoError(t, err)
	require.Len(t, resp.Results, 1)
	assert.Equal(t, "cache", resp.Source)
	assert.Equal(t, "AAPL", resp.Results[0].Symbol)
}

func TestService_MockModeSearchesSeededInstruments(t *testing.T) {
	svc := NewService(
		NewInMemoryRepository(),
		NewMockProvider(prices.NewMockPriceProvider()),
		ServiceConfig{RealMarketDataEnabled: false, SearchLimit: 10},
	)

	tests := []struct {
		query string
		first string
	}{
		{query: "AAP", first: "AAPL"},
		{query: "NV", first: "NVDA"},
		{query: "QQ", first: "QQQ"},
		{query: "SP", first: "SPY"},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			resp, err := svc.SearchInstruments(context.Background(), tt.query)
			require.NoError(t, err)
			require.NotEmpty(t, resp.Results)
			assert.Equal(t, tt.first, resp.Results[0].Symbol)
		})
	}
}

func TestNormalizeAssetTypeLooseProviderValues(t *testing.T) {
	assert.Equal(t, "etf", normalizeAssetType("Exchange-Traded Fund"))
	assert.Equal(t, "stock", normalizeAssetType("Common Equity"))
	assert.Equal(t, "stock", normalizeAssetType("Common Stock"))
}

type instrumentProvider struct {
	items []Instrument
	err   error
}

func (p *instrumentProvider) SearchInstruments(context.Context, string) ([]Instrument, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.items, nil
}

func (p *instrumentProvider) GetQuote(context.Context, string) (Quote, error) {
	return Quote{}, errors.New("not used")
}

func (p *instrumentProvider) GetQuotes(context.Context, []string) ([]Quote, error) {
	return nil, errors.New("not used")
}

func TestRequestLimiter_RespectsMaxRequestsPerMinute(t *testing.T) {
	limiter := NewRequestLimiter(1, 10)
	require.NoError(t, limiter.Allow())
	assert.ErrorIs(t, limiter.Allow(), ErrProviderRateLimited)
}
