import { useQuery } from "@tanstack/react-query";

import { getPerformanceHistory } from "@/api/performance";
import { queryKeys } from "@/hooks/queryKeys";
import type { PortfolioArchiveTimeframe } from "@/types/portfolio";

export function usePerformanceHistory(timeframe: PortfolioArchiveTimeframe) {
  return useQuery({
    queryKey: queryKeys.performance.history(timeframe),
    queryFn: ({ signal }) => getPerformanceHistory(timeframe, signal),
  });
}
