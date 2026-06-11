package prices

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ErrPriceUnavailable is returned when a provider has no price for a symbol.
var ErrPriceUnavailable = errors.New("price unavailable for symbol")

// ErrInvalidSymbolFormat is returned when a symbol is empty, too long, or
// contains characters outside the safe set [A-Z0-9.-].
var ErrInvalidSymbolFormat = errors.New("invalid symbol format")

// maxSymbolLength caps symbol length to reject obviously malformed input.
const maxSymbolLength = 20

// safeSymbol matches uppercase letters, digits, dot, and dash only.
var safeSymbol = regexp.MustCompile(`^[A-Z0-9.\-]+$`)

// ValidateAndNormalizeSymbol trims and upper-cases a symbol and enforces the
// safe-character/length rules. It does NOT check priceability — callers combine
// it with GetLatestPrice for that.
func ValidateAndNormalizeSymbol(symbol string) (string, error) {
	s := normalizeSymbol(symbol)
	if s == "" || len(s) > maxSymbolLength || !safeSymbol.MatchString(s) {
		return "", ErrInvalidSymbolFormat
	}
	return s, nil
}

// PriceProvider is the abstraction over market-data sources. All business logic
// depends on this interface, never on a concrete client, so the Yahoo prototype
// provider can later be swapped for Twelve Data, Finnhub, or a licensed feed
// without touching the portfolio service.
type PriceProvider interface {
	GetLatestPrice(ctx context.Context, symbol string) (*Price, error)
}

// NewProvider builds a provider by name. Allowed values: "mock", "yahoo".
func NewProvider(name string) (PriceProvider, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "mock":
		return NewMockPriceProvider(), nil
	case "yahoo":
		return NewYahooFinanceProvider(), nil
	default:
		return nil, fmt.Errorf("unknown price provider %q (allowed: mock, yahoo)", name)
	}
}

// normalizeSymbol upper-cases and trims a symbol for consistent lookups.
func normalizeSymbol(symbol string) string {
	return strings.ToUpper(strings.TrimSpace(symbol))
}
