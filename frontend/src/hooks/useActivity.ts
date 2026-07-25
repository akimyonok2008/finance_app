import { useQuery } from "@tanstack/react-query";

import {
  getActivityDetail,
  getActivityList,
} from "@/api/activity";
import { queryKeys } from "@/hooks/queryKeys";
import type { ActivityListParams } from "@/types/activity";

export function useActivityList(params: ActivityListParams = {}, enabled = true) {
  return useQuery({
    queryKey: queryKeys.activity.list(params),
    queryFn: ({ signal }) => getActivityList(params, signal),
    enabled,
  });
}

export function useActivityDetail(id?: string) {
  return useQuery({
    queryKey: queryKeys.activity.detail(id ?? ""),
    queryFn: ({ signal }) => getActivityDetail(id!, signal),
    enabled: Boolean(id),
  });
}
