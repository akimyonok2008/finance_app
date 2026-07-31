package marketdata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestTwelveProvider(t *testing.T, handler http.HandlerFunc) (*TwelveDataProvider, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	p, err := NewTwelveDataProvider(TwelveDataConfig{
		APIKey: "test-key", BaseURL: srv.URL,
		Timeout: time.Second, CacheTTL: 10 * time.Minute,
		MaxPerMinute: 100, DailyBudget: 100,
	})
	require.NoError(t, err)
	return p, srv
}

func TestTwelveDataProvider_GetQuoteSuccess(t *testing.T) {
	p, srv := newTestTwelveProvider(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/quote", r.URL.Path)
		assert.Equal(t, "AAPL", r.URL.Query().Get("symbol"))
		_, _ = w.Write([]byte(`{
			"symbol":"AAPL","close":"195.12","currency":"USD",
			"percent_change":"0.82","previous_close":"193.54",
			"datetime":"2026-06-29 14:31:00"
		}`))
	})
	defer srv.Close()

	q, err := p.GetQuote(context.Background(), "aapl")
	require.NoError(t, err)
	assert.Equal(t, "AAPL", q.Symbol)
	assert.Equal(t, 195.12, q.Price)
	assert.Equal(t, "USD", q.Currency)
	assert.Equal(t, ProviderTwelveData, q.Provider)
	require.NotNil(t, q.ChangePercentage)
	assert.Equal(t, 0.82, *q.ChangePercentage)
	require.NotNil(t, q.MarketTime)
}

func TestTwelveDataProvider_BatchQuoteSuccess(t *testing.T) {
	p, srv := newTestTwelveProvider(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "AAPL,MSFT", r.URL.Query().Get("symbol"))
		_, _ = w.Write([]byte(`{
			"AAPL":{"symbol":"AAPL","close":"195","currency":"USD"},
			"MSFT":{"symbol":"MSFT","close":"430","currency":"USD"}
		}`))
	})
	defer srv.Close()

	quotes, err := p.GetQuotes(context.Background(), []string{"AAPL", "MSFT", "AAPL"})
	require.NoError(t, err)
	require.Len(t, quotes, 2)
	bySymbol := make(map[string]Quote, len(quotes))
	for _, q := range quotes {
		bySymbol[q.Symbol] = q
	}
	require.Contains(t, bySymbol, "AAPL")
	require.Contains(t, bySymbol, "MSFT")
	assert.Equal(t, "AAPL", bySymbol["AAPL"].Symbol)
	assert.Equal(t, "MSFT", bySymbol["MSFT"].Symbol)
}

func TestTwelveDataProvider_MalformedQuote(t *testing.T) {
	p, srv := newTestTwelveProvider(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"symbol":"AAPL","close":"not-a-number"}`))
	})
	defer srv.Close()

	_, err := p.GetQuote(context.Background(), "AAPL")
	assert.ErrorIs(t, err, ErrInvalidProviderResponse)
}

func TestTwelveDataProvider_RateLimited(t *testing.T) {
	p, srv := newTestTwelveProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"status":"error","message":"rate limit"}`))
	})
	defer srv.Close()

	_, err := p.GetQuote(context.Background(), "AAPL")
	assert.ErrorIs(t, err, ErrProviderRateLimited)
}

func TestTwelveDataProvider_SearchInstruments(t *testing.T) {
	p, srv := newTestTwelveProvider(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/symbol_search", r.URL.Path)
		assert.Equal(t, "AAP", r.URL.Query().Get("symbol"))
		assert.Empty(t, r.URL.Query().Get("q"))
		_, _ = w.Write([]byte(`{"data":[
			{"symbol":"AAPL","instrument_name":"Apple Inc.","exchange":"NASDAQ","currency":"USD","country":"United States","type":"Common Stock"},
			{"symbol":"AAP","instrument_name":"Advance Auto Parts","exchange":"NYSE","currency":"USD","country":"United States","type":"Common Stock"},
			{"symbol":"EUR/USD","instrument_name":"Euro Dollar","exchange":"FX","currency":"USD","country":"United States","type":"Forex"}
		]}`))
	})
	defer srv.Close()

	results, err := p.SearchInstruments(context.Background(), "AAP")
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "AAP", results[0].Symbol)
	assert.Equal(t, "AAPL", results[1].Symbol)
}

func TestTwelveDataProvider_SearchKeepsLooseStockAndETFResults(t *testing.T) {
	p, srv := newTestTwelveProvider(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[
			{"symbol":"AAPL","instrument_name":"Apple Inc.","exchange":"NASDAQ","currency":"","country":"","type":"Common Equity"},
			{"symbol":"QQQ","instrument_name":"Invesco QQQ Trust","exchange":"NASDAQ","currency":"USD","country":"","type":""},
			{"symbol":"AAPL:EUR","instrument_name":"Apple foreign line","exchange":"XETRA","currency":"EUR","country":"Germany","type":"Common Equity"}
		]}`))
	})
	defer srv.Close()

	results, err := p.SearchInstruments(context.Background(), "A")
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "AAPL", results[0].Symbol)
	assert.Equal(t, "stock", results[0].AssetType)
	assert.Equal(t, "USD", results[0].Currency)
	assert.Equal(t, "US", results[0].Country)
	assert.Equal(t, "QQQ", results[1].Symbol)
	assert.Equal(t, "etf", results[1].AssetType)
}

func TestTwelveDataProvider_EmptySearch(t *testing.T) {
	p, srv := newTestTwelveProvider(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	})
	defer srv.Close()

	results, err := p.SearchInstruments(context.Background(), "ZZZ")
	require.NoError(t, err)
	assert.Empty(t, results)
}
