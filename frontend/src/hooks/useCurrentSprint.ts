import { useQuery } from "@tanstack/react-query";
import {
  getCompetitions,
  getMyCompetitionStatus,
  getSprintLeaderboard,
} from "@/api/dashboardApi";
import { queryKeys } from "@/hooks/queryKeys";

export function useCompetitions() {
  return useQuery({
    queryKey: queryKeys.competitions,
    queryFn: ({ signal }) => getCompetitions(signal),
    retry: 1,
  });
}

export function useCurrentSprint() {
  const { data: competitions, ...rest } = useCompetitions();
  const sprint = competitions?.find(
    (c) => c.status === "active" && c.type === "weekly_sprint",
  ) ?? null;
  return { currentSprint: sprint, ...rest };
}

export function useCurrentSprintStatus(competitionId: string | undefined) {
  return useQuery({
    queryKey: competitionId
      ? queryKeys.currentSprintStatus(competitionId)
      : ["currentSprintStatus", ""],
    queryFn: ({ signal }) =>
      getMyCompetitionStatus(competitionId!, signal),
    enabled: Boolean(competitionId),
    retry: 1,
  });
}

export function useSprintLeaderboard(competitionId: string | undefined) {
  return useQuery({
    queryKey: competitionId
      ? queryKeys.sprintLeaderboard(competitionId)
      : ["sprintLeaderboard", ""],
    queryFn: ({ signal }) => getSprintLeaderboard(competitionId!, signal),
    enabled: Boolean(competitionId),
    retry: 1,
  });
}
