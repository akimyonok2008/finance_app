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

	"github.com/ardakimyonok/finance_app/internal/prices"
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

func TestPriceAtOrBefore_UsesSessionCloseAtOrBeforeCutoff(t *testing.T) {
	provider, _ := newHistoryProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(timeSeriesBody))
	})

	// Cutoff falls mid-day on 2026-01-04, hours after that session's close:
	// the result must be the 2026-01-04 close, not 2026-01-05's (which is
	// still in the future relative to the cutoff) or a live quote.
	cutoff := mustDate("2026-01-04").Add(20 * time.Hour)
	got, err := provider.PriceAtOrBefore(context.Background(), "SPY", cutoff)
	require.NoError(t, err)
	assert.Equal(t, "2026-01-04", got.TradingSessionDate)
	assert.Equal(t, 105.0, got.Price)
	assert.Equal(t, prices.MethodologySessionClose, got.Methodology)
	assert.Equal(t, mustDate("2026-01-04"), got.ProviderTimestamp)
}

func TestPriceAtOrBefore_FallsBackToPriorSessionOnNonTradingDay(t *testing.T) {
	provider, _ := newHistoryProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(timeSeriesBody))
	})

	// No bar exists for 2026-01-06 (a gap, e.g. weekend/holiday): the nearest
	// PRIOR session's close must be used, never a later one.
	got, err := provider.PriceAtOrBefore(context.Background(), "SPY", mustDate("2026-01-06"))
	require.NoError(t, err)
	assert.Equal(t, "2026-01-05", got.TradingSessionDate)
	assert.Equal(t, 110.0, got.Price)
}

func TestPriceAtOrBefore_ErrorsWhenCutoffPredatesAllHistory(t *testing.T) {
	provider, _ := newHistoryProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(timeSeriesBody))
	})

	_, err := provider.PriceAtOrBefore(context.Background(), "SPY", mustDate("2025-12-31"))
	require.Error(t, err)
}

func TestPriceAtOrBefore_ParsesCurrencyFromMeta(t *testing.T) {
	body := `{
	  "meta": {"symbol": "THYAO.IS", "interval": "1day", "currency": "try"},
	  "values": [{"datetime": "2026-01-05", "close": "295.50"}],
	  "status": "ok"
	}`
	provider, _ := newHistoryProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	})

	got, err := provider.PriceAtOrBefore(context.Background(), "THYAO.IS", mustDate("2026-01-05"))
	require.NoError(t, err)
	assert.Equal(t, "TRY", got.Currency)
}

func mustDate(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}
