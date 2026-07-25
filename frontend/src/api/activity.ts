import { apiRequest } from "@/api/client";
import type {
  ActivityListParams,
  ActivityListResponse,
} from "@/types/activity";
import type { PortfolioActivity } from "@/types/portfolio";

export function getActivityList(
  params: ActivityListParams = {},
  signal?: AbortSignal,
): Promise<ActivityListResponse> {
  const query = new URLSearchParams();
  if (params.category && params.category !== "all") query.set("category", params.category);
  if (params.symbol) query.set("symbol", params.symbol);
  if (params.limit) query.set("limit", String(params.limit));
  if (params.offset) query.set("offset", String(params.offset));
  const suffix = query.size ? `?${query.toString()}` : "";
  return apiRequest<ActivityListResponse>(`/activity${suffix}`, { signal });
}

export function getActivityDetail(
  id: string,
  signal?: AbortSignal,
): Promise<PortfolioActivity> {
  return apiRequest<PortfolioActivity>(`/activity/${id}`, { signal });
}
