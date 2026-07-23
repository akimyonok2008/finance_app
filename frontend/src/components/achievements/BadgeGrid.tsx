import { BadgeCard } from "@/components/achievements/BadgeCard";
import type { AchievementProgress } from "@/types/achievements";

export function BadgeGrid({
  badges,
  onSelect,
}: {
  badges: AchievementProgress[];
  onSelect: (badge: AchievementProgress) => void;
}) {
  return (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
      {badges.map((badge) => (
        <BadgeCard key={badge.id} badge={badge} onSelect={onSelect} />
      ))}
    </div>
  );
}
