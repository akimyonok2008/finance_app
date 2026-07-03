export type Instrument = {
  symbol: string;
  display_symbol: string;
  name: string;
  exchange: string;
  country: string;
  currency: string;
  asset_type: "stock" | "etf" | "fund" | "crypto" | "forex" | "other" | string;
  provider: string;
};

export type InstrumentSearchResponse = {
  results: Instrument[];
  source: "cache" | "provider" | "mixed" | string;
};
