import { apiRequest } from "@/api/client";
import type { PortfolioArchiveTimeframe } from "@/types/portfolio";
import type {
  PerformanceTimeframe,
  PortfolioValueHistory,
  RankedPerformanceHistory,
} from "@/types/performance";

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
  );
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
