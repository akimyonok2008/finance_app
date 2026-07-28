import { apiRequest } from "@/api/client";
import {
  getGlobalLeaderboard,
  getLeaderboardStanding,
} from "@/api/leaderboardApi";
import { decimalToChartNumber } from "@/utils/decimal";
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

export async function getLeaderboard(signal?: AbortSignal): Promise<LeaderboardEntry[]> {
  const { entries } = await getGlobalLeaderboard({ timeframe: "ALL" }, signal);
  return entries.map((entry) => {
    const gainLossPercentage = decimalToChartNumber(entry.ranked_return_percentage);
    const portfolioIndex = decimalToChartNumber(entry.ranked_index);
    if (gainLossPercentage === null || portfolioIndex === null) {
      throw new Error("Contract violation in GET /leaderboard: ranked decimal is out of display range");
    }
    return {
      rank: entry.rank,
      display_name: entry.display_name,
      avatar_key: entry.avatar_key ?? "",
      gain_loss_percentage: gainLossPercentage,
      portfolio_index: portfolioIndex,
    };
  });
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
  const gainLossPercentage = decimalToChartNumber(
    standing.ranked_return_percentage,
  );
  const portfolioIndex = decimalToChartNumber(standing.ranked_index);
  if (gainLossPercentage === null || portfolioIndex === null) {
    throw new Error(
      "Contract violation in GET /leaderboard/me: ranked decimal is out of display range",
    );
  }
  return {
    rank: standing.rank,
    total_participants: standing.total_participants,
    // LeaderboardMe is a display-only aggregate DTO (not itself a backend
    // response); converting here is the sanctioned chart/display boundary.
    gain_loss_percentage: gainLossPercentage,
    portfolio_index: portfolioIndex,
    rank_delta: null,
  };
}
