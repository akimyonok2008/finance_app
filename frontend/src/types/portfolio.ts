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
