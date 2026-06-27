import { keepPreviousData, useQuery } from "@tanstack/react-query";

import { getExploreProfiles } from "@/api/explore";
import { queryKeys } from "@/hooks/queryKeys";
import type { ExploreParams } from "@/types/explore";

export function useExplore(params: ExploreParams) {
  return useQuery({
    queryKey: queryKeys.exploreProfiles(params),
    queryFn: ({ signal }) => getExploreProfiles(params, signal),
    placeholderData: keepPreviousData,
  });
}
