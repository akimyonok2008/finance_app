import { apiRequest } from "@/api/client";
import type { QuotesResponse } from "@/types/quotes";

export function getQuotes(
  symbols: string[],
  signal?: AbortSignal,
): Promise<QuotesResponse> {
  const unique = Array.from(new Set(symbols.map((s) => s.trim().toUpperCase()).filter(Boolean)));
  return apiRequest<QuotesResponse>(
    `/quotes?symbols=${encodeURIComponent(unique.join(","))}`,
    { signal },
  );
}
