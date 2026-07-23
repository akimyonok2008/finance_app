export type StrategySourceProfile = {
  handle: string;
  display_name: string;
  avatar_key?: string;
  strategy_tag?: string;
};

export type StrategyWeight = {
  symbol: string;
  asset_type: string;
  weight_percentage: number;
};

export type CopyPreviewResponse = {
  source_profile: StrategySourceProfile;
  weights: StrategyWeight[];
  disclaimer: string;
};

export type CopyFromProfileResponse = CopyPreviewResponse;

export type WeightDifference = {
  symbol: string;
  my_weight_percentage: number;
  target_weight_percentage: number;
  difference_percentage: number;
  asset_type?: string;
};

export type ConcentrationComparison = {
  my_position_count: number;
  target_position_count: number;
  my_top3_weight_percentage: number;
  target_top3_weight_percentage: number;
};

export type CompareProfileResponse = {
  target_profile: StrategySourceProfile;
  summary: string;
  overlap_score: number;
  shared_symbols: string[];
  weight_differences: WeightDifference[];
  concentration_comparison: ConcentrationComparison;
  learning_points: Array<{ title: string; body: string }>;
  disclaimer: string;
};
