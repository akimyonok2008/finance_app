import { useQuery } from "@tanstack/react-query";
import { useAuth } from "@/auth/useAuth";
import { getLeaderboardMe } from "@/api/dashboardApi";
import { queryKeys } from "@/hooks/queryKeys";

export function useLeaderboardMe() {
  const { user } = useAuth();
  return useQuery({
    queryKey: queryKeys.leaderboardMe,
    queryFn: ({ signal }) => getLeaderboardMe(user?.display_name, signal),
    // Ranking is non-critical — a failure should degrade gracefully, not block.
    retry: 1,
  });
}
