export const STRATEGY_TAGS = [
  "conservative",
  "balanced_global",
  "growth",
  "dividend_income",
  "tech_focused",
  "value",
  "crypto_heavy",
  "esg",
  "active_trader",
  "long_term_investor",
] as const;

export type StrategyTag = (typeof STRATEGY_TAGS)[number];

export type PublicWeight = {
  symbol: string;
  weight_percentage: number;
  asset_type?: string;
};

export type Exposure = {
  name: string;
  weight_percentage: number;
};

export type Concentration = {
  position_count?: number;
  largest_weight_percentage?: number;
  top3_weight_percentage?: number;
};

export type ProfileBadge = {
  key: string;
  name: string;
  icon_key?: string;
  unlocked_at?: string;
};

export type PerformanceHistoryPoint = {
  captured_at: string;
  portfolio_index: number;
  return_percentage: number;
};

export type PublicClosedPosition = {
  symbol: string;
  asset_type?: string;
  closed_at?: string;
  return_percentage: number;
};

export type ProfileDNAScores = {
  growth: number;
  income: number;
  commodities: number;
  defensive: number;
  international: number;
  concentration: number;
  volatility: number;
};

export type ProfileDNAExplanations = {
  growth: string[];
  income: string[];
  commodities: string[];
  defensive: string[];
  international: string[];
  concentration: string[];
  volatility: string[];
};

export type ProfilePerformanceDrivers = {
  summary: string;
  positive_drivers: string[];
  negative_drivers: string[];
  open_contribution_points: number;
  closed_contribution_points: number;
};

export type ProfileBenchmark = {
  symbol: string;
  name: string;
  index: number;
  edge_points: number;
};

export type ProfileBenchmarkContext = {
  investor_index: number;
  benchmarks: ProfileBenchmark[];
  note?: string;
};

export type ProfileContributor = {
  symbol: string;
  name?: string;
  contribution_points: number;
};

export type ProfileOpenClosedPerformance = {
  open_return_percentage: number;
  closed_return_percentage: number;
  open_contribution_points: number;
  closed_contribution_points: number;
  has_closed_positions: boolean;
  composition_visible: boolean;
  // True when open_return_percentage or closed_return_percentage above is
  // built from at least one user-entered execution price rather than a
  // provider estimate. Unlike the ranked index (always priced from tracked
  // market quotes), these two figures are unverifiable whenever this is true.
  includes_self_reported_prices: boolean;
};

export type ProfileInsights = {
  investment_style: string;
  style_summary: string;
  focus_areas: string[];
  dna: ProfileDNAScores;
  dna_explanations: ProfileDNAExplanations;
  performance_drivers: ProfilePerformanceDrivers;
  benchmark_context: ProfileBenchmarkContext;
  contributors: ProfileContributor[];
  detractors: ProfileContributor[];
  open_closed_performance: ProfileOpenClosedPerformance;
};

export type PublicProfile = {
  handle: string;
  display_name: string;
  avatar_key?: string;
  bio?: string;
  strategy_tag?: string;
  joined_at?: string;
  portfolio_index?: number;
  return_percentage?: number;
  global_rank?: number | null;
  sprint_rank?: number | null;
  badges: ProfileBadge[];
  performance_history: PerformanceHistoryPoint[];
  public_closed_positions: PublicClosedPosition[];
  public_weights: PublicWeight[];
  asset_type_exposure: Exposure[];
  currency_exposure: Exposure[];
  concentration?: Concentration;
  insights: ProfileInsights;
};

export type MyProfile = {
  handle: string;
  display_name: string;
  avatar_key?: string;
  bio?: string;
  strategy_tag?: string;
  is_public: boolean;
  show_public_weights: boolean;
  created_at?: string;
  updated_at?: string;
  public_preview: PublicProfile;
};

export type UpdateProfileRequest = {
  display_name?: string;
  bio?: string;
  strategy_tag?: string;
  is_public?: boolean;
  show_public_weights?: boolean;
};
