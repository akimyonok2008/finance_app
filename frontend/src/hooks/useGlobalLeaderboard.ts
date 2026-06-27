import { keepPreviousData, useQuery } from "@tanstack/react-query";

import { getGlobalLeaderboard } from "@/api/leaderboardApi";
import type { LeaderboardQueryParams } from "@/types/leaderboard";

export function useGlobalLeaderboard(params: LeaderboardQueryParams) {
  return useQuery({
    queryKey: ["leaderboard", "global", params.timeframe],
    queryFn: () => getGlobalLeaderboard(params),
    placeholderData: keepPreviousData,
    staleTime: 30_000,
  });
}
