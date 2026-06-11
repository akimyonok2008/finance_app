/**
 * Centralized query keys. Position changes ripple into performance ranking and
 * badges, so mutations invalidate all four of these.
 */
export const queryKeys = {
  positions: ["positions"] as const,
  portfolioSummary: ["portfolioSummary"] as const,
  leaderboard: ["leaderboard"] as const,
  dashboardLeaderboard: ["leaderboard", "dashboard"] as const,
  leaderboardMe: ["leaderboardMe"] as const,
  achievements: ["achievements"] as const,
  competitions: ["competitions"] as const,
  currentSprintStatus: (id: string) => ["currentSprintStatus", id] as const,
  sprintLeaderboard: (id: string) => ["sprintLeaderboard", id] as const,
};

/** Queries to invalidate after any successful position mutation. */
export const POSITION_MUTATION_INVALIDATIONS = [
  queryKeys.positions,
  queryKeys.portfolioSummary,
  queryKeys.leaderboard,
  queryKeys.leaderboardMe,
  queryKeys.achievements,
] as const;
