import { keepPreviousData, useQuery } from "@tanstack/react-query";

import { getLeaderboardStanding } from "@/api/leaderboardApi";
import { queryKeys } from "@/hooks/queryKeys";
import type { LeaderboardTimeframe } from "@/types/leaderboard";

export function useLeaderboardStanding(timeframe: LeaderboardTimeframe) {
  return useQuery({
    queryKey: queryKeys.leaderboardStanding(timeframe),
    queryFn: ({ signal }) => getLeaderboardStanding(timeframe, signal),
    placeholderData: keepPreviousData,
    retry: 1,
    staleTime: 30_000,
  });
}
