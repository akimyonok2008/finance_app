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
