import { useQuery } from "@tanstack/react-query";
import { getLeaderboardMe } from "@/api/dashboardApi";
import { queryKeys } from "@/hooks/queryKeys";

export function useLeaderboardMe() {
  return useQuery({
    queryKey: queryKeys.leaderboardMe,
    queryFn: ({ signal }) => getLeaderboardMe(signal),
    // Ranking is non-critical — a failure should degrade gracefully, not block.
    retry: 1,
  });
}
