import { apiRequest } from "@/api/client";
import type { PortfolioArchiveTimeframe } from "@/types/portfolio";
import type {
  PerformanceSummary,
  PerformanceTimeframe,
  PortfolioValueHistory,
  RankedPerformanceHistory,
} from "@/types/performance";
import { assertShape, rankedPerformanceHistorySchema } from "@/api/schemas";
import { z } from "zod";

/**
 * `available: false` responses omit `points`/`starting_index`/etc entirely
 * (see RankedPerformanceHistory's docstring), so validate leniently when
 * unavailable and strictly (full decimal-string shape) when available.
 */
const rankedHistoryResponseSchema = z.union([
  z.object({ available: z.literal(false) }).passthrough(),
  rankedPerformanceHistorySchema,
]);

/**
 * The performance layer's own summary DTO. The economic breakdown and the
 * contributor analysis come from here — NOT from the portfolio-state summary,
 * which deliberately does not carry them.
 */
export function getPerformanceSummary(
  signal?: AbortSignal,
): Promise<PerformanceSummary> {
  return apiRequest<PerformanceSummary>("/performance/summary", { signal });
}

/**
 * Canonical ranked-performance history. The backend computes the timeframe
 * return and the drawdown series from the ranked snapshot table — the frontend
 * only formats what it receives and never recomputes these itself.
 */
export function getPerformanceHistory(
  timeframe: PerformanceTimeframe,
  signal?: AbortSignal,
): Promise<RankedPerformanceHistory> {
  return apiRequest<RankedPerformanceHistory>(
    `/performance/history?timeframe=${timeframe}`,
    { signal },
  ).then((data) => {
    assertShape(rankedHistoryResponseSchema, data, "GET /performance/history");
    return data;
  });
}

/**
 * Private valuation history for the Portfolio Value chart mode only. It is
 * deposit/withdrawal sensitive, so it must never back return or drawdown.
 */
export function getPortfolioValueHistory(
  timeframe: PortfolioArchiveTimeframe,
  signal?: AbortSignal,
): Promise<PortfolioValueHistory> {
  return apiRequest<PortfolioValueHistory>(
    `/portfolio/archives?timeframe=${timeframe}`,
    { signal },
  );
}
