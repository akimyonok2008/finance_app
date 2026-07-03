package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type TwelveDataProvider struct {
	apiKey  string
	baseURL string
	client  *http.Client
	limiter *RequestLimiter
	ttl     time.Duration
}

type TwelveDataConfig struct {
	APIKey       string
	BaseURL      string
	Timeout      time.Duration
	CacheTTL     time.Duration
	MaxPerMinute int
	DailyBudget  int
}

func NewTwelveDataProvider(cfg TwelveDataConfig) (*TwelveDataProvider, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("TWELVE_DATA_API_KEY is required when PRICE_PROVIDER=twelvedata")
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.twelvedata.com"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = 10 * time.Minute
	}
	return &TwelveDataProvider{
		apiKey: cfg.APIKey, baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		client:  &http.Client{Timeout: cfg.Timeout},
		limiter: NewRequestLimiter(cfg.MaxPerMinute, cfg.DailyBudget),
		ttl:     cfg.CacheTTL,
	}, nil
}

func (p *TwelveDataProvider) SearchInstruments(ctx context.Context, query string) ([]Instrument, error) {
	if err := p.limiter.Allow(); err != nil {
		return nil, err
	}
	u, _ := url.Parse(p.baseURL + "/symbol_search")
	q := u.Query()
	q.Set("symbol", strings.TrimSpace(query))
	q.Set("apikey", p.apiKey)
	u.RawQuery = q.Encode()

	body, err := p.get(ctx, u.String())
	if err != nil {
		return nil, err
	}
	var resp struct {
		Status  string `json:"status"`
		Message string `json:"message"`
		Data    []struct {
			Symbol         string `json:"symbol"`
			InstrumentName string `json:"instrument_name"`
			Exchange       string `json:"exchange"`
			Currency       string `json:"currency"`
			Country        string `json:"country"`
			Type           string `json:"type"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, ErrInvalidProviderResponse
	}
	if strings.EqualFold(resp.Status, "error") {
		return nil, providerError(resp.Message)
	}
	out := make([]Instrument, 0, len(resp.Data))
	for _, item := range resp.Data {
		assetType := normalizeAssetType(item.Type)
		country := normalizeCountry(item.Country)
		if country != "" && country != "US" {
			continue
		}
		if assetType == "other" {
			assetType = inferAssetType(item.Symbol, item.InstrumentName, item.Exchange)
		}
		if assetType != "stock" && assetType != "etf" {
			continue
		}
		out = append(out, Instrument{
			Symbol: normalizeSymbol(item.Symbol), DisplaySymbol: normalizeSymbol(item.Symbol),
			Name: item.InstrumentName, Exchange: item.Exchange, Country: firstNonEmpty(country, "US"),
			Currency: strings.ToUpper(firstNonEmpty(item.Currency, "USD")), AssetType: assetType,
			Provider: ProviderTwelveData, ProviderSymbol: normalizeSymbol(item.Symbol),
		})
	}
	sortInstruments(out, strings.ToUpper(strings.TrimSpace(query)))
	return out, nil
}

func (p *TwelveDataProvider) GetQuote(ctx context.Context, symbol string) (Quote, error) {
	quotes, err := p.GetQuotes(ctx, []string{symbol})
	if err != nil {
		return Quote{}, err
	}
	if len(quotes) == 0 {
		return Quote{}, ErrSymbolNotFound
	}
	return quotes[0], nil
}

func (p *TwelveDataProvider) GetQuotes(ctx context.Context, symbols []string) ([]Quote, error) {
	normalized := dedupeSymbols(symbols)
	if len(normalized) == 0 {
		return nil, ErrInvalidSymbols
	}
	if err := p.limiter.Allow(); err != nil {
		return nil, err
	}
	u, _ := url.Parse(p.baseURL + "/quote")
	q := u.Query()
	q.Set("symbol", strings.Join(normalized, ","))
	q.Set("apikey", p.apiKey)
	u.RawQuery = q.Encode()

	body, err := p.get(ctx, u.String())
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if len(normalized) == 1 {
		quote, err := parseTwelveQuote(normalized[0], body, now, p.ttl)
		if err != nil {
			return nil, err
		}
		return []Quote{quote}, nil
	}
	var batch map[string]json.RawMessage
	if err := json.Unmarshal(body, &batch); err != nil {
		return nil, ErrInvalidProviderResponse
	}
	out := make([]Quote, 0, len(batch))
	for symbol, raw := range batch {
		quote, err := parseTwelveQuote(symbol, raw, now, p.ttl)
		if err != nil {
			if err == ErrSymbolNotFound {
				continue
			}
			return nil, err
		}
		out = append(out, quote)
	}
	if len(out) == 0 {
		return nil, ErrSymbolNotFound
	}
	return out, nil
}

func (p *TwelveDataProvider) get(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, ErrProviderUnavailable
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusTooManyRequests {
		p.limiter.Backoff(2 * time.Minute)
		return nil, ErrProviderRateLimited
	}
	if resp.StatusCode >= 500 {
		return nil, ErrProviderUnavailable
	}
	if resp.StatusCode >= 400 {
		return nil, ErrInvalidProviderResponse
	}
	return body, nil
}

func parseTwelveQuote(symbol string, raw []byte, now time.Time, ttl time.Duration) (Quote, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return Quote{}, ErrInvalidProviderResponse
	}
	if status, _ := payload["status"].(string); strings.EqualFold(status, "error") {
		msg, _ := payload["message"].(string)
		return Quote{}, providerError(msg)
	}
	price, ok := numberFromAny(payload["close"])
	if !ok || price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
		return Quote{}, ErrInvalidProviderResponse
	}
	change, hasChange := numberFromAny(payload["percent_change"])
	prev, hasPrev := numberFromAny(payload["previous_close"])
	currency, _ := payload["currency"].(string)
	if currency == "" {
		currency = "USD"
	}
	marketTime := parseMarketTime(payload)
	rawSymbol, _ := payload["symbol"].(string)
	if rawSymbol == "" {
		rawSymbol = symbol
	}
	return Quote{
		Symbol: normalizeSymbol(firstNonEmpty(rawSymbol, symbol)), Price: price,
		Currency: strings.ToUpper(currency), ChangePercentage: optionalFloat(change, hasChange),
		PreviousClose: optionalFloat(prev, hasPrev), MarketTime: marketTime,
		Provider: ProviderTwelveData, ProviderStatus: StatusOK,
		FetchedAt: now, ExpiresAt: now.Add(ttl), RawProviderSymbol: normalizeSymbol(rawSymbol),
		UpdatedAt: now,
	}, nil
}

func providerError(message string) error {
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "rate") || strings.Contains(lower, "limit"):
		return ErrProviderRateLimited
	case strings.Contains(lower, "not found") || strings.Contains(lower, "symbol"):
		return ErrSymbolNotFound
	default:
		return ErrProviderUnavailable
	}
}

func numberFromAny(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case string:
		n, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return n, err == nil
	default:
		return 0, false
	}
}

func optionalFloat(v float64, ok bool) *float64 {
	if !ok {
		return nil
	}
	return &v
}

func parseMarketTime(payload map[string]any) *time.Time {
	for _, key := range []string{"datetime", "timestamp"} {
		if raw, ok := payload[key].(string); ok && raw != "" {
			if t, err := time.Parse(time.RFC3339, raw); err == nil {
				return &t
			}
			if t, err := time.Parse("2006-01-02 15:04:05", raw); err == nil {
				utc := t.UTC()
				return &utc
			}
		}
	}
	return nil
}

func normalizeCountry(v string) string {
	v = strings.ToUpper(strings.TrimSpace(v))
	if v == "UNITED STATES" || v == "USA" {
		return "US"
	}
	return v
}

func inferAssetType(symbol, name, exchange string) string {
	text := strings.ToLower(symbol + " " + name + " " + exchange)
	switch {
	case strings.Contains(text, "etf") || strings.Contains(text, "trust") || strings.Contains(text, "fund"):
		return "etf"
	case safeUSSymbol(symbol):
		return "stock"
	default:
		return "other"
	}
}

func safeUSSymbol(symbol string) bool {
	s := normalizeSymbol(symbol)
	if s == "" || strings.Contains(s, "/") || strings.Contains(s, ":") || strings.Contains(s, "-") {
		return false
	}
	for _, r := range s {
		if (r < 'A' || r > 'Z') && r != '.' {
			return false
		}
	}
	return true
}

func dedupeSymbols(symbols []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		s := normalizeSymbol(symbol)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
