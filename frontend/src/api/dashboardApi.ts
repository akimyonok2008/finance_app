import { apiRequest } from "@/api/client";
import type {
  Achievement,
  Competition,
  DashboardPortfolioSummary,
  LeaderboardEntry,
  LeaderboardMe,
  MyCompetitionStatus,
  SprintLeaderboardEntry,
} from "@/types/dashboard";

export function getDashboardPortfolioSummary(
  signal?: AbortSignal,
): Promise<DashboardPortfolioSummary> {
  return apiRequest<DashboardPortfolioSummary>("/portfolio/summary", { signal });
}

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
 * Derive the current user's leaderboard entry from GET /leaderboard.
 * The backend has no /leaderboard/me endpoint yet — we match by display_name
 * from the auth context, which is passed in. If no match is found we return a
 * safe empty state so the widget degrades gracefully.
 *
 * TODO: Replace this display-name lookup with GET /leaderboard/me when the
 * backend provides a personalized ranking endpoint.
 */
export async function getLeaderboardMe(
  displayName: string | undefined,
  signal?: AbortSignal,
): Promise<LeaderboardMe> {
  const board = await getLeaderboard(signal);

  const total = board.length;

  if (!displayName) {
    return { rank: null, total_participants: total };
  }

  const entry = board.find(
    (e) => e.display_name.toLowerCase() === displayName.toLowerCase(),
  );

  if (!entry) {
    return { rank: null, total_participants: total };
  }

  return {
    rank: entry.rank,
    total_participants: total,
    display_name: entry.display_name,
    avatar_key: entry.avatar_key,
    gain_loss_percentage: entry.gain_loss_percentage,
    portfolio_index: entry.portfolio_index,
    rank_delta: null,
  };
}
