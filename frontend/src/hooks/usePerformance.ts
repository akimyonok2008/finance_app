import { keepPreviousData, useQuery } from "@tanstack/react-query";

import {
  getPerformanceHistory,
  getPerformanceSummary,
  getPortfolioValueHistory,
} from "@/api/performance";
import { queryKeys } from "@/hooks/queryKeys";
import type { PerformanceTimeframe } from "@/types/performance";
import type { PortfolioArchiveTimeframe } from "@/types/portfolio";

/** Canonical ranked history — backs the Return and Drawdown chart modes. */
export function usePerformanceHistory(timeframe: PerformanceTimeframe) {
  return useQuery({
    queryKey: queryKeys.performance.history(timeframe),
    queryFn: ({ signal }) => getPerformanceHistory(timeframe, signal),
    placeholderData: keepPreviousData,
  });
}

/**
 * The performance layer's summary — economic breakdown and contributors.
 * Separate from usePortfolioSummary on purpose: the two layers own two DTOs.
 */
export function usePerformanceSummary() {
  return useQuery({
    queryKey: queryKeys.performance.summary,
    queryFn: ({ signal }) => getPerformanceSummary(signal),
  });
}

/** ALL has no archive equivalent; the archive endpoint tops out at 1Y. */
function toArchiveTimeframe(
  timeframe: PerformanceTimeframe,
): PortfolioArchiveTimeframe {
  return timeframe === "ALL" ? "1Y" : timeframe;
}

/** Private valuation history — backs the Portfolio Value chart mode only. */
export function usePortfolioValueHistory(timeframe: PerformanceTimeframe) {
  const archiveTimeframe = toArchiveTimeframe(timeframe);
  return useQuery({
    queryKey: queryKeys.performance.valueHistory(archiveTimeframe),
    queryFn: ({ signal }) => getPortfolioValueHistory(archiveTimeframe, signal),
    placeholderData: keepPreviousData,
  });
}
