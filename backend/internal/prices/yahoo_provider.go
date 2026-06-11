package prices

import (
	"context"
	"fmt"
	"time"

	"github.com/piquette/finance-go/quote"
)

// YahooFinanceProvider fetches real prices via the finance-go Yahoo client.
//
// PROTOTYPE ONLY: finance-go scrapes an unofficial, unauthenticated Yahoo
// endpoint. It is not suitable for production and must be replaced by a
// licensed market-data provider (Twelve Data, Finnhub, etc.). This is the only
// file in the codebase permitted to import finance-go; business logic depends
// solely on the PriceProvider interface.
type YahooFinanceProvider struct{}

// NewYahooFinanceProvider returns a Yahoo-backed price provider.
func NewYahooFinanceProvider() *YahooFinanceProvider {
	return &YahooFinanceProvider{}
}

// GetLatestPrice fetches the latest regular-market price for symbol. Any
// failure is wrapped with a clear message; a missing quote yields
// ErrPriceUnavailable.
func (y *YahooFinanceProvider) GetLatestPrice(_ context.Context, symbol string) (*Price, error) {
	sym := normalizeSymbol(symbol)

	q, err := quote.Get(sym)
	if err != nil {
		return nil, fmt.Errorf("yahoo finance lookup for %q failed: %w", sym, err)
	}
	if q == nil {
		return nil, fmt.Errorf("%w: %s", ErrPriceUnavailable, sym)
	}

	return &Price{
		Symbol:    sym,
		Price:     q.RegularMarketPrice,
		Currency:  q.CurrencyID,
		Timestamp: time.Now().UTC(),
		Source:    "yahoo",
	}, nil
}
