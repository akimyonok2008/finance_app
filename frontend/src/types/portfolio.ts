import type { DecimalString } from "@/utils/decimal";

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
  quantity: DecimalString;
  baseline_price: DecimalString;
  currency: string;
  status?: "open" | "closed";
  position_episode_id: string;
  opened_at: string;
};

export type ClosedPosition = {
  id: string;
  symbol: string;
  asset_type: AssetType;
  quantity: DecimalString;
  baseline_price: DecimalString;
  baseline_currency: string;
  close_price: DecimalString;
  close_price_currency: string;
  closed_at: string;
  realized_gain_loss_base: DecimalString;
  /** presentation-only float64, not part of the money contract */
  realized_gain_loss_percentage: number;
  closed_cost_basis_base?: DecimalString;
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
  quantity: DecimalString;
  baseline_price: DecimalString;
  current_price?: DecimalString;
  current_price_currency?: string;
  cost_basis?: DecimalString;
  current_value?: DecimalString;
  cost_basis_base?: DecimalString;
  current_value_base?: DecimalString;
  gain_loss?: DecimalString;
  gain_loss_base?: DecimalString;
  /** presentation-only float64, not part of the money contract */
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

/** GET /portfolio and PATCH /portfolio/settings. */
export type PortfolioSettings = {
  id: string;
  user_id: string;
  name: string;
  currency: string;
  // When true (the default), a buy that would need more cash than is
  // available automatically draws an implicit deposit for the shortfall.
  // When false, such a buy is rejected instead — "buys require sufficient
  // cash" for users who want that stricter behavior.
  auto_fund_purchases: boolean;
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
  amount: DecimalString;
  value_base: DecimalString;
  /** presentation-only float64, not part of the money contract */
  weight_percentage: number;
};

export type CashResponse = {
  cash_balances: CashBalance[];
  /** still a float64 aggregate on PortfolioSummary; kept as number here too for symmetry with the summary DTO */
  total_cash_value_base: number;
  base_currency: string;
};

export type PortfolioActivity = {
  id: string;
  activity_type:
    | "deposit" | "withdrawal" | "buy" | "sell" | "opening_balance"
    | "cash_dividend" | "etf_distribution" | "interest_income"
    | "reinvested_dividend" | "return_of_capital" | "stock_dividend"
    | "buy_fee" | "sell_fee"
    | "management_fee" | "custody_fee" | "other_fee"
    | "stock_split" | "reverse_split" | "symbol_change" | "write_off";
  symbol?: string;
  asset_type?: AssetType;
  currency: CurrencyCode;
  /**
   * Verified against internal/portfolio/model.go `ActivityView`: Quantity,
   * UnitPrice, GrossAmount, CostBasisAllocated, RealizedGainLossBase,
   * FeeAmount, NetAmount are all `money.*` types (decimal strings) on the
   * wire. Only RealizedGainLossPercentage stays float64. This struct is not
   * explicitly listed in the handoff summary but was verified directly.
   */
  quantity?: DecimalString;
  unit_price?: DecimalString;
  gross_amount: DecimalString;
  cost_basis_allocated?: DecimalString;
  realized_gain_loss_base?: DecimalString;
  /** presentation-only float64, not part of the money contract */
  realized_gain_loss_percentage?: number;
  occurred_at: string;
  portfolio_version: number;
  origin: "user_recorded" | "system_generated" | "provider_generated" | "migration_generated";
  status: "completed" | "pending" | "processing" | "corrected" | "reversed" | "failed";
  group_id?: string;
  position_episode_id?: string;
  fee_amount?: DecimalString;
  net_amount?: DecimalString;
};

export type ActivityMutationResponse = {
  position?: Position;
  activity?: PortfolioActivity;
  portfolio_version: number;
  /** verified: handler.go declares this `money.IndexValue` (decimal string). */
  ranked_index: DecimalString;
  ranking_status: "active" | "paused";
};

export type CashFlowInput = { currency: CurrencyCode; amount: DecimalString };
export type BuyPreview = {
  symbol: string;
  asset_type: AssetType;
  quantity: DecimalString;
  execution_price: DecimalString;
  execution_price_source: PriceSource;
  fee: DecimalString;
  fee_source: FeeSource;
  gross_purchase_amount: DecimalString;
  total_cash_required: DecimalString;
  available_cash: DecimalString;
  cash_used: DecimalString;
  automatic_funding_amount: DecimalString;
  remaining_cash: DecimalString;
  creates_new_episode: boolean;
  position_episode_id?: string;
  resulting_quantity: DecimalString;
  resulting_average_cost: DecimalString;
  effective_at: string;
  currency: string;
  base_currency: string;
  calculation_status: string;
};

export type SellPositionInput = {
  position_id: string;
  quantity: DecimalString;
  execution_price?: DecimalString;
  fee?: DecimalString;
  effective_at?: string;
};

export type SellPreview = {
  position_id: string;
  position_episode_id: string;
  symbol: string;
  available_quantity: DecimalString;
  sold_quantity: DecimalString;
  remaining_quantity: DecimalString;
  execution_price: DecimalString;
  execution_price_source: PriceSource;
  fee_source: FeeSource;
  effective_at: string;
  calculation_status: string;
  gross_proceeds: DecimalString;
  fee: DecimalString;
  net_proceeds: DecimalString;
  allocated_basis: DecimalString;
  estimated_realized_pnl: DecimalString;
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
  /** Private snapshot value in base currency. Value chart mode only. */
  total_value_base: number;
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
  quantity: DecimalString;
  /** Real per-unit price paid. Omitted → estimated from the latest quote. */
  execution_price?: DecimalString;
  /** Transaction fee actually charged. Omitted → zero. */
  fee?: DecimalString;
  /** When the trade really happened (RFC3339). Omitted → now. */
  effective_at?: string;
};

/** Where a recorded price/fee came from, so estimates are never shown as facts. */
export type PriceSource = "user_recorded" | "provider_estimate" | "legacy_unknown";
export type FeeSource = "user_recorded" | "default_zero" | "legacy_unknown";

/** Only the quantity is editable; the locked baseline price is immutable. */
export type UpdatePositionInput = {
  quantity: DecimalString;
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
