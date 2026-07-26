import type { PortfolioArchives } from "@/types/portfolio";

/** Timeframes accepted by the canonical ranked-history endpoint. */
export type PerformanceTimeframe = "1W" | "1M" | "3M" | "6M" | "1Y" | "ALL";

export const PERFORMANCE_TIMEFRAMES: PerformanceTimeframe[] = [
  "1W",
  "1M",
  "3M",
  "6M",
  "1Y",
  "ALL",
];

/**
 * One canonical ranked-history point. Ranked projections only — never values,
 * quantities, or cost basis.
 */
export type RankedHistoryPoint = {
  captured_at: string;
  ranked_index: number;
  /** `index_t / starting_index - 1`, in percent, computed by the backend. */
  return_percentage: number;
  /** `index_t / running_peak_t - 1`, in percent (non-positive). */
  drawdown_percentage: number;
  ranking_status: string;
};

/**
 * GET /performance/history — the canonical ranked-performance history, the same
 * source the leaderboard and benchmark achievements treat as truth.
 *
 * When `available` is false the analytics fields are ABSENT, not zero, so the UI
 * shows a truthful empty state instead of a fabricated flat line.
 */
export type RankedPerformanceHistory = {
  timeframe: PerformanceTimeframe;
  from: string;
  to: string;
  available: boolean;
  reason?: string;
  points: RankedHistoryPoint[];
  starting_index?: number;
  ending_index?: number;
  timeframe_return_percentage?: number;
  max_drawdown_percentage?: number;
};

/**
 * Private valuation history. Backs the Portfolio Value chart mode ONLY — it is
 * deposit/withdrawal sensitive and must never be used for return or drawdown.
 */
export type PortfolioValueHistory = PortfolioArchives;
