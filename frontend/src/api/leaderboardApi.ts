import { apiRequest } from "@/api/client";
import {
  assertShape,
  leaderboardResponseSchema,
  leaderboardStandingSchema,
} from "@/api/schemas";
import type {
  LeaderboardGlobalResponse,
  LeaderboardQueryParams,
  LeaderboardStanding,
} from "@/types/leaderboard";

export async function getGlobalLeaderboard(
  params: LeaderboardQueryParams,
  signal?: AbortSignal,
): Promise<LeaderboardGlobalResponse> {
  const response = await apiRequest<unknown>(
    `/leaderboard?timeframe=${encodeURIComponent(params.timeframe)}`,
    { signal },
  );
  const entries = assertShape(
    leaderboardResponseSchema,
    response,
    "GET /leaderboard",
  );
  return {
    timeframe: params.timeframe,
    entries,
  };
}

export async function getLeaderboardStanding(
  timeframe: LeaderboardQueryParams["timeframe"],
  signal?: AbortSignal,
): Promise<LeaderboardStanding> {
  const response = await apiRequest<unknown>(
    `/leaderboard/me?timeframe=${encodeURIComponent(timeframe)}`,
    { signal },
  );
  const standing = assertShape(
    leaderboardStandingSchema,
    response,
    "GET /leaderboard/me",
  );
  if (standing.timeframe !== timeframe) {
    throw new Error(
      `Contract violation in GET /leaderboard/me: timeframe: expected ${timeframe}, received ${standing.timeframe}`,
    );
  }
  return standing;
}
