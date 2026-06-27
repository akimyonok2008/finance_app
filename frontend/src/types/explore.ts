export type ExploreSort = "top" | "return" | "rank" | "recent";

export type ExplorePublicWeight = {
  symbol: string;
  weight_percentage: number;
  asset_type?: string;
};

export type ExploreBadge = {
  key: string;
  name: string;
  icon_key?: string;
};

export type ExploreConcentration = {
  position_count?: number;
  largest_weight_percentage?: number;
  top3_weight_percentage?: number;
};

export type ExploreProfile = {
  handle: string;
  display_name: string;
  avatar_key?: string;
  bio?: string;
  strategy_tag?: string;
  ranked_index?: number;
  ranked_return_percentage?: number;
  global_rank?: number | null;
  sprint_rank?: number | null;
  public_weights: ExplorePublicWeight[];
  concentration?: ExploreConcentration;
  badges: ExploreBadge[];
};

export type TrendingHolding = {
  symbol: string;
  profile_count: number;
  average_weight_percentage?: number;
  top10_count?: number;
  asset_type?: string;
};

export type ExplorePagination = {
  limit: number;
  offset: number;
  total?: number;
  has_more?: boolean;
};

export type ExploreResponse = {
  featured: ExploreProfile[];
  similar: ExploreProfile[];
  top_performers: ExploreProfile[];
  trending_holdings: TrendingHolding[];
  pagination?: ExplorePagination;
};

export type ExploreParams = {
  q?: string;
  symbol?: string;
  sort?: ExploreSort;
  limit?: number;
  offset?: number;
};
