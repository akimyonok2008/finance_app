export type AssetType = "stock" | "etf" | "crypto";

/** Supported demo/mock currencies (backend rejects others with 400). */
export type CurrencyCode = "USD" | "TRY" | "EUR" | "GBP";

/**
 * A raw position as returned by GET /portfolio/positions. `baseline_price` is
 * the market price locked at add time (today's price) in the position's quote
 * currency — there is no average/historical buy price in the product.
 */
export type Position = {
  id: string;
  symbol: string;
  asset_type: AssetType;
  quantity: number;
  baseline_price: number;
  currency: string;
  status?: "open" | "closed";
  position_episode_id: string;
  opened_at: string;
};

export type ClosedPosition = {
  id: string;
  symbol: string;
  asset_type: AssetType;
  quantity: number;
  baseline_price: number;
  baseline_currency: string;
  close_price: number;
  close_price_currency: string;
  closed_at: string;
  realized_gain_loss_base: number;
  realized_gain_loss_percentage: number;
  closed_cost_basis_base?: number;
  base_currency: string;
};

/**
 * A position enriched with pricing/performance data inside the summary payload.
 * Many fields are optional because the backend may omit them when a price is
 * unavailable — build defensively.
 */
export type PositionSummary = {
  position_id?: string;
  id?: string;
  symbol: string;
  asset_type: AssetType;
  quantity: number;
  baseline_price: number;
  current_price?: number;
  current_price_currency?: string;
  cost_basis?: number;
  current_value?: number;
  cost_basis_base?: number;
  current_value_base?: number;
  gain_loss?: number;
  gain_loss_base?: number;
  gain_loss_percentage?: number;
  currency: string;
  base_currency?: string;
  quote_provider?: string;
  quote_provider_status?: string;
  quote_is_stale?: boolean;
  quote_fetched_at?: string;
  quote_expires_at?: string;
};

export type QuoteStatus = {
  provider: string;
  provider_status: string;
  last_fetched_at?: string;
  stale_count: number;
  total_quotes: number;
};

/** Aggregated portfolio performance from GET /portfolio/summary. */
export type PortfolioSummary = {
  user_id?: string;
  portfolio_id?: string;
  base_currency?: string;
  total_cost_basis: number;
  current_value: number;
  gain_loss: number;
  gain_loss_percentage: number;
  portfolio_index: number;
  positions?: PositionSummary[];
  closed_positions?: ClosedPosition[];
  active_cost_basis_base?: number;
  active_current_value_base?: number;
  unrealized_gain_loss_base?: number;
  closed_cost_basis_base?: number;
  realized_gain_loss_base?: number;
  quote_status?: QuoteStatus;
  cash_balances?: CashBalance[];
  total_cash_value_base?: number;
  ranked_performance: {
    index: number;
    return_percentage: number;
    tracking_status: "active" | "paused" | "unavailable";
  };
  valuation: {
    open_holdings_market_value_base: number;
    cash_value_base: number;
    current_portfolio_value_base: number;
  };
  open_holdings: {
    cost_basis_base: number;
    unrealized_pnl_base: number;
    unrealized_return_percentage: number | null;
  };
  realized: {
    realized_pnl_base: number;
  };
  income: {
    dividends_base: number;
    distributions_base: number;
    interest_base: number;
    other_income_base: number;
    total_income_base: number;
  };
  fees: {
    transaction_fees_base: number;
    management_fees_base: number;
    custody_fees_base: number;
    other_fees_base: number;
    total_fees_base: number;
  };
  economic_performance: {
    total_pnl_base: number | null;
    net_contributions_base: number | null;
    return_percentage: number | null;
    calculation_status: "complete" | "legacy_estimate" | "insufficient_history";
    is_complete: boolean;
  };
  reconciliation: {
    is_complete: boolean;
    is_consistent: boolean;
    difference: number;
    reasons?: string[];
  };
};

export type CashBalance = {
  currency: CurrencyCode;
  amount: number;
  value_base: number;
  weight_percentage: number;
};

export type CashResponse = {
  cash_balances: CashBalance[];
  total_cash_value_base: number;
  base_currency: string;
};

export type PortfolioActivity = {
  id: string;
  activity_type:
    | "deposit" | "withdrawal" | "buy" | "sell" | "opening_balance"
    | "cash_dividend" | "etf_distribution" | "interest_income"
    | "reinvested_dividend" | "buy_fee" | "sell_fee"
    | "management_fee" | "custody_fee" | "other_fee"
    | "stock_split" | "reverse_split" | "symbol_change" | "write_off";
  symbol?: string;
  asset_type?: AssetType;
  currency: CurrencyCode;
  quantity?: number;
  unit_price?: number;
  gross_amount: number;
  cost_basis_allocated?: number;
  realized_gain_loss_base?: number;
  realized_gain_loss_percentage?: number;
  occurred_at: string;
  portfolio_version: number;
  origin: "user_recorded" | "system_generated" | "provider_generated" | "migration_generated";
  status: "completed" | "pending" | "processing" | "corrected" | "reversed" | "failed";
  group_id?: string;
  position_episode_id?: string;
  fee_amount?: number;
  net_amount?: number;
};

export type ActivityMutationResponse = {
  position?: Position;
  activity?: PortfolioActivity;
  portfolio_version: number;
  ranked_index: number;
  ranking_status: "active" | "paused";
};

export type CashFlowInput = { currency: CurrencyCode; amount: number };
export type BuyPreview = {
  symbol: string;
  asset_type: AssetType;
  quantity: number;
  execution_price: number;
  execution_price_source: PriceSource;
  fee: number;
  fee_source: FeeSource;
  gross_purchase_amount: number;
  total_cash_required: number;
  available_cash: number;
  cash_used: number;
  automatic_funding_amount: number;
  remaining_cash: number;
  creates_new_episode: boolean;
  position_episode_id?: string;
  resulting_quantity: number;
  resulting_average_cost: number;
  effective_at: string;
  currency: string;
  base_currency: string;
  calculation_status: string;
};

export type SellPositionInput = {
  position_id: string;
  quantity: number;
  execution_price?: number;
  fee?: number;
  effective_at?: string;
};

export type SellPreview = {
  position_id: string;
  position_episode_id: string;
  symbol: string;
  available_quantity: number;
  sold_quantity: number;
  remaining_quantity: number;
  execution_price: number;
  execution_price_source: PriceSource;
  fee_source: FeeSource;
  effective_at: string;
  calculation_status: string;
  gross_proceeds: number;
  fee: number;
  net_proceeds: number;
  allocated_basis: number;
  estimated_realized_pnl: number;
  will_close_position: boolean;
  proceeds_currency: string;
  base_currency: string;
};

export type PortfolioStateSummary = Pick<
  PortfolioSummary,
  "base_currency" | "valuation" | "open_holdings" | "positions" | "closed_positions" | "cash_balances"
>;

export type PortfolioArchiveTimeframe = "1W" | "1M" | "3M" | "6M" | "1Y";

export type PortfolioArchivePoint = {
  captured_at: string;
  portfolio_index: number;
  gain_loss_percentage: number;
};

export type PortfolioArchiveSnapshot = {
  captured_at: string;
  portfolio_index: number;
  gain_loss_percentage?: number;
  total_cost_basis?: number;
  current_value?: number;
  unrealized_gain_loss_base?: number;
  realized_gain_loss_base?: number;
  positions?: PositionSummary[];
  closed_positions?: ClosedPosition[];
  cash_balances?: CashBalance[];
  total_cash_value_base?: number;
};

export type PortfolioArchives = {
  timeframe: PortfolioArchiveTimeframe;
  from: string;
  to: string;
  points: PortfolioArchivePoint[];
  earliest_snapshot?: PortfolioArchiveSnapshot;
  latest_snapshot?: PortfolioArchiveSnapshot;
};

/**
 * Create payload: no price and no currency. The backend locks the baseline at
 * the current market quote, so positions always start at index 100.
 */
export type CreatePositionInput = {
  symbol: string;
  asset_type: AssetType;
  quantity: number;
  /** Real per-unit price paid. Omitted → estimated from the latest quote. */
  execution_price?: number;
  /** Transaction fee actually charged. Omitted → zero. */
  fee?: number;
  /** When the trade really happened (RFC3339). Omitted → now. */
  effective_at?: string;
};

/** Where a recorded price/fee came from, so estimates are never shown as facts. */
export type PriceSource = "user_recorded" | "provider_estimate" | "legacy_unknown";
export type FeeSource = "user_recorded" | "default_zero" | "legacy_unknown";

/** Only the quantity is editable; the locked baseline price is immutable. */
export type UpdatePositionInput = {
  quantity: number;
};

export const PORTFOLIO_ARCHIVE_TIMEFRAMES: PortfolioArchiveTimeframe[] = [
  "1W",
  "1M",
  "3M",
  "6M",
  "1Y",
];

export const ASSET_TYPES: AssetType[] = ["stock", "etf", "crypto"];

export const CURRENCIES: CurrencyCode[] = ["USD", "TRY", "EUR", "GBP"];

export const DEMO_SYMBOLS = [
  "AAPL",
  "MSFT",
  "NVDA",
  "SPY",
  "BTC-USD",
  "ETH-USD",
  "THYAO.IS",
  "GARAN.IS",
  "ASELS.IS",
] as const;

/** Resolve the stable id of a summary position (backend may use either key). */
export function summaryPositionId(p: PositionSummary): string {
  return p.position_id ?? p.id ?? p.symbol;
}
