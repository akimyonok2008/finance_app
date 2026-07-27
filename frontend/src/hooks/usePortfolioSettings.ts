import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";

import {
  getPortfolioSettings,
  updatePortfolioSettings,
} from "@/api/portfolioApi";
import { queryKeys } from "@/hooks/queryKeys";

export function usePortfolioSettings() {
  return useQuery({
    queryKey: queryKeys.portfolio.settings,
    queryFn: ({ signal }) => getPortfolioSettings(signal),
  });
}

export function useUpdatePortfolioSettings() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: updatePortfolioSettings,
    onSuccess: (settings) => {
      queryClient.setQueryData(queryKeys.portfolio.settings, settings);
      toast.success("Portfolio settings updated");
    },
    onError: (err: Error) => {
      toast.error(err.message);
    },
  });
}
