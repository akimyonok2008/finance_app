/**
 * Centralized query keys. Position changes ripple into performance ranking and
 * badges, so mutations invalidate all four of these.
 */
export const queryKeys = {
  positions: ["positions"] as const,
  closedPositions: ["positions", "closed"] as const,
  portfolioSummary: ["portfolioSummary"] as const,
  portfolioArchives: (timeframe: string) => ["portfolioArchives", timeframe] as const,
  leaderboard: ["leaderboard"] as const,
  dashboardLeaderboard: ["leaderboard", "dashboard"] as const,
  leaderboardMe: ["leaderboardMe"] as const,
  leaderboardStanding: (timeframe: string) => ["leaderboardMe", timeframe] as const,
  achievements: ["achievements"] as const,
  competitions: ["competitions"] as const,
  currentSprintStatus: (id: string) => ["currentSprintStatus", id] as const,
  sprintLeaderboard: (id: string) => ["sprintLeaderboard", id] as const,
  myProfile: ["profile", "me"] as const,
  publicProfile: (handle: string) => ["profile", handle] as const,
  exploreProfiles: (params: object) => ["exploreProfiles", params] as const,
  instrumentSearch: (q: string) => ["instruments", "search", q] as const,
  quotes: (symbols: string[]) => ["quotes", symbols] as const,
  followState: (handle: string) => ["social", "followState", handle] as const,
  friends: ["social", "friends"] as const,
  following: ["social", "following"] as const,
  followers: ["social", "followers"] as const,
  dmConversations: ["dm", "conversations"] as const,
  dmMessages: (conversationId: string) => ["dm", "messages", conversationId] as const,
};

/** Queries to invalidate after any successful position mutation. */
export const POSITION_MUTATION_INVALIDATIONS = [
  queryKeys.positions,
  queryKeys.closedPositions,
  queryKeys.portfolioSummary,
  ["portfolioArchives"],
  queryKeys.leaderboard,
  queryKeys.leaderboardMe,
  queryKeys.achievements,
  queryKeys.myProfile,
] as const;
