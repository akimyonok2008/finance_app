import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";

import {
  type ArenaBucket,
  getAchievements,
  getArenaCatalogue,
  getArenaCatalogueItem,
  getCompetitionLeaderboard,
  getCompetitionResults,
  getEligibility,
  getMyCompetitionStatus,
  joinCompetition,
  withdrawFromCompetition,
} from "@/api/arenaApi";
import { useAuth } from "@/auth/useAuth";
import { queryKeys } from "@/hooks/queryKeys";

const LEADERBOARD_PAGE_SIZE = 25;
export const ARENA_CATALOGUE_PAGE_SIZE = 12;

/** One catalogue tab (live/upcoming/mine/completed), server-paginated with "load more". */
export function useArenaCatalogueBucket(bucket: ArenaBucket, category?: string) {
  return useInfiniteQuery({
    queryKey: queryKeys.arenaCatalogue({ bucket, category }),
    queryFn: ({ pageParam, signal }) =>
      getArenaCatalogue(
        { bucket, category, limit: ARENA_CATALOGUE_PAGE_SIZE, offset: pageParam },
        signal,
      ),
    initialPageParam: 0,
    getNextPageParam: (lastPage, allPages) =>
      lastPage.hasMore
        ? allPages.reduce((count, page) => count + page.items.length, 0)
        : undefined,
    retry: 1,
  });
}

export function useAchievements() {
  return useQuery({
    queryKey: queryKeys.achievements,
    queryFn: getAchievements,
    retry: 1,
  });
}

/** Bundles everything a competition detail screen needs for one competition. */
export function useCompetitionDetail(competitionId: string | undefined) {
  const { user } = useAuth();
  const queryClient = useQueryClient();
  const [leaderboardCursor, setLeaderboardCursor] = useState<number | undefined>(undefined);

  const cardQuery = useQuery({
    queryKey: competitionId ? queryKeys.arenaCompetition(competitionId) : ["arenaCompetition", ""],
    queryFn: ({ signal }) => getArenaCatalogueItem(competitionId!, signal),
    enabled: Boolean(competitionId),
    retry: 1,
  });
  const eligibilityQuery = useQuery({
    queryKey: competitionId ? queryKeys.arenaEligibility(competitionId) : ["arenaEligibility", ""],
    queryFn: ({ signal }) => getEligibility(competitionId!, signal),
    enabled: Boolean(competitionId) && cardQuery.data?.joined === false,
    retry: 1,
  });
  const statusQuery = useQuery({
    queryKey: competitionId ? queryKeys.arenaMyStatus(competitionId) : ["arenaMyStatus", ""],
    queryFn: ({ signal }) => getMyCompetitionStatus(competitionId!, signal),
    enabled: Boolean(competitionId) && cardQuery.data?.joined === true,
    retry: 1,
  });
  const leaderboardQuery = useQuery({
    queryKey: competitionId
      ? queryKeys.arenaLeaderboard(competitionId, leaderboardCursor ?? 0)
      : ["arenaLeaderboard", "", 0],
    queryFn: ({ signal }) =>
      getCompetitionLeaderboard(
        competitionId!,
        {
          after: leaderboardCursor,
          limit: LEADERBOARD_PAGE_SIZE,
          currentUserDisplayName: user?.display_name,
        },
        signal,
      ),
    enabled: Boolean(competitionId) && cardQuery.data?.status !== "completed",
    retry: 1,
  });
  const resultsQuery = useQuery({
    queryKey: competitionId ? queryKeys.arenaResults(competitionId) : ["arenaResults", ""],
    queryFn: ({ signal }) =>
      getCompetitionResults(competitionId!, user?.display_name, signal),
    enabled: Boolean(competitionId) && cardQuery.data?.status === "completed",
    retry: 1,
  });

  const invalidateAll = () => {
    if (!competitionId) return;
    queryClient.invalidateQueries({ queryKey: queryKeys.arenaCompetition(competitionId) });
    queryClient.invalidateQueries({ queryKey: queryKeys.arenaEligibility(competitionId) });
    queryClient.invalidateQueries({ queryKey: queryKeys.arenaMyStatus(competitionId) });
    queryClient.invalidateQueries({ queryKey: ["arenaLeaderboard", competitionId] });
    queryClient.invalidateQueries({ queryKey: queryKeys.arenaCatalogue() });
  };

  const joinMutation = useMutation({
    mutationFn: () => joinCompetition(competitionId!),
    onSuccess: (result) => {
      invalidateAll();
      toast.success(result.joined ? "You joined the competition" : "Join request submitted");
    },
    onError: (error: Error) => toast.error(error.message),
  });
  const withdrawMutation = useMutation({
    mutationFn: () => withdrawFromCompetition(competitionId!),
    onSuccess: () => {
      invalidateAll();
      toast.success("You withdrew from the competition");
    },
    onError: (error: Error) => toast.error(error.message),
  });

  return {
    card: cardQuery.data,
    eligibility: eligibilityQuery.data,
    status: statusQuery.data,
    leaderboard: leaderboardQuery.data,
    results: resultsQuery.data,
    isLoading: cardQuery.isLoading,
    isError: cardQuery.isError,
    error: cardQuery.error,
    leaderboardCursor,
    nextLeaderboardPage: () => {
      if (leaderboardQuery.data?.nextCursor !== undefined) {
        setLeaderboardCursor(leaderboardQuery.data.nextCursor);
      }
    },
    resetLeaderboardPage: () => setLeaderboardCursor(undefined),
    hasNextLeaderboardPage: leaderboardQuery.data?.nextCursor !== undefined,
    isLoadingLeaderboard: leaderboardQuery.isFetching,
    join: joinMutation.mutate,
    isJoining: joinMutation.isPending,
    withdraw: withdrawMutation.mutate,
    isWithdrawing: withdrawMutation.isPending,
    refetch: async () => {
      await Promise.all([
        cardQuery.refetch(),
        eligibilityQuery.refetch(),
        statusQuery.refetch(),
        leaderboardQuery.refetch(),
        resultsQuery.refetch(),
      ]);
    },
  };
}
