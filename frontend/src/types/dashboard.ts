import type { PortfolioSummary } from "@/types/portfolio";

export type DashboardPortfolioSummary = PortfolioSummary;

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
  evaluation_model?: string;
  evidence_version?: number;
  tracking_epoch?: string;
  start_ranked_index?: number;
  end_ranked_index?: number;
  start_snapshot_at?: string;
  end_snapshot_at?: string;
  active_coverage_percentage?: number;
  trusted_data_coverage_percentage?: number;
  benchmark_data_source?: string;
  snapshot_frequency?: string;
  // Benchmark data-integrity provenance (evidence v2).
  benchmark_recipe_version?: string;
  verification?: string;
  rebalancing_policy?: string;
  benchmark_input_hash?: string;
  benchmark_data?: {
    providers?: string[];
    symbols?: string[];
    price_type?: string;
    quality?: string;
    is_synthetic?: boolean;
    used_stale_data?: boolean;
    includes_dividends?: boolean;
    includes_splits?: boolean;
    total_return_method?: string;
    recipe_source_type?: string;
    recipe_source_accession?: string;
    recipe_source_url?: string;
    recipe_report_period_end?: string;
    recipe_publicly_known_at?: string;
    mapping_coverage_percentage?: number;
  };
};

export type AchievementBenchmarkProgress = {
  state: string;
  progress_percentage: number;
  history_coverage_percentage: number;
  active_coverage_percentage: number;
  trusted_data_coverage_percentage: number;
  portfolio_return_percentage?: number;
  benchmark_return_percentage?: number;
  current_edge_points?: number;
  start_date?: string;
  end_date?: string;
  effective_start_at?: string;
  effective_end_at?: string;
  latest_snapshot_at?: string;
  required_edge_points: number;
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
  legacy_evidence?: boolean;
};

export type PerformanceTone = "positive" | "negative" | "neutral";
