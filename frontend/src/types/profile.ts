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
  public_weights: PublicWeight[];
  asset_type_exposure: Exposure[];
  currency_exposure: Exposure[];
  concentration?: Concentration;
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
  handle?: string;
  display_name?: string;
  avatar_key?: string;
  bio?: string;
  strategy_tag?: string;
  is_public?: boolean;
  show_public_weights?: boolean;
};
