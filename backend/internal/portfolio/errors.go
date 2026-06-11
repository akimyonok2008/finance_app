package portfolio

import "errors"

// Domain errors. Handlers map these to HTTP status codes.
var (
	// ErrSymbolRequired is returned when a position has no symbol.
	ErrSymbolRequired = errors.New("symbol is required")
	// ErrInvalidAssetType is returned for an asset_type outside stock/etf/crypto.
	ErrInvalidAssetType = errors.New("asset_type must be one of: stock, etf, crypto")
	// ErrInvalidQuantity is returned when quantity is not greater than zero.
	ErrInvalidQuantity = errors.New("quantity must be greater than 0")
	// ErrInvalidPrice is returned when average_buy_price is not greater than zero.
	ErrInvalidPrice = errors.New("average_buy_price must be greater than 0")
	// ErrCurrencyRequired is returned when currency is missing.
	ErrCurrencyRequired = errors.New("currency is required")
	// ErrUnsupportedSymbol is returned when a symbol has an invalid format or
	// cannot be priced by the active provider. Maps to HTTP 400.
	ErrUnsupportedSymbol = errors.New("unsupported or unpriceable symbol")
	// ErrUnsupportedCurrency is returned when a position's currency cannot be
	// converted to the base currency. Maps to HTTP 400.
	ErrUnsupportedCurrency = errors.New("unsupported currency")

	// ErrPositionNotFound is returned when a position does not exist OR does not
	// belong to the requesting user. The same error is used for both cases so
	// the API never reveals the existence of another user's positions.
	ErrPositionNotFound = errors.New("position not found")
	// ErrPortfolioNotFound is an internal repository signal that a user has no
	// portfolio yet; the service responds by creating the default portfolio.
	ErrPortfolioNotFound = errors.New("portfolio not found")

	// ErrPriceProvider wraps any failure from the price provider so the handler
	// can respond with 502 Bad Gateway.
	ErrPriceProvider = errors.New("price provider error")
)
