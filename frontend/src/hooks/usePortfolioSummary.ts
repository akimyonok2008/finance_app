import { useQuery } from "@tanstack/react-query";

import { getPortfolioSummary } from "@/api/portfolioApi";
import { queryKeys } from "@/hooks/queryKeys";

export function usePortfolioSummary() {
  return useQuery({
    queryKey: queryKeys.portfolioSummary,
    queryFn: ({ signal }) => getPortfolioSummary(signal),
  });
}
