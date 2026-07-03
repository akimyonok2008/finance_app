package marketdata

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

type Repository interface {
	SearchInstruments(ctx context.Context, query string, limit int) ([]Instrument, error)
	UpsertInstruments(ctx context.Context, instruments []Instrument) error
	GetQuote(ctx context.Context, symbol string) (Quote, bool, error)
	UpsertQuotes(ctx context.Context, quotes []Quote) error
}

type InMemoryRepository struct {
	mu          sync.RWMutex
	instruments map[string]Instrument
	quotes      map[string]Quote
}

func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		instruments: make(map[string]Instrument),
		quotes:      make(map[string]Quote),
	}
}

func (r *InMemoryRepository) SearchInstruments(_ context.Context, query string, limit int) ([]Instrument, error) {
	q := strings.ToUpper(strings.TrimSpace(query))
	r.mu.RLock()
	out := make([]Instrument, 0)
	for _, instrument := range r.instruments {
		if matchesInstrument(instrument, q) {
			out = append(out, instrument)
		}
	}
	r.mu.RUnlock()
	sortInstruments(out, q)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *InMemoryRepository) UpsertInstruments(_ context.Context, instruments []Instrument) error {
	now := time.Now().UTC()
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, item := range instruments {
		item = normalizeInstrument(item, now)
		if existing, ok := r.instruments[item.Symbol]; ok && !existing.CreatedAt.IsZero() {
			item.CreatedAt = existing.CreatedAt
		}
		r.instruments[item.Symbol] = item
	}
	return nil
}

func (r *InMemoryRepository) GetQuote(_ context.Context, symbol string) (Quote, bool, error) {
	r.mu.RLock()
	q, ok := r.quotes[normalizeSymbol(symbol)]
	r.mu.RUnlock()
	return q, ok, nil
}

func (r *InMemoryRepository) UpsertQuotes(_ context.Context, quotes []Quote) error {
	now := time.Now().UTC()
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, quote := range quotes {
		quote.Symbol = normalizeSymbol(quote.Symbol)
		quote.UpdatedAt = now
		r.quotes[quote.Symbol] = quote
	}
	return nil
}

func normalizeInstrument(item Instrument, now time.Time) Instrument {
	item.Symbol = normalizeSymbol(item.Symbol)
	if item.DisplaySymbol == "" {
		item.DisplaySymbol = item.Symbol
	}
	item.ProviderSymbol = normalizeSymbol(firstNonEmpty(item.ProviderSymbol, item.Symbol))
	item.AssetType = normalizeAssetType(item.AssetType)
	item.Country = strings.ToUpper(strings.TrimSpace(item.Country))
	item.Currency = strings.ToUpper(strings.TrimSpace(item.Currency))
	item.Exchange = strings.ToUpper(strings.TrimSpace(item.Exchange))
	item.Provider = strings.ToLower(strings.TrimSpace(item.Provider))
	item.IsActive = true
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	item.UpdatedAt = now
	seen := now
	item.LastSeenAt = &seen
	return item
}

func matchesInstrument(item Instrument, query string) bool {
	return strings.Contains(strings.ToUpper(item.Symbol), query) ||
		strings.Contains(strings.ToUpper(item.DisplaySymbol), query) ||
		strings.Contains(strings.ToUpper(item.Name), query)
}

func sortInstruments(items []Instrument, query string) {
	sort.SliceStable(items, func(i, j int) bool {
		a := rankInstrument(items[i], query)
		b := rankInstrument(items[j], query)
		if a != b {
			return a < b
		}
		return items[i].Symbol < items[j].Symbol
	})
}

func rankInstrument(item Instrument, query string) int {
	s := strings.ToUpper(item.Symbol)
	n := strings.ToUpper(item.Name)
	switch {
	case s == query:
		return 0
	case strings.HasPrefix(s, query):
		return 1
	case strings.HasPrefix(n, query):
		return 2
	case strings.Contains(s, query):
		return 3
	case strings.Contains(n, query):
		return 4
	default:
		return 5
	}
}

func normalizeSymbol(symbol string) string {
	return strings.ToUpper(strings.TrimSpace(symbol))
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func normalizeAssetType(v string) string {
	value := strings.ToLower(strings.TrimSpace(v))
	switch {
	case strings.Contains(value, "etf") || strings.Contains(value, "exchange-traded") || strings.Contains(value, "exchange traded"):
		return "etf"
	case strings.Contains(value, "stock") || strings.Contains(value, "common") || strings.Contains(value, "equity"):
		return "stock"
	case strings.Contains(value, "fund"):
		return "fund"
	case strings.Contains(value, "crypto") || strings.Contains(value, "digital currency"):
		return "crypto"
	case strings.Contains(value, "forex") || value == "fx" || strings.Contains(value, "currency"):
		return "forex"
	default:
		return "other"
	}
}
