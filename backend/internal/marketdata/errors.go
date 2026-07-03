package marketdata

import "errors"

var (
	ErrProviderRateLimited     = errors.New("market data provider rate limited")
	ErrProviderUnavailable     = errors.New("market data provider unavailable")
	ErrSymbolNotFound          = errors.New("symbol not found")
	ErrInvalidProviderResponse = errors.New("invalid market data provider response")
	ErrInvalidQuery            = errors.New("invalid instrument search query")
	ErrInvalidSymbols          = errors.New("invalid symbols")
	ErrDailyBudgetExhausted    = errors.New("market data daily request budget exhausted")
)
