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
};

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
};

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
