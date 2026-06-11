// Privacy guard: leaderboard UI must never include portfolio value, cost basis,
// holdings, symbols, quantities, average buy price, email, user id, or portfolio id.

export type LeaderboardTimeframe =
  | "daily"
  | "weekly"
  | "monthly"
  | "all_time";

export type LeaderboardEntry = {
  rank: number;
  display_name: string;
  avatar_key?: string;
  strategy_tag?: string;
  change_24h_percentage?: number | null;
  total_roi_percentage: number;
  portfolio_index?: number;
  is_me?: boolean;
};

export type LeaderboardGlobalResponse = {
  timeframe: LeaderboardTimeframe;
  page: number;
  page_size: number;
  total: number;
  entries: LeaderboardEntry[];
  me?: LeaderboardEntry | null;
};

export type LeaderboardQueryParams = {
  timeframe: LeaderboardTimeframe;
  page?: number;
  page_size?: number;
};
