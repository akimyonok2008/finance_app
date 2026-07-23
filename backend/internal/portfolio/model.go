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

const (
	PositionStatusOpen   = "open"
	PositionStatusClosed = "closed"
)

const (
	ArchiveTimeframe1W = "1W"
	ArchiveTimeframe1M = "1M"
	ArchiveTimeframe3M = "3M"
	ArchiveTimeframe6M = "6M"
	ArchiveTimeframe1Y = "1Y"
)

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
	ID                         string
	UserID                     string
	PortfolioID                string
	Symbol                     string
	AssetType                  string
	Quantity                   float64
	AverageBuyPrice            float64 // locked baseline price (today's price at add)
	Currency                   string  // quote currency of the locked baseline price
	Status                     string
	ClosedAt                   *time.Time
	ClosePrice                 *float64
	CloseCurrency              string
	RealizedGainLossBase       float64
	RealizedGainLossPercentage float64
	CreatedAt                  time.Time
	UpdatedAt                  time.Time
}

// PositionInput carries the client-supplied fields for creating a position.
// There is deliberately NO price and NO currency here: the baseline price is
// locked server-side at the current market quote.
type PositionInput struct {
	Symbol    string
	AssetType string
	Quantity  float64
}

// StrategyWeightInput is a target allocation used to create a fresh strategy
// baseline. It contains no quantities, money values, prices, or private ids.
type StrategyWeightInput struct {
	Symbol           string  `json:"symbol"`
	AssetType        string  `json:"asset_type"`
	WeightPercentage float64 `json:"weight_percentage"`
}

// PortfolioSummary is the calculated, response-ready view of a portfolio. All
// totals are expressed in the base currency (USD) after FX normalization, so
// mixed-currency portfolios are comparable.
type PortfolioSummary struct {
	UserID                 string                  `json:"user_id"`
	PortfolioID            string                  `json:"portfolio_id"`
	BaseCurrency           string                  `json:"base_currency"`
	TotalCostBasis         float64                 `json:"total_cost_basis"` // base currency
	CurrentValue           float64                 `json:"current_value"`    // base currency
	GainLoss               float64                 `json:"gain_loss"`        // base currency
	GainLossPercentage     float64                 `json:"gain_loss_percentage"`
	PortfolioIndex         float64                 `json:"portfolio_index"`
	Positions              []PositionSummary       `json:"positions"`
	ClosedPositions        []ClosedPositionSummary `json:"closed_positions"`
	ActiveCostBasisBase    float64                 `json:"active_cost_basis_base"`
	ActiveCurrentValueBase float64                 `json:"active_current_value_base"`
	UnrealizedGainLossBase float64                 `json:"unrealized_gain_loss_base"`
	ClosedCostBasisBase    float64                 `json:"closed_cost_basis_base"`
	RealizedGainLossBase   float64                 `json:"realized_gain_loss_base"`
	QuoteStatus            QuoteStatus             `json:"quote_status"`
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
	QuoteProvider        string  `json:"quote_provider,omitempty"`
	QuoteProviderStatus  string  `json:"quote_provider_status,omitempty"`
	QuoteIsStale         bool    `json:"quote_is_stale"`
	QuoteFetchedAt       string  `json:"quote_fetched_at,omitempty"`
	QuoteExpiresAt       string  `json:"quote_expires_at,omitempty"`
}

type ClosedPositionSummary struct {
	ID                         string  `json:"id"`
	Symbol                     string  `json:"symbol"`
	AssetType                  string  `json:"asset_type"`
	Quantity                   float64 `json:"quantity"`
	BaselinePrice              float64 `json:"baseline_price"`
	BaselineCurrency           string  `json:"baseline_currency"`
	ClosePrice                 float64 `json:"close_price"`
	ClosePriceCurrency         string  `json:"close_price_currency"`
	ClosedAt                   string  `json:"closed_at"`
	RealizedGainLossBase       float64 `json:"realized_gain_loss_base"`
	RealizedGainLossPercentage float64 `json:"realized_gain_loss_percentage"`
	ClosedCostBasisBase        float64 `json:"closed_cost_basis_base"`
	BaseCurrency               string  `json:"base_currency"`
}

type PortfolioArchiveSnapshot struct {
	ID                     string
	UserID                 string
	PortfolioID            string
	CapturedAt             time.Time
	BaseCurrency           string
	PortfolioIndex         float64
	GainLossPercentage     float64
	TotalCostBasis         float64
	CurrentValue           float64
	UnrealizedGainLossBase float64
	RealizedGainLossBase   float64
	Positions              []PositionSummary
	ClosedPositions        []ClosedPositionSummary
}

type PortfolioArchivePoint struct {
	CapturedAt         string  `json:"captured_at"`
	PortfolioIndex     float64 `json:"portfolio_index"`
	GainLossPercentage float64 `json:"gain_loss_percentage"`
}

type PortfolioArchiveSnapshotView struct {
	CapturedAt             string                  `json:"captured_at"`
	PortfolioIndex         float64                 `json:"portfolio_index"`
	GainLossPercentage     float64                 `json:"gain_loss_percentage,omitempty"`
	TotalCostBasis         float64                 `json:"total_cost_basis,omitempty"`
	CurrentValue           float64                 `json:"current_value,omitempty"`
	UnrealizedGainLossBase float64                 `json:"unrealized_gain_loss_base,omitempty"`
	RealizedGainLossBase   float64                 `json:"realized_gain_loss_base,omitempty"`
	Positions              []PositionSummary       `json:"positions,omitempty"`
	ClosedPositions        []ClosedPositionSummary `json:"closed_positions,omitempty"`
}

type PortfolioArchives struct {
	Timeframe        string                        `json:"timeframe"`
	From             string                        `json:"from"`
	To               string                        `json:"to"`
	Points           []PortfolioArchivePoint       `json:"points"`
	EarliestSnapshot *PortfolioArchiveSnapshotView `json:"earliest_snapshot,omitempty"`
	LatestSnapshot   *PortfolioArchiveSnapshotView `json:"latest_snapshot,omitempty"`
}

type QuoteStatus struct {
	Provider       string `json:"provider"`
	ProviderStatus string `json:"provider_status"`
	LastFetchedAt  string `json:"last_fetched_at,omitempty"`
	StaleCount     int    `json:"stale_count"`
	TotalQuotes    int    `json:"total_quotes"`
}

// validAssetTypes is the set of allowed asset_type values.
var validAssetTypes = map[string]bool{
	AssetTypeStock:  true,
	AssetTypeETF:    true,
	AssetTypeCrypto: true,
}
