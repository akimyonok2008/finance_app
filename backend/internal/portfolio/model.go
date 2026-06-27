package portfolio

import "time"

// Asset types accepted for a position.
const (
	AssetTypeStock  = "stock"
	AssetTypeETF    = "etf"
	AssetTypeCrypto = "crypto"
)

// DefaultPortfolioName is the name given to the auto-created portfolio.
const DefaultPortfolioName = "Default Portfolio"

// Portfolio groups a user's positions. For this milestone each user has a
// single default portfolio.
type Portfolio struct {
	ID        string
	UserID    string
	Name      string
	Currency  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Position is a single manually-entered holding. AverageBuyPrice is the LOCKED
// BASELINE PRICE: it is set by the backend to the current market price at add
// time and can never be supplied or edited by the client. All performance
// (index starting at 100) is measured from this baseline, so historical buy
// prices cannot inflate ranked gains. Currency is the quote currency of the
// locked price, also backend-derived.
type Position struct {
	ID              string
	UserID          string
	PortfolioID     string
	Symbol          string
	AssetType       string
	Quantity        float64
	AverageBuyPrice float64 // locked baseline price (today's price at add)
	Currency        string  // quote currency of the locked baseline price
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// PositionInput carries the client-supplied fields for creating a position.
// There is deliberately NO price and NO currency here: the baseline price is
// locked server-side at the current market quote.
type PositionInput struct {
	Symbol    string
	AssetType string
	Quantity  float64
}

// PortfolioSummary is the calculated, response-ready view of a portfolio. All
// totals are expressed in the base currency (USD) after FX normalization, so
// mixed-currency portfolios are comparable.
type PortfolioSummary struct {
	UserID             string            `json:"user_id"`
	PortfolioID        string            `json:"portfolio_id"`
	BaseCurrency       string            `json:"base_currency"`
	TotalCostBasis     float64           `json:"total_cost_basis"` // base currency
	CurrentValue       float64           `json:"current_value"`    // base currency
	GainLoss           float64           `json:"gain_loss"`        // base currency
	GainLossPercentage float64           `json:"gain_loss_percentage"`
	PortfolioIndex     float64           `json:"portfolio_index"`
	Positions          []PositionSummary `json:"positions"`
}

// PositionSummary is the calculated view of a single position. CostBasis and
// CurrentValue are in the position's local currency; the *Base fields are the
// FX-normalized base-currency equivalents used for portfolio totals.
type PositionSummary struct {
	PositionID string  `json:"position_id"`
	Symbol     string  `json:"symbol"`
	AssetType  string  `json:"asset_type"`
	Quantity   float64 `json:"quantity"`
	// AverageBuyPrice is the locked baseline price (today's price at add time).
	// Serialized as baseline_price — the product has no "average buy price".
	AverageBuyPrice      float64 `json:"baseline_price"`
	CurrentPrice         float64 `json:"current_price"`
	CurrentPriceCurrency string  `json:"current_price_currency"`
	CostBasis            float64 `json:"cost_basis"`           // local currency
	CurrentValue         float64 `json:"current_value"`        // local currency
	GainLoss             float64 `json:"gain_loss"`            // local currency
	GainLossPercentage   float64 `json:"gain_loss_percentage"` // base-currency performance
	Currency             string  `json:"currency"`
	CostBasisBase        float64 `json:"cost_basis_base"`    // base currency
	CurrentValueBase     float64 `json:"current_value_base"` // base currency
	GainLossBase         float64 `json:"gain_loss_base"`     // base currency
	BaseCurrency         string  `json:"base_currency"`
}

// validAssetTypes is the set of allowed asset_type values.
var validAssetTypes = map[string]bool{
	AssetTypeStock:  true,
	AssetTypeETF:    true,
	AssetTypeCrypto: true,
}
