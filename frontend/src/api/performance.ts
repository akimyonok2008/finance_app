import { apiRequest } from "@/api/client";
import type { PortfolioArchiveTimeframe } from "@/types/portfolio";
import type { RankedPerformanceHistory } from "@/types/performance";

export function getPerformanceHistory(
  timeframe: PortfolioArchiveTimeframe,
  signal?: AbortSignal,
): Promise<RankedPerformanceHistory> {
  return apiRequest<RankedPerformanceHistory>(
    `/performance/history?timeframe=${timeframe}`,
    { signal },
  );
}
