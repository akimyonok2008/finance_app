export type DashboardPortfolioSummary = {
  user_id?: string;
  portfolio_id?: string;
  base_currency?: string;
  total_cost_basis: number;
  current_value: number;
  gain_loss: number;
  gain_loss_percentage: number;
  portfolio_index: number;
};

export type LeaderboardEntry = {
  rank: number;
  display_name: string;
  avatar_key: string;
  gain_loss_percentage: number;
  portfolio_index: number;
};

export type LeaderboardMe = {
  rank: number | null;
  total_participants: number | null;
  display_name?: string;
  avatar_key?: string;
  gain_loss_percentage?: number;
  portfolio_index?: number;
  rank_delta?: number | null;
};

export type SprintLeaderboardEntry = {
  rank: number;
  display_name: string;
  avatar_key: string;
  sprint_return_percentage: number;
  sprint_index: number;
};

export type Competition = {
  id: string;
  name: string;
  type: string;
  starts_at: string;
  ends_at: string;
  status: "upcoming" | "active" | "completed" | string;
};

export type MyCompetitionStatus = {
  competition_id: string;
  joined: boolean;
  current_rank: number;
  sprint_return_percentage: number;
  sprint_index: number;
};

export type AchievementEvidence = {
  period: string;
  start_date: string;
  end_date: string;
  portfolio_return_pct: number;
  benchmark_return_pct: number;
  edge_points: number;
  benchmark_recipe_id: string;
};

export type AchievementBenchmarkProgress = {
  state: string;
  progress_percentage: number;
  history_coverage_percentage: number;
  portfolio_return_percentage?: number;
  benchmark_return_percentage?: number;
  current_edge_points?: number;
  start_date?: string;
  end_date?: string;
  reason: string;
};

export type Achievement = {
  key: string;
  name: string;
  description: string;
  icon_key: string;
  difficulty?: string;
  period?: string;
  inspired_by?: string;
  unlocked: boolean;
  unlocked_at?: string | null;
  evidence?: AchievementEvidence | null;
  progress?: AchievementBenchmarkProgress | null;
};

export type PerformanceTone = "positive" | "negative" | "neutral";
