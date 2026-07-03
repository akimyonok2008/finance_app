import { useQuery } from "@tanstack/react-query";

import { searchInstruments } from "@/api/instruments";
import { queryKeys } from "@/hooks/queryKeys";

export function useInstrumentSearch(query: string) {
  return useQuery({
    queryKey: queryKeys.instrumentSearch(query),
    queryFn: ({ signal }) => searchInstruments(query, signal),
    enabled: query.trim().length >= 1,
    staleTime: 5 * 60_000,
  });
}
