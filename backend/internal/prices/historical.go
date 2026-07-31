package prices

import (
	"context"
	"time"
)

// Methodology labels how a HistoricalPrice was derived, so callers can judge
// how precisely it represents the requested instant instead of assuming
// every stored price is an exact boundary value.
const (
	// MethodologyExact means the provider returned a quote timestamped at or
	// within the provider's own intraday resolution of the requested instant.
	// No provider in this codebase produces this today; it exists so a future
	// intraday-capable provider has somewhere to report it.
	MethodologyExact = "exact"
	// MethodologySessionClose means only daily-bar history was available: the
	// price is the close of the trading session at or before the requested
	// instant, not the price at that instant's time of day.
	MethodologySessionClose = "session_close"
	// MethodologyFallbackLatest means no historical data was available at all
	// (an unsupported provider, or the history lookup failed) and the
	// provider's current live quote was used instead — it may postdate the
	// requested instant by an unbounded amount.
	MethodologyFallbackLatest = "fallback_latest"
)

// HistoricalPrice is a price resolved for a specific past instant, annotated
// with how precisely it represents that instant so callers never mistake a
// same-day close or a stale live quote for an exact boundary price.
type HistoricalPrice struct {
	Symbol             string
	Price              float64
	Currency           string
	ProviderTimestamp  time.Time // the provider's own timestamp for this price
	TradingSessionDate string    // YYYY-MM-DD, set when Methodology is MethodologySessionClose
	Methodology        string
}

// HistoricalPriceProvider is implemented by providers that can resolve a
// price at or before a specific past instant, rather than only "now".
// Providers that can't (the Yahoo prototype, the mock provider) simply don't
// implement it; callers type-assert and fall back to GetLatestPrice with
// MethodologyFallbackLatest.
type HistoricalPriceProvider interface {
	PriceAtOrBefore(ctx context.Context, symbol string, at time.Time) (*HistoricalPrice, error)
}
