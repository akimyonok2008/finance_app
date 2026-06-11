import { usePortfolioSummary } from "@/hooks/usePortfolioSummary";
import { useLeaderboardMe } from "@/hooks/useLeaderboardMe";
import {
  useCurrentSprint,
  useCurrentSprintStatus,
  useSprintLeaderboard,
} from "@/hooks/useCurrentSprint";
import { useAchievements } from "@/hooks/useAchievements";
import { getLeaderboard } from "@/api/dashboardApi";
import { queryKeys } from "@/hooks/queryKeys";
import { useQuery } from "@tanstack/react-query";

export function useDashboard() {
  const summaryQuery = usePortfolioSummary();
  const leaderboardMeQuery = useLeaderboardMe();
  const leaderboardQuery = useQuery({
    queryKey: queryKeys.dashboardLeaderboard,
    queryFn: ({ signal }) => getLeaderboard(signal),
    retry: 1,
  });
  const { currentSprint, isLoading: sprintLoading, isError: sprintError } = useCurrentSprint();
  const sprintStatusQuery = useCurrentSprintStatus(currentSprint?.id);
  const sprintLeaderboardQuery = useSprintLeaderboard(currentSprint?.id);
  const achievementsQuery = useAchievements();

  const isLoading =
    summaryQuery.isLoading ||
    leaderboardMeQuery.isLoading ||
    sprintLoading ||
    achievementsQuery.isLoading;

  const isError =
    summaryQuery.isError &&
    leaderboardMeQuery.isError &&
    achievementsQuery.isError;

  return {
    portfolioSummary: summaryQuery.data ?? null,
    leaderboardMe: leaderboardMeQuery.data ?? null,
    leaderboard: leaderboardQuery.data ?? null,
    currentSprint: currentSprint ?? null,
    sprintStatus: sprintStatusQuery.data ?? null,
    sprintLeaderboard: sprintLeaderboardQuery.data ?? null,
    achievements: achievementsQuery.data ?? null,
    isLoading,
    isError,
    errors: {
      summary: summaryQuery.isError ? summaryQuery.error : null,
      leaderboard: leaderboardMeQuery.isError ? leaderboardMeQuery.error : null,
      sprint: sprintError,
      sprintLeaderboard: sprintLeaderboardQuery.isError
        ? sprintLeaderboardQuery.error
        : null,
      achievements: achievementsQuery.isError ? achievementsQuery.error : null,
    },
    refetch: {
      summary: summaryQuery.refetch,
      leaderboard: async () => {
        await Promise.all([
          leaderboardMeQuery.refetch(),
          leaderboardQuery.refetch(),
        ]);
      },
      sprint: async () => {
        await Promise.all([
          sprintStatusQuery.refetch(),
          sprintLeaderboardQuery.refetch(),
        ]);
      },
      achievements: achievementsQuery.refetch,
      all: async () => {
        await Promise.all([
          summaryQuery.refetch(),
          leaderboardMeQuery.refetch(),
          leaderboardQuery.refetch(),
          sprintStatusQuery.refetch(),
          sprintLeaderboardQuery.refetch(),
          achievementsQuery.refetch(),
        ]);
      },
    },
  };
}
