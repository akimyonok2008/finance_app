package prices

import (
	"context"
	"sync"
	"time"
)

type mockQuote struct {
	price    float64
	currency string
}

// MockPriceProvider returns deterministic prices for tests and local
// development. It is seeded with a handful of well-known symbols and can be
// extended at runtime via Set.
type MockPriceProvider struct {
	mu     sync.RWMutex
	quotes map[string]mockQuote
}

// NewMockPriceProvider returns a provider seeded with default quotes.
func NewMockPriceProvider() *MockPriceProvider {
	return &MockPriceProvider{
		quotes: map[string]mockQuote{
			"AAPL":     {195.00, "USD"},
			"MSFT":     {430.00, "USD"},
			"NVDA":     {130.00, "USD"},
			"SPY":      {540.00, "USD"},
			"BTC-USD":  {68000.00, "USD"},
			"ETH-USD":  {3500.00, "USD"},
			"THYAO.IS": {295.00, "TRY"},
			"GARAN.IS": {120.00, "TRY"},
			"ASELS.IS": {85.00, "TRY"},
		},
	}
}

// Set adds or overrides the quote for a symbol.
func (m *MockPriceProvider) Set(symbol string, price float64, currency string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.quotes[normalizeSymbol(symbol)] = mockQuote{price: price, currency: currency}
}

// GetLatestPrice returns the seeded quote for symbol, or ErrPriceUnavailable.
func (m *MockPriceProvider) GetLatestPrice(_ context.Context, symbol string) (*Price, error) {
	sym := normalizeSymbol(symbol)

	m.mu.RLock()
	q, ok := m.quotes[sym]
	m.mu.RUnlock()
	if !ok {
		return nil, ErrPriceUnavailable
	}

	return &Price{
		Symbol:    sym,
		Price:     q.price,
		Currency:  q.currency,
		Timestamp: time.Now().UTC(),
		Source:    "mock",
	}, nil
}
