package fx

import (
	"context"
	"strings"
)

// MockFXProvider holds deterministic prototype rates expressed as "1 unit of the
// currency = N USD". Conversion between any two supported currencies is derived
// from their USD rates.
type MockFXProvider struct {
	ratesToUSD map[string]float64
}

// NewMockFXProvider returns a provider seeded with the prototype rates.
func NewMockFXProvider() *MockFXProvider {
	return &MockFXProvider{ratesToUSD: map[string]float64{
		"USD": 1.0,
		"TRY": 0.031,
		"EUR": 1.08,
		"GBP": 1.27,
	}}
}

func normalizeCurrency(c string) string { return strings.ToUpper(strings.TrimSpace(c)) }

// GetRate returns the multiplier to convert an amount in fromCurrency into
// toCurrency: rate = (from→USD) / (to→USD).
func (m *MockFXProvider) GetRate(_ context.Context, fromCurrency, toCurrency string) (float64, error) {
	from := normalizeCurrency(fromCurrency)
	to := normalizeCurrency(toCurrency)
	rf, ok := m.ratesToUSD[from]
	if !ok {
		return 0, ErrUnsupportedCurrency
	}
	rt, ok := m.ratesToUSD[to]
	if !ok {
		return 0, ErrUnsupportedCurrency
	}
	return rf / rt, nil
}

// Convert converts amount from one currency to another.
func (m *MockFXProvider) Convert(ctx context.Context, amount float64, fromCurrency, toCurrency string) (float64, error) {
	rate, err := m.GetRate(ctx, fromCurrency, toCurrency)
	if err != nil {
		return 0, err
	}
	return amount * rate, nil
}
