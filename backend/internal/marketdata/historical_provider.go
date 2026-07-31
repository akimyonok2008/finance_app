package marketdata

import (
	"context"
	"encoding/json"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ardakimyonok/finance_app/internal/prices"
)

// DailyBar is a single daily close observation (adjusted where the provider
// plan supplies it).
type DailyBar struct {
	Date  string // YYYY-MM-DD
	Close float64
}

// TwelveDataHistoryProvider fetches daily close series from Twelve Data's
// /time_series endpoint. To keep API usage low it fetches a wide window once per
// symbol, caches it in memory, and serves any sub-range by filtering — so a full
// benchmark evaluation costs at most one request per unique symbol per cache TTL.
type TwelveDataHistoryProvider struct {
	td         *TwelveDataProvider
	ttl        time.Duration
	outputSize int

	mu    sync.Mutex
	cache map[string]historyEntry
}

type historyEntry struct {
	bars      []DailyBar // ascending by date
	currency  string
	fetchedAt time.Time
}

// NewTwelveDataHistoryProvider wraps an existing Twelve Data client. ttl bounds
// how long a fetched series is reused (a few hours is plenty for daily bars).
func NewTwelveDataHistoryProvider(td *TwelveDataProvider, ttl time.Duration) *TwelveDataHistoryProvider {
	if ttl <= 0 {
		ttl = 6 * time.Hour
	}
	return &TwelveDataHistoryProvider{
		td:         td,
		ttl:        ttl,
		outputSize: 400, // ~1.5 trading years, covers the longest (1Y) badge window
		cache:      make(map[string]historyEntry),
	}
}

// DailySeries returns the daily close bars for symbol within [start, end]
// inclusive, sorted ascending by date.
func (h *TwelveDataHistoryProvider) DailySeries(ctx context.Context, symbol string, start, end time.Time) ([]DailyBar, error) {
	sym := normalizeSymbol(symbol)
	bars, _, err := h.fullSeries(ctx, sym)
	if err != nil {
		return nil, err
	}
	startKey := start.UTC().Format("2006-01-02")
	endKey := end.UTC().Format("2006-01-02")
	out := make([]DailyBar, 0, len(bars))
	for _, bar := range bars {
		if bar.Date >= startKey && bar.Date <= endKey {
			out = append(out, bar)
		}
	}
	return out, nil
}

// PriceAtOrBefore resolves symbol's price as the daily session close at or
// before at, the finest resolution this provider has: Twelve Data's
// /time_series is only queried here at 1-day granularity (see fetch), so the
// result is always tagged MethodologySessionClose, never MethodologyExact —
// it is the close of that trading session, not the price at at's specific
// time of day. Returns ErrSymbolNotFound if no session on or before at has a
// bar (e.g. at predates the symbol's history).
func (h *TwelveDataHistoryProvider) PriceAtOrBefore(ctx context.Context, symbol string, at time.Time) (*prices.HistoricalPrice, error) {
	sym := normalizeSymbol(symbol)
	bars, currency, err := h.fullSeries(ctx, sym)
	if err != nil {
		return nil, err
	}
	cutoff := at.UTC().Format("2006-01-02")
	var match DailyBar
	found := false
	for _, bar := range bars {
		if bar.Date > cutoff {
			break
		}
		match = bar
		found = true
	}
	if !found {
		return nil, ErrSymbolNotFound
	}
	sessionClose, parseErr := time.Parse("2006-01-02", match.Date)
	if parseErr != nil {
		sessionClose = at.UTC()
	}
	return &prices.HistoricalPrice{
		Symbol: sym, Price: match.Close, Currency: currency,
		ProviderTimestamp: sessionClose.UTC(), TradingSessionDate: match.Date,
		Methodology: prices.MethodologySessionClose,
	}, nil
}

// fullSeries returns the cached wide series and currency for a symbol,
// fetching it when the cache is empty or stale.
func (h *TwelveDataHistoryProvider) fullSeries(ctx context.Context, symbol string) ([]DailyBar, string, error) {
	h.mu.Lock()
	entry, ok := h.cache[symbol]
	if ok && time.Since(entry.fetchedAt) < h.ttl {
		h.mu.Unlock()
		return entry.bars, entry.currency, nil
	}
	h.mu.Unlock()

	bars, currency, err := h.fetch(ctx, symbol)
	if err != nil {
		// Serve a stale-but-usable cached series rather than failing evaluation.
		h.mu.Lock()
		if ok && len(entry.bars) > 0 {
			h.mu.Unlock()
			return entry.bars, entry.currency, nil
		}
		h.mu.Unlock()
		return nil, "", err
	}

	h.mu.Lock()
	h.cache[symbol] = historyEntry{bars: bars, currency: currency, fetchedAt: time.Now().UTC()}
	h.mu.Unlock()
	return bars, currency, nil
}

func (h *TwelveDataHistoryProvider) fetch(ctx context.Context, symbol string) ([]DailyBar, string, error) {
	if err := h.td.limiter.Allow(); err != nil {
		return nil, "", err
	}
	u, _ := url.Parse(h.td.baseURL + "/time_series")
	q := u.Query()
	q.Set("symbol", symbol)
	q.Set("interval", "1day")
	q.Set("outputsize", strconv.Itoa(h.outputSize))
	q.Set("order", "ASC")
	q.Set("apikey", h.td.apiKey)
	u.RawQuery = q.Encode()

	body, err := h.td.get(ctx, u.String())
	if err != nil {
		return nil, "", err
	}

	var resp struct {
		Status  string `json:"status"`
		Message string `json:"message"`
		Meta    struct {
			Currency string `json:"currency"`
		} `json:"meta"`
		Values []struct {
			Datetime string `json:"datetime"`
			Close    string `json:"close"`
		} `json:"values"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, "", ErrInvalidProviderResponse
	}
	if resp.Status == "error" {
		return nil, "", providerError(resp.Message)
	}

	bars := make([]DailyBar, 0, len(resp.Values))
	for _, v := range resp.Values {
		closePx, ok := numberFromAny(v.Close)
		if !ok || closePx <= 0 {
			continue
		}
		date := v.Datetime
		if len(date) > 10 {
			date = date[:10] // normalize any datetime to a day key
		}
		bars = append(bars, DailyBar{Date: date, Close: closePx})
	}
	if len(bars) == 0 {
		return nil, "", ErrSymbolNotFound
	}
	sort.Slice(bars, func(i, j int) bool { return bars[i].Date < bars[j].Date })
	currency := strings.ToUpper(strings.TrimSpace(resp.Meta.Currency))
	if currency == "" {
		currency = "USD"
	}
	return bars, currency, nil
}
