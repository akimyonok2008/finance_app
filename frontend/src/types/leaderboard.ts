import type { ProfileBadge } from "@/types/profile";

// Public ranking projections only. Never add quantities, values, cost basis,
// prices, emails, user ids, or portfolio ids to this contract.
export type LeaderboardTimeframe = "1W" | "1M" | "3M" | "6M" | "1Y" | "ALL";

export type PublicAssetType =
  | "stock"
  | "etf"
  | "crypto"
  | "fund"
  | "cash"
  | "other";

/** Public composition weight: symbol + asset type + percentage only. */
export type PublicWeight = {
  symbol: string;
  asset_type: PublicAssetType;
  weight_percentage: number;
};

export type LeaderboardEntry = {
  rank: number;
  display_name: string;
  handle?: string;
  avatar_key?: string;
  strategy_tag?: string;
  ranked_index: number;
  ranked_return_percentage: number;
  public_weights: PublicWeight[];
  badges: ProfileBadge[];
  is_me?: boolean;
};

export type LeaderboardGlobalResponse = {
  timeframe: LeaderboardTimeframe;
  entries: LeaderboardEntry[];
};

export type LeaderboardQueryParams = {
  timeframe: LeaderboardTimeframe;
};

export type LeaderboardMilestone = {
  label: string;
  target_rank: number;
  rank_gap: number;
  return_gap_percentage: number;
};

/** GET /leaderboard/me — the caller's own standing for a timeframe. */
export type LeaderboardStanding = {
  timeframe: LeaderboardTimeframe;
  eligible: boolean;
  rank: number | null;
  previous_rank: number | null;
  rank_delta: number | null;
  best_rank: number | null;
  participant_count: number;
  total_participants: number;
  percentile: number;
  ranked_return_percentage: number;
  ranked_index: number;
  /** True when the portfolio is empty: ranked tracking is paused, index preserved. */
  paused: boolean;
  next_milestone: LeaderboardMilestone | null;
  reason: string;
};
