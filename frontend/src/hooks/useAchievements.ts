import { useQuery } from "@tanstack/react-query";
import { getAchievements } from "@/api/dashboardApi";
import { queryKeys } from "@/hooks/queryKeys";

export function useAchievements() {
  return useQuery({
    queryKey: queryKeys.achievements,
    queryFn: ({ signal }) => getAchievements(signal),
    retry: 1,
  });
}
