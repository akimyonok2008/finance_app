export type Quote = {
  symbol: string;
  price: number;
  currency: string;
  change_percentage?: number;
  previous_close?: number;
  market_time?: string;
  provider: string;
  is_delayed: boolean;
  delay_minutes?: number;
  is_stale: boolean;
  fetched_at: string;
  expires_at: string;
  provider_status: string;
};

export type QuotesResponse = {
  quotes: Quote[];
  source: "cache" | "provider" | "mixed" | string;
};
