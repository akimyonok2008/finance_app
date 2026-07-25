import type { PortfolioActivity } from "@/types/portfolio";

export type ActivityCategory =
  | "all"
  | "cash_flows"
  | "trades"
  | "income"
  | "fees"
  | "automatic_adjustments"
  | "corrections";

export type ActivityListResponse = {
  items: PortfolioActivity[];
  next_offset: number | null;
  total: number;
};

export type ActivityListParams = {
  category?: ActivityCategory;
  symbol?: string;
  limit?: number;
  offset?: number;
};
