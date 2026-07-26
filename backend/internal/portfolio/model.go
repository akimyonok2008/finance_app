package portfolio

import "time"

// Asset types accepted for a position.
const (
	AssetTypeStock  = "stock"
	AssetTypeETF    = "etf"
	AssetTypeCrypto = "crypto"
	AssetTypeCash   = "cash" // composition/strategy only; never a Position
)

// DefaultPortfolioName is the name given to the auto-created portfolio.
const DefaultPortfolioName = "Default Portfolio"

const (
	PositionStatusOpen   = "open"
	PositionStatusClosed = "closed"
)

type ActivityType string

const (
	ActivityDeposit        ActivityType = "deposit"
	ActivityWithdrawal     ActivityType = "withdrawal"
	ActivityBuy            ActivityType = "buy"
	ActivitySell           ActivityType = "sell"
	ActivityOpeningBalance ActivityType = "opening_balance"
)

// CashBalance is materialized, immediately-settled portfolio cash. Amount is
// always in Currency and is constrained to be non-negative in both the domain
// and database.
type CashBalance struct {
	PortfolioID string    `json:"-"`
	Currency    string    `json:"currency"`
	Amount      float64   `json:"amount"`
	CreatedAt   time.Time `json:"-"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CashBalanceView struct {
	Currency         string  `json:"currency"`
	Amount           float64 `json:"amount"`
	ValueBase        float64 `json:"value_base"`
	WeightPercentage float64 `json:"weight_percentage"`
}

// Activity is an immutable owner-private record of a user-reported portfolio
// action. It is a ledger foundation, not a claim of brokerage execution.
type Activity struct {
	ID                         string
	RequestID                  string
	PortfolioID                string
	UserID                     string
	Type                       ActivityType
	Symbol                     string
	AssetType                  string
	Currency                   string
	Quantity                   *float64
	UnitPrice                  *float64
	GrossAmount                float64
	CostBasisAllocated         *float64
	RealizedGainLossBase       *float64
	RealizedGainLossPercentage *float64
	OccurredAt                 time.Time
	PortfolioVersion           int64
	Metadata                   map[string]any
	CreatedAt                  time.Time
	Origin                     string
	Status                     string
	GroupID                    string
	PositionEpisodeID          string
	FeeAmount                  float64
	NetAmount                  float64
}

type ActivityView struct {
	ID                         string       `json:"id"`
	Type                       ActivityType `json:"activity_type"`
	Symbol                     string       `json:"symbol,omitempty"`
	AssetType                  string       `json:"asset_type,omitempty"`
	Currency                   string       `json:"currency"`
	Quantity                   *float64     `json:"quantity,omitempty"`
	UnitPrice                  *float64     `json:"unit_price,omitempty"`
	GrossAmount                float64      `json:"gross_amount"`
	CostBasisAllocated         *float64     `json:"cost_basis_allocated,omitempty"`
	RealizedGainLossBase       *float64     `json:"realized_gain_loss_base,omitempty"`
	RealizedGainLossPercentage *float64     `json:"realized_gain_loss_percentage,omitempty"`
	OccurredAt                 string       `json:"occurred_at"`
	PortfolioVersion           int64        `json:"portfolio_version"`
	Origin                     string       `json:"origin"`
	Status                     string       `json:"status"`
	GroupID                    string       `json:"group_id,omitempty"`
	PositionEpisodeID          string       `json:"position_episode_id,omitempty"`
	FeeAmount                  float64      `json:"fee_amount,omitempty"`
	NetAmount                  float64      `json:"net_amount,omitempty"`
}

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
	ID       string
	UserID   string
	Name     string
	Currency string
	// Version is the aggregate version. Every committed portfolio mutation
	// increments it exactly once, giving auditable mutation ordering, stale-
	// operation detection, and an optimistic check for API clients. It is not a
	// substitute for the row lock: serialization comes from SELECT ... FOR UPDATE
	// (Postgres) / the per-portfolio mutex (in-memory).
	Version   int64
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

type CashFlowInput struct {
	Currency   string
	Amount     float64
	OccurredAt *time.Time

	// CorrectionOf/CorrectionReason optionally link this cash flow back to the
	// user-recorded activity it compensates for (see ActivityCorrectionInput).
	// Empty for an ordinary deposit/withdrawal.
	CorrectionOf     string
	CorrectionReason string
}

// ActivityCorrectionInput reconciles a user-recorded activity to its actual
// amount. It never edits the original (immutable) activity; the service posts
// a compensating deposit/withdrawal for the delta and links it back via
// metadata. Only deposit and withdrawal activities are supported: correcting
// a buy/sell safely would require reconstructing everything that happened to
// the position afterward, so those are rejected — record an offsetting
// buy/sell instead.
type ActivityCorrectionInput struct {
	ActivityID   string
	ActualAmount float64
	Reason       string
}

type BuyInput struct {
	Symbol    string
	AssetType string
	Quantity  float64
}

type SellInput struct {
	PositionID     string
	Symbol         string
	Quantity       float64
	ExecutionPrice float64
	Fee            float64
	EffectiveAt    *time.Time
}

type SellPreview struct {
	PositionID           string  `json:"position_id"`
	PositionEpisodeID    string  `json:"position_episode_id"`
	Symbol               string  `json:"symbol"`
	AvailableQuantity    float64 `json:"available_quantity"`
	SoldQuantity         float64 `json:"sold_quantity"`
	RemainingQuantity    float64 `json:"remaining_quantity"`
	ExecutionPrice       float64 `json:"execution_price"`
	GrossProceeds        float64 `json:"gross_proceeds"`
	Fee                  float64 `json:"fee"`
	NetProceeds          float64 `json:"net_proceeds"`
	AllocatedBasis       float64 `json:"allocated_basis"`
	EstimatedRealizedPnL float64 `json:"estimated_realized_pnl"`
	WillClosePosition    bool    `json:"will_close_position"`
	ProceedsCurrency     string  `json:"proceeds_currency"`
	BaseCurrency         string  `json:"base_currency"`
}

type ActivityListResponse struct {
	Items      []ActivityView `json:"items"`
	NextOffset *int           `json:"next_offset"`
	Total      int            `json:"total"`
}

type PerformanceSummaryResponse struct {
	BaseCurrency   string                 `json:"base_currency"`
	Ranked         RankedPerformanceView  `json:"ranked"`
	Economic       EconomicPerformance    `json:"economic"`
	Attribution    PerformanceAttribution `json:"attribution"`
	Reconciliation ReconciliationStatus   `json:"reconciliation"`
}

type PerformanceAttribution struct {
	UnrealizedPnLBase float64 `json:"unrealized_pnl_base"`
	RealizedPnLBase   float64 `json:"realized_pnl_base"`
	IncomeBase        float64 `json:"income_base"`
	FeesBase          float64 `json:"fees_base"`
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
	UserID       string `json:"user_id"`
	PortfolioID  string `json:"portfolio_id"`
	BaseCurrency string `json:"base_currency"`
	// Deprecated compatibility aliases. New clients must use the explicit
	// nested metric groups below. Their fixed scopes are: open holdings basis,
	// total portfolio value, open holdings unrealized P&L/return, and ranked
	// index respectively.
	TotalCostBasis         float64                 `json:"total_cost_basis"`
	CurrentValue           float64                 `json:"current_value"`
	GainLoss               float64                 `json:"gain_loss"`
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
	CashBalances           []CashBalanceView       `json:"cash_balances"`
	TotalCashValueBase     float64                 `json:"total_cash_value_base"`
	RankedPerformance      RankedPerformanceView   `json:"ranked_performance"`
	Valuation              PortfolioValuation      `json:"valuation"`
	OpenHoldings           OpenHoldingsMetrics     `json:"open_holdings"`
	Realized               RealizedMetrics         `json:"realized"`
	Income                 IncomeMetrics           `json:"income"`
	Fees                   FeeMetrics              `json:"fees"`
	EconomicPerformance    EconomicPerformance     `json:"economic_performance"`
	Reconciliation         ReconciliationStatus    `json:"reconciliation"`
}

// RankedPerformanceView is percentage-only competitive performance. It must
// never be converted to, or presented beside, an absolute monetary P&L.
type RankedPerformanceView struct {
	Index            float64 `json:"index"`
	ReturnPercentage float64 `json:"return_percentage"`
	TrackingStatus   string  `json:"tracking_status"`
}

type PortfolioValuation struct {
	OpenHoldingsMarketValueBase float64 `json:"open_holdings_market_value_base"`
	CashValueBase               float64 `json:"cash_value_base"`
	CurrentPortfolioValueBase   float64 `json:"current_portfolio_value_base"`
}

type OpenHoldingsMetrics struct {
	CostBasisBase              float64  `json:"cost_basis_base"`
	UnrealizedPnLBase          float64  `json:"unrealized_pnl_base"`
	UnrealizedReturnPercentage *float64 `json:"unrealized_return_percentage"`
}

type RealizedMetrics struct {
	RealizedPnLBase float64 `json:"realized_pnl_base"`
}

type IncomeMetrics struct {
	DividendsBase     float64 `json:"dividends_base"`
	DistributionsBase float64 `json:"distributions_base"`
	InterestBase      float64 `json:"interest_base"`
	OtherIncomeBase   float64 `json:"other_income_base"`
	TotalIncomeBase   float64 `json:"total_income_base"`
	// ReturnOfCapitalBase is disclosed separately for audit visibility. Return
	// of capital is NOT ordinary income (it is a basis-reducing cash credit),
	// so it is deliberately excluded from TotalIncomeBase. See
	// backend/README.md for the documented accounting policy.
	ReturnOfCapitalBase float64 `json:"return_of_capital_base"`
}

type FeeMetrics struct {
	TransactionFeesBase float64 `json:"transaction_fees_base"`
	ManagementFeesBase  float64 `json:"management_fees_base"`
	CustodyFeesBase     float64 `json:"custody_fees_base"`
	OtherFeesBase       float64 `json:"other_fees_base"`
	TotalFeesBase       float64 `json:"total_fees_base"`
	// EmbeddedInRealizedPnLBase discloses the portion of TotalFeesBase that is
	// a sale fee already netted into RealizedMetrics.RealizedPnLBase (canonical
	// sale contract: net proceeds = gross proceeds - sale fee, realized P&L =
	// net proceeds - allocated cost basis). It is shown for fee-reporting
	// audit purposes only and must never be subtracted a second time from
	// economic attribution — see ReconcilePortfolioFinancials.
	EmbeddedInRealizedPnLBase float64 `json:"embedded_in_realized_pnl_base"`
}

type EconomicPerformance struct {
	TotalPnLBase         *float64 `json:"total_pnl_base"`
	NetContributionsBase *float64 `json:"net_contributions_base"`
	ReturnPercentage     *float64 `json:"return_percentage"`
	CalculationStatus    string   `json:"calculation_status"`
	IsComplete           bool     `json:"is_complete"`
}

type ReconciliationStatus struct {
	IsComplete   bool     `json:"is_complete"`
	IsConsistent bool     `json:"is_consistent"`
	Difference   float64  `json:"difference"`
	Reasons      []string `json:"reasons,omitempty"`
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
	CashBalances           []CashBalanceView
	TotalCashValueBase     float64
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
	CashBalances           []CashBalanceView       `json:"cash_balances,omitempty"`
	TotalCashValueBase     float64                 `json:"total_cash_value_base,omitempty"`
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
