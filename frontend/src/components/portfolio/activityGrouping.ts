import type { PortfolioActivity } from "@/types/portfolio";

export type ActivityGroup = {
  key: string;
  activities: PortfolioActivity[];
};

export function groupActivities(items: PortfolioActivity[]): ActivityGroup[] {
  const groups = new Map<string, ActivityGroup>();
  for (const activity of items) {
    // Legacy rows without a durable group remain independent. Timestamp and
    // symbol are intentionally never used as grouping keys.
    const key = activity.group_id
      ? `group:${activity.group_id}`
      : `activity:${activity.id}`;
    const existing = groups.get(key);
    if (existing) existing.activities.push(activity);
    else groups.set(key, { key, activities: [activity] });
  }
  return [...groups.values()];
}
