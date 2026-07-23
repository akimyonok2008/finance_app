import { apiRequest } from "@/api/client";
import type { Achievement } from "@/types/dashboard";

/**
 * Re-evaluates the authenticated user's benchmark badges and returns the full
 * catalogue with unlock state + award evidence. POST is used (over GET) so newly
 * earned badges are reflected immediately.
 */
export function evaluateAchievements(signal?: AbortSignal): Promise<Achievement[]> {
  return apiRequest<Achievement[]>("/achievements/evaluate", {
    method: "POST",
    signal,
  });
}

/** Lists badges with the user's current unlock state without re-evaluating. */
export function listAchievements(signal?: AbortSignal): Promise<Achievement[]> {
  return apiRequest<Achievement[]>("/achievements", { signal });
}
