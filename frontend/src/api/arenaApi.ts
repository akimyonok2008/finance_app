import { ApiError, apiRequest } from "@/api/client";
import type {
  Achievement,
  ActiveSprint,
  CohortLeaderboardEntry,
  JoinSprintResponse,
} from "@/types/arena";
import type {
  Achievement as LegacyAchievement,
  Competition,
  MyCompetitionStatus,
  SprintLeaderboardEntry,
} from "@/types/dashboard";

const STANDARD_RULES = [
  "Your portfolio baseline is locked when you join.",
  "Rankings use percentage performance only.",
  "Holdings and portfolio values always remain private.",
];

export async function getActiveSprint(): Promise<ActiveSprint> {
  const competitions = await apiRequest<Competition[]>("/competitions");
  const active = competitions.find(
    (competition) =>
      competition.type === "weekly_sprint" && competition.status === "active",
  );
  if (!active) {
    throw new ApiError("No active sprint right now.", 404);
  }
  const status = await apiRequest<MyCompetitionStatus>(
    `/competitions/${active.id}/me`,
  );
  return {
    id: active.id,
    name: active.name,
    endsAt: active.ends_at,
    rules: STANDARD_RULES,
    isJoined: status.joined,
  };
}

export async function getCohortLeaderboard(): Promise<
  CohortLeaderboardEntry[]
> {
  const sprint = await getActiveSprint();
  const board = await apiRequest<SprintLeaderboardEntry[]>(
    `/competitions/${sprint.id}/leaderboard`,
  );
  return board.map((entry) => ({
    rank: entry.rank,
    username: entry.display_name,
    roi: entry.sprint_return_percentage,
    isCurrentUser: false,
  }));
}

export async function getAchievements(): Promise<Achievement[]> {
  const achievements = await apiRequest<LegacyAchievement[]>(
    "/achievements/evaluate",
    { method: "POST" },
  );
  return achievements.map((achievement) => ({
    id: achievement.key,
    name: achievement.name,
    description: achievement.description,
    currentProgress: achievement.unlocked ? 1 : 0,
    targetProgress: 1,
    isUnlocked: achievement.unlocked,
    unlockedAt: achievement.unlocked_at ?? undefined,
  }));
}

export async function joinSprint(
  sprintId: string,
): Promise<JoinSprintResponse> {
  const response = await apiRequest<{
    competition_id: string;
    joined: boolean;
  }>(`/competitions/${sprintId}/join`, { method: "POST" });
  return {
    success: response.joined,
    sprintId: response.competition_id,
    isJoined: response.joined,
  };
}
