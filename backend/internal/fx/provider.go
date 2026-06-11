package fx

import (
	"context"
	"errors"
)

// ErrUnsupportedCurrency is returned when a currency has no known rate.
var ErrUnsupportedCurrency = errors.New("unsupported currency")

// FXProvider converts monetary amounts between currencies. Business logic
// depends only on this interface, so the mock can later be replaced by a live
// FX feed.
type FXProvider interface {
	Convert(ctx context.Context, amount float64, fromCurrency, toCurrency string) (float64, error)
	GetRate(ctx context.Context, fromCurrency, toCurrency string) (float64, error)
}
