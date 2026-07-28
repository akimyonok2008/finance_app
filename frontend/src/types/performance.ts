import type { PortfolioArchives } from "@/types/portfolio";
import type { DecimalString } from "@/utils/decimal";

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
  ranked_index: DecimalString;
  /** `index_t / starting_index - 1`, in percent, computed by the backend (float64 presentation, not authoritative decimal state). */
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
  starting_index?: DecimalString;
  ending_index?: DecimalString;
  timeframe_return_percentage?: number;
  max_drawdown_percentage?: number;
  risk: RiskConsistency;
  benchmark: BenchmarkComparison;
};

/** One complete calendar period's ranked return. */
export type PeriodReturn = {
  /** "2026-03" for a month. */
  label: string;
  return_percentage: number;
};

/**
 * Risk & Consistency, computed by the `performancehistory` service from the
 * ranked index. Every field is `null` — never 0 — when there is not enough
 * history, and the paired `*_reason` explains why.
 */
export type RiskConsistency = {
  max_drawdown_percentage: number | null;
  current_drawdown_percentage: number | null;
  positive_weeks_percentage: number | null;
  positive_weeks: number;
  complete_weeks: number;
  best_month: PeriodReturn | null;
  worst_month: PeriodReturn | null;
  complete_months: number;
  weeks_reason?: string;
  months_reason?: string;
  drawdown_reason?: string;
  calculation_base: string;
};

/**
 * Benchmark comparison. Only emitted when both legs were measured over the SAME
 * boundary dates (`aligned_from`/`aligned_to`); otherwise `available` is false
 * with a reason rather than a fabricated 0-point difference.
 */
export type BenchmarkComparison = {
  available: boolean;
  reason?: string;
  recipe_id?: string;
  name?: string;
  aligned_from?: string;
  aligned_to?: string;
  benchmark_return_percentage?: number;
  /** The ranked return over the ALIGNED window, not the full chart window. */
  portfolio_return_percentage?: number;
  difference_percentage_points?: number;
  data_quality?: string;
  data_type: "total_return" | "adjusted_close" | "raw_close" | "synthetic" | "unavailable";
  data_quality_status: string;
  currency_treatment?: string;
  is_synthetic?: boolean;
  verification_status: "verified" | "preview" | "unavailable";
};

/**
 * The economic breakdown. These are LEDGER figures in base currency — ranked
 * return is never converted into money. `total_economic_pnl_base` is null when
 * the ledger does not cover the full holding history.
 */
export type EconomicBreakdown = {
  realized_pnl_base: number;
  unrealized_pnl_base: number;
  net_income_base: number;
  standalone_fees_base: number;
  attributed_total_base: number;
  total_economic_pnl_base: number | null;
  unattributed_base: number | null;
  calculation_status: string;
  is_complete: boolean;
};

/** One instrument's contribution to portfolio return, in percentage points. */
export type InstrumentContribution = {
  symbol: string;
  asset_type?: string;
  weight_percentage: number;
  /** Standalone return, shown for context. Ranking never uses it. */
  instrument_return_percentage: number | null;
  contribution_percentage_points: number;
  economic_result_base: number;
  income_base: number;
};

/**
 * Contributors / Detractors.
 *
 * `basis` is "since_inception": per-instrument daily valuation history does not
 * exist, so a period-specific attribution cannot be computed honestly. The UI
 * must label the section with this scope and never present it as a figure for
 * the selected timeframe.
 */
export type ContributionAnalysis = {
  basis: string;
  calculation_status: string;
  available: boolean;
  reason?: string;
  total_capital_base: number;
  contributors: InstrumentContribution[];
  detractors: InstrumentContribution[];
  unattributed_percentage_points: number;
};

/** GET /performance/summary — the performance layer's own DTO. */
export type PerformanceSummary = {
  base_currency: string;
  ranked: { index: number; return_percentage: number; tracking_status: string };
  economic_breakdown: EconomicBreakdown;
  contributions: ContributionAnalysis;
  reconciliation: {
    is_complete: boolean;
    is_consistent: boolean;
    difference: number;
    reasons?: string[];
  };
};

/**
 * Private valuation history. Backs the Portfolio Value chart mode ONLY — it is
 * deposit/withdrawal sensitive and must never be used for return or drawdown.
 */
export type PortfolioValueHistory = PortfolioArchives;
