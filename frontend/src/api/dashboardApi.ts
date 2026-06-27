import { apiRequest } from "@/api/client";
import { getLeaderboardStanding } from "@/api/leaderboardApi";
import type {
  Achievement,
  Competition,
  LeaderboardEntry,
  LeaderboardMe,
  MyCompetitionStatus,
  SprintLeaderboardEntry,
} from "@/types/dashboard";

export function getCompetitions(signal?: AbortSignal): Promise<Competition[]> {
  return apiRequest<Competition[]>("/competitions", { signal });
}

export function getMyCompetitionStatus(
  competitionId: string,
  signal?: AbortSignal,
): Promise<MyCompetitionStatus> {
  return apiRequest<MyCompetitionStatus>(
    `/competitions/${competitionId}/me`,
    { signal },
  );
}

export function getAchievements(signal?: AbortSignal): Promise<Achievement[]> {
  return apiRequest<Achievement[]>("/achievements", { signal });
}

export function getLeaderboard(signal?: AbortSignal): Promise<LeaderboardEntry[]> {
  return apiRequest<LeaderboardEntry[]>("/leaderboard", { signal });
}

export function getSprintLeaderboard(
  competitionId: string,
  signal?: AbortSignal,
): Promise<SprintLeaderboardEntry[]> {
  return apiRequest<SprintLeaderboardEntry[]>(
    `/competitions/${competitionId}/leaderboard`,
    { signal },
  );
}

/**
 * The current user's ranking, from GET /leaderboard/me (all-time). The backend
 * returns the exact rank + participant count, so no display-name matching is
 * needed. rank_delta is not tracked yet, so it stays null.
 */
export async function getLeaderboardMe(
  signal?: AbortSignal,
): Promise<LeaderboardMe> {
  const standing = await getLeaderboardStanding("ALL", signal);
  return {
    rank: standing.rank,
    total_participants: standing.total_participants,
    gain_loss_percentage: standing.ranked_return_percentage,
    portfolio_index: standing.ranked_index,
    rank_delta: null,
  };
}
