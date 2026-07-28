package portfolio

import (
	"time"

	"github.com/ardakimyonok/finance_app/internal/money"
)

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
	PortfolioID string       `json:"-"`
	Currency    string       `json:"currency"`
	Amount      money.Amount `json:"amount"`
	CreatedAt   time.Time    `json:"-"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

type CashBalanceView struct {
	Currency         string       `json:"currency"`
	Amount           money.Amount `json:"amount"`
	ValueBase        money.Amount `json:"value_base"`
	WeightPercentage float64      `json:"weight_percentage"`
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
	InstrumentID               string
	AssetType                  string
	Currency                   string
	Quantity                   *money.Quantity
	UnitPrice                  *money.Price
	GrossAmount                money.Amount
	CostBasisAllocated         *money.Amount
	RealizedGainLossBase       *money.Amount
	RealizedGainLossPercentage *float64
	OccurredAt                 time.Time
	PortfolioVersion           int64
	Metadata                   map[string]any
	CreatedAt                  time.Time
	Origin                     string
	Status                     string
	GroupID                    string
	PositionEpisodeID          string
	FeeAmount                  money.Amount
	NetAmount                  money.Amount
}

type ActivityView struct {
	ID                         string          `json:"id"`
	Type                       ActivityType    `json:"activity_type"`
	Symbol                     string          `json:"symbol,omitempty"`
	InstrumentID               string          `json:"instrument_id,omitempty"`
	AssetType                  string          `json:"asset_type,omitempty"`
	Currency                   string          `json:"currency"`
	Quantity                   *money.Quantity `json:"quantity,omitempty"`
	UnitPrice                  *money.Price    `json:"unit_price,omitempty"`
	GrossAmount                money.Amount    `json:"gross_amount"`
	CostBasisAllocated         *money.Amount   `json:"cost_basis_allocated,omitempty"`
	RealizedGainLossBase       *money.Amount   `json:"realized_gain_loss_base,omitempty"`
	RealizedGainLossPercentage *float64        `json:"realized_gain_loss_percentage,omitempty"`
	OccurredAt                 string          `json:"occurred_at"`
	PortfolioVersion           int64           `json:"portfolio_version"`
	Origin                     string          `json:"origin"`
	Status                     string          `json:"status"`
	GroupID                    string          `json:"group_id,omitempty"`
	PositionEpisodeID          string          `json:"position_episode_id,omitempty"`
	FeeAmount                  money.Amount    `json:"fee_amount,omitempty"`
	NetAmount                  money.Amount    `json:"net_amount,omitempty"`
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
	Version int64
	// AutoFundPurchases is a portfolio-level preference (default true). When
	// true, a buy that exceeds the available cash in the instrument's quote
	// currency automatically records a neutral funding deposit for exactly the
	// shortfall instead of being rejected. When false, the buy is rejected with
	// ErrInsufficientCash.
	AutoFundPurchases bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
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
	InstrumentID               string
	AssetType                  string
	Quantity                   money.Quantity
	AverageBuyPrice            money.Price // locked baseline price (today's price at add)
	Currency                   string      // quote currency of the locked baseline price
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
	Quantity  money.Quantity
}

type CashFlowInput struct {
	Currency   string
	Amount     money.Amount
	OccurredAt *time.Time

	// CorrectionOf/CorrectionReason optionally link this cash flow back to the
	// user-recorded activity it compensates for (see ActivityCorrectionInput).
	// Empty for an ordinary deposit/withdrawal.
	CorrectionOf     string
	CorrectionReason string
}

// ActivityCorrectionInput reconciles a user-recorded activity to its actual
// values. It never edits the original (immutable) activity.
//
// Deposit/withdrawal: the service posts a compensating cash flow for the
// delta between ActualAmount and the original, linked back via metadata.
//
// Buy/sell: CorrectedQuantity/CorrectedExecutionPrice/CorrectedFee are the
// only correctable fields, and only when the original is the most recent
// ledger event in its position episode (see isLastEpisodeEvent) — otherwise
// reversing its contribution could conflict with activity that happened
// afterward (partial sells, closures, rebuys), so it is rejected. Symbol,
// instrument, and the trade's date are never correctable: record an
// offsetting buy/sell instead.
type ActivityCorrectionInput struct {
	ActivityID   string
	ActualAmount money.Amount
	Reason       string

	CorrectedQuantity       money.Quantity
	CorrectedExecutionPrice money.Price
	CorrectedFee            money.Amount
}

// TradeCorrectionInput carries a validated buy/sell correction into the
// mutation pipeline. Original is the immutable activity being corrected; the
// coordinator never re-fetches it from storage.
type TradeCorrectionInput struct {
	Original                Activity
	CorrectedQuantity       money.Quantity
	CorrectedExecutionPrice money.Price
	CorrectedFee            money.Amount
	Reason                  string
}

// Execution-price and fee provenance labels. They record HOW a recorded trade's
// price and fee were obtained, so an estimate is never presented as a confirmed
// broker execution. Stored on the activity (metadata + dedicated columns).
const (
	// PriceSourceUserRecorded: the user entered the real execution price.
	PriceSourceUserRecorded = "user_recorded"
	// PriceSourceProviderEstimate: no price was entered, so the latest tracked
	// market quote was used as an estimate.
	PriceSourceProviderEstimate = "provider_estimate"
	// PriceSourceLegacyUnknown: backfilled rows recorded before provenance
	// existed. Never written by new code.
	PriceSourceLegacyUnknown = "legacy_unknown"

	// FeeSourceUserRecorded: the user entered a fee.
	FeeSourceUserRecorded = "user_recorded"
	// FeeSourceDefaultZero: no fee was entered, so zero was assumed.
	FeeSourceDefaultZero = "default_zero"
)

// BuyInput records a real-world purchase happening now. Only
// Symbol/AssetType/Quantity are required: the user records the essential
// action and the backend infers the accounting consequences.
//
//   - ExecutionPrice: the real per-unit price paid. Zero means "estimate it
//     from the latest tracked quote" (provider_estimate).
//   - Fee: the transaction fee actually charged. Zero means default_zero.
//
// There is no backdating: every trade is recorded against the live quote
// pinned for the mutation, so a user-supplied price always has a real
// comparator (see validateLiveExecutionPrice) rather than an unverifiable
// historical claim.
//
// Basis convention (documented in backend/README.md): a position's cost basis
// INCLUDES the purchase price and the buy fee, mirroring the canonical sale
// contract where realized P&L is measured net of the sale fee.
type BuyInput struct {
	Symbol          string
	InstrumentID    string
	ExchangeCode    string
	MIC             string
	IdentityQuality string
	AssetType       string
	Quantity        money.Quantity
	ExecutionPrice  money.Price
	Fee             money.Amount
}

// BuyPreview never writes portfolio, cash, activity, ranked, audit or outbox
// state. Identity resolution may register one unambiguous instrument/alias.
type BuyPreview struct {
	Symbol               string         `json:"symbol"`
	InstrumentID         string         `json:"instrument_id,omitempty"`
	AssetType            string         `json:"asset_type"`
	Quantity             money.Quantity `json:"quantity"`
	ExecutionPrice       money.Price    `json:"execution_price"`
	ExecutionPriceSource string         `json:"execution_price_source"`
	Fee                  money.Amount   `json:"fee"`
	FeeSource            string         `json:"fee_source"`
	GrossPurchaseAmount  money.Amount   `json:"gross_purchase_amount"`
	TotalCashRequired    money.Amount   `json:"total_cash_required"`
	AvailableCash        money.Amount   `json:"available_cash"`
	CashUsed             money.Amount   `json:"cash_used"`
	AutomaticFunding     money.Amount   `json:"automatic_funding_amount"`
	RemainingCash        money.Amount   `json:"remaining_cash"`
	CreatesNewEpisode    bool           `json:"creates_new_episode"`
	PositionEpisodeID    string         `json:"position_episode_id,omitempty"`
	ResultingQuantity    money.Quantity `json:"resulting_quantity"`
	ResultingAverageCost money.Price    `json:"resulting_average_cost"`
	EffectiveAt          string         `json:"effective_at"`
	Currency             string         `json:"currency"`
	BaseCurrency         string         `json:"base_currency"`
	CalculationStatus    string         `json:"calculation_status"`
}

type SellInput struct {
	PositionID     string
	Symbol         string
	Quantity       money.Quantity
	ExecutionPrice money.Price
	Fee            money.Amount
}

type SellPreview struct {
	PositionID           string         `json:"position_id"`
	PositionEpisodeID    string         `json:"position_episode_id"`
	Symbol               string         `json:"symbol"`
	AvailableQuantity    money.Quantity `json:"available_quantity"`
	SoldQuantity         money.Quantity `json:"sold_quantity"`
	RemainingQuantity    money.Quantity `json:"remaining_quantity"`
	ExecutionPrice       money.Price    `json:"execution_price"`
	ExecutionPriceSource string         `json:"execution_price_source"`
	FeeSource            string         `json:"fee_source"`
	EffectiveAt          string         `json:"effective_at"`
	CalculationStatus    string         `json:"calculation_status"`
	GrossProceeds        money.Amount   `json:"gross_proceeds"`
	Fee                  money.Amount   `json:"fee"`
	NetProceeds          money.Amount   `json:"net_proceeds"`
	AllocatedBasis       money.Amount   `json:"allocated_basis"`
	EstimatedRealizedPnL money.Amount   `json:"estimated_realized_pnl"`
	WillClosePosition    bool           `json:"will_close_position"`
	ProceedsCurrency     string         `json:"proceeds_currency"`
	BaseCurrency         string         `json:"base_currency"`
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
	// EconomicBreakdown is the display-ready decomposition reconciliation
	// already verifies (realized + unrealized + income - standalone fees).
	EconomicBreakdown EconomicAttribution `json:"economic_breakdown"`
	// Contributions ranks instruments by contribution in percentage points.
	// Read ContributionAnalysis' doc comment for its honest scope limits.
	Contributions ContributionAnalysis `json:"contributions"`
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
	TotalCostBasis         money.Amount            `json:"total_cost_basis"`
	CurrentValue           money.Amount            `json:"current_value"`
	GainLoss               money.Amount            `json:"gain_loss"`
	GainLossPercentage     float64                 `json:"gain_loss_percentage"`
	PortfolioIndex         float64                 `json:"portfolio_index"`
	Positions              []PositionSummary       `json:"positions"`
	ClosedPositions        []ClosedPositionSummary `json:"closed_positions"`
	ActiveCostBasisBase    money.Amount            `json:"active_cost_basis_base"`
	ActiveCurrentValueBase money.Amount            `json:"active_current_value_base"`
	UnrealizedGainLossBase money.Amount            `json:"unrealized_gain_loss_base"`
	ClosedCostBasisBase    money.Amount            `json:"closed_cost_basis_base"`
	RealizedGainLossBase   money.Amount            `json:"realized_gain_loss_base"`
	QuoteStatus            QuoteStatus             `json:"quote_status"`
	CashBalances           []CashBalanceView       `json:"cash_balances"`
	TotalCashValueBase     money.Amount            `json:"total_cash_value_base"`
	RankedPerformance      RankedPerformanceView   `json:"ranked_performance"`
	Valuation              PortfolioValuation      `json:"valuation"`
	OpenHoldings           OpenHoldingsMetrics     `json:"open_holdings"`
	Realized               RealizedMetrics         `json:"realized"`
	Income                 IncomeMetrics           `json:"income"`
	Fees                   FeeMetrics              `json:"fees"`
	EconomicPerformance    EconomicPerformance     `json:"economic_performance"`
	Reconciliation         ReconciliationStatus    `json:"reconciliation"`

	// HasSelfReportedExecutionPrice is true when any buy/sell contributing to
	// the current cost basis or realized P&L used a user-entered execution
	// price rather than a provider estimate. It exists so a public consumer of
	// this data (a profile page) can disclose that open/closed holdings P&L —
	// unlike the ranked index, which is always priced from tracked market
	// quotes — may include unverifiable, self-reported figures.
	HasSelfReportedExecutionPrice bool `json:"has_self_reported_execution_price"`

	// EconomicAttribution and Contributions are computed alongside the summary
	// but are NOT part of the portfolio-state DTO: they are performance-layer
	// concerns, serialized only by PerformanceSummaryResponse. `json:"-"` keeps
	// the two DTOs separately owned even though one calculation feeds both.
	EconomicAttribution EconomicAttribution  `json:"-"`
	Contributions       ContributionAnalysis `json:"-"`
}

// RankedPerformanceView is percentage-only competitive performance. It must
// never be converted to, or presented beside, an absolute monetary P&L.
type RankedPerformanceView struct {
	Index            float64 `json:"index"`
	ReturnPercentage float64 `json:"return_percentage"`
	TrackingStatus   string  `json:"tracking_status"`
}

type PortfolioValuation struct {
	OpenHoldingsMarketValueBase money.Amount `json:"open_holdings_market_value_base"`
	CashValueBase               money.Amount `json:"cash_value_base"`
	CurrentPortfolioValueBase   money.Amount `json:"current_portfolio_value_base"`
}

type OpenHoldingsMetrics struct {
	CostBasisBase              money.Amount `json:"cost_basis_base"`
	UnrealizedPnLBase          money.Amount `json:"unrealized_pnl_base"`
	UnrealizedReturnPercentage *float64     `json:"unrealized_return_percentage"`
}

type RealizedMetrics struct {
	RealizedPnLBase money.Amount `json:"realized_pnl_base"`
}

type IncomeMetrics struct {
	DividendsBase     money.Amount `json:"dividends_base"`
	DistributionsBase money.Amount `json:"distributions_base"`
	InterestBase      money.Amount `json:"interest_base"`
	OtherIncomeBase   money.Amount `json:"other_income_base"`
	TotalIncomeBase   money.Amount `json:"total_income_base"`
	// ReturnOfCapitalBase is disclosed separately for audit visibility. Return
	// of capital is NOT ordinary income (it is a basis-reducing cash credit),
	// so it is deliberately excluded from TotalIncomeBase. See
	// backend/README.md for the documented accounting policy.
	ReturnOfCapitalBase money.Amount `json:"return_of_capital_base"`
}

type FeeMetrics struct {
	TransactionFeesBase money.Amount `json:"transaction_fees_base"`
	ManagementFeesBase  money.Amount `json:"management_fees_base"`
	CustodyFeesBase     money.Amount `json:"custody_fees_base"`
	OtherFeesBase       money.Amount `json:"other_fees_base"`
	TotalFeesBase       money.Amount `json:"total_fees_base"`
	// EmbeddedInRealizedPnLBase discloses the portion of TotalFeesBase that is
	// a sale fee already netted into RealizedMetrics.RealizedPnLBase (canonical
	// sale contract: net proceeds = gross proceeds - sale fee, realized P&L =
	// net proceeds - allocated cost basis). It is shown for fee-reporting
	// audit purposes only and must never be subtracted a second time from
	// economic attribution — see ReconcilePortfolioFinancials.
	EmbeddedInRealizedPnLBase money.Amount `json:"embedded_in_realized_pnl_base"`
}

type EconomicPerformance struct {
	TotalPnLBase         *money.Amount `json:"total_pnl_base"`
	NetContributionsBase *money.Amount `json:"net_contributions_base"`
	ReturnPercentage     *float64      `json:"return_percentage"`
	CalculationStatus    string        `json:"calculation_status"`
	IsComplete           bool          `json:"is_complete"`
}

type ReconciliationStatus struct {
	IsComplete   bool         `json:"is_complete"`
	IsConsistent bool         `json:"is_consistent"`
	Difference   money.Amount `json:"difference"`
	Reasons      []string     `json:"reasons,omitempty"`
}

// PositionSummary is the calculated view of a single position. CostBasis and
// CurrentValue are in the position's local currency; the *Base fields are the
// FX-normalized base-currency equivalents used for portfolio totals.
type PositionSummary struct {
	PositionID string         `json:"position_id"`
	Symbol     string         `json:"symbol"`
	AssetType  string         `json:"asset_type"`
	Quantity   money.Quantity `json:"quantity"`
	// AverageBuyPrice is the locked baseline price (today's price at add time).
	// Serialized as baseline_price — the product has no "average buy price".
	AverageBuyPrice      money.Price  `json:"baseline_price"`
	CurrentPrice         money.Price  `json:"current_price"`
	CurrentPriceCurrency string       `json:"current_price_currency"`
	CostBasis            money.Amount `json:"cost_basis"`           // local currency
	CurrentValue         money.Amount `json:"current_value"`        // local currency
	GainLoss             money.Amount `json:"gain_loss"`            // local currency
	GainLossPercentage   float64      `json:"gain_loss_percentage"` // base-currency performance
	Currency             string       `json:"currency"`
	CostBasisBase        money.Amount `json:"cost_basis_base"`    // base currency
	CurrentValueBase     money.Amount `json:"current_value_base"` // base currency
	GainLossBase         money.Amount `json:"gain_loss_base"`     // base currency
	BaseCurrency         string       `json:"base_currency"`
	QuoteProvider        string       `json:"quote_provider,omitempty"`
	QuoteProviderStatus  string       `json:"quote_provider_status,omitempty"`
	QuoteIsStale         bool         `json:"quote_is_stale"`
	QuoteFetchedAt       string       `json:"quote_fetched_at,omitempty"`
	QuoteExpiresAt       string       `json:"quote_expires_at,omitempty"`
}

type ClosedPositionSummary struct {
	ID                         string         `json:"id"`
	Symbol                     string         `json:"symbol"`
	AssetType                  string         `json:"asset_type"`
	Quantity                   money.Quantity `json:"quantity"`
	BaselinePrice              money.Price    `json:"baseline_price"`
	BaselineCurrency           string         `json:"baseline_currency"`
	ClosePrice                 money.Price    `json:"close_price"`
	ClosePriceCurrency         string         `json:"close_price_currency"`
	ClosedAt                   string         `json:"closed_at"`
	RealizedGainLossBase       money.Amount   `json:"realized_gain_loss_base"`
	RealizedGainLossPercentage float64        `json:"realized_gain_loss_percentage"`
	ClosedCostBasisBase        money.Amount   `json:"closed_cost_basis_base"`
	BaseCurrency               string         `json:"base_currency"`
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
	// TotalValueBase is the private snapshot value in base currency. It backs the
	// Portfolio Value chart mode only; it is deposit/withdrawal sensitive and must
	// never be used to derive return or drawdown.
	TotalValueBase float64 `json:"total_value_base"`
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
