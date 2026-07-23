package marketdata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const timeSeriesBody = `{
  "meta": {"symbol": "SPY", "interval": "1day"},
  "values": [
    {"datetime": "2026-01-05", "close": "110.00"},
    {"datetime": "2026-01-04", "close": "105.00"},
    {"datetime": "2026-01-03", "close": "100.00"}
  ],
  "status": "ok"
}`

func newHistoryProvider(t *testing.T, handler http.HandlerFunc) (*TwelveDataHistoryProvider, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	td, err := NewTwelveDataProvider(TwelveDataConfig{
		APIKey: "test-key", BaseURL: server.URL,
		Timeout: 5 * time.Second, MaxPerMinute: 100, DailyBudget: 1000,
	})
	require.NoError(t, err)
	return NewTwelveDataHistoryProvider(td, time.Hour), server
}

func TestHistoryProviderParsesAndSortsAscending(t *testing.T) {
	provider, _ := newHistoryProvider(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/time_series", r.URL.Path)
		assert.Equal(t, "1day", r.URL.Query().Get("interval"))
		_, _ = w.Write([]byte(timeSeriesBody))
	})

	bars, err := provider.DailySeries(context.Background(), "SPY",
		mustDate("2026-01-01"), mustDate("2026-01-31"))
	require.NoError(t, err)
	require.Len(t, bars, 3)
	assert.Equal(t, "2026-01-03", bars[0].Date)
	assert.Equal(t, 100.0, bars[0].Close)
	assert.Equal(t, "2026-01-05", bars[2].Date)
	assert.Equal(t, 110.0, bars[2].Close)
}

func TestHistoryProviderFiltersToWindow(t *testing.T) {
	provider, _ := newHistoryProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(timeSeriesBody))
	})

	bars, err := provider.DailySeries(context.Background(), "SPY",
		mustDate("2026-01-04"), mustDate("2026-01-05"))
	require.NoError(t, err)
	require.Len(t, bars, 2)
	assert.Equal(t, "2026-01-04", bars[0].Date)
	assert.Equal(t, "2026-01-05", bars[1].Date)
}

func TestHistoryProviderCachesFetch(t *testing.T) {
	var calls int32
	provider, _ := newHistoryProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		_, _ = w.Write([]byte(timeSeriesBody))
	})

	_, err := provider.DailySeries(context.Background(), "SPY", mustDate("2026-01-01"), mustDate("2026-01-31"))
	require.NoError(t, err)
	// A different sub-range for the same symbol must reuse the cached fetch.
	_, err = provider.DailySeries(context.Background(), "SPY", mustDate("2026-01-04"), mustDate("2026-01-05"))
	require.NoError(t, err)

	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))
}

func mustDate(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}
