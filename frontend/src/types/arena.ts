// Arena deliberately shows rank and percentage performance only. The broader
// product policy permits symbols on separately opted-in public profiles, but
// this DTO must never render portfolio values, holdings, quantities, symbols,
// average buy prices, email, user id, or portfolio id.

export type ActiveSprint = {
  id: string;
  name: string;
  endsAt: string;
  rules: string[];
  isJoined: boolean;
};

export type CohortLeaderboardEntry = {
  rank: number;
  username: string;
  roi: number;
  isCurrentUser: boolean;
};

export type Achievement = {
  id: string;
  name: string;
  description: string;
  currentProgress: number;
  targetProgress: number;
  isUnlocked: boolean;
  unlockedAt?: string;
  difficulty?: string;
  period?: string;
  inspiredBy?: string;
  edgePoints?: number;
};

export type JoinSprintResponse = {
  success: boolean;
  sprintId: string;
  isJoined: boolean;
};
