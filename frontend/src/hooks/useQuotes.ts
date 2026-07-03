import { useQuery } from "@tanstack/react-query";

import { getQuotes } from "@/api/quotes";
import { queryKeys } from "@/hooks/queryKeys";

export function useQuotes(symbols: string[]) {
  const normalized = Array.from(new Set(symbols.map((s) => s.trim().toUpperCase()).filter(Boolean))).sort();
  return useQuery({
    queryKey: queryKeys.quotes(normalized),
    queryFn: ({ signal }) => getQuotes(normalized, signal),
    enabled: normalized.length > 0,
    staleTime: 5 * 60_000,
  });
}
