import { Trophy } from "lucide-react";

import { sortAchievements } from "@/pages/arena/arenaUtils";
import { BadgeCard } from "@/pages/arena/BadgeCard";
import type { Achievement } from "@/types/arena";

export function TrophyCase({
  achievements,
  isError,
}: {
  achievements: Achievement[] | undefined;
  isLoading?: boolean;
  isError?: boolean;
}) {
  const sorted = sortAchievements(achievements ?? []);
  const unlocked = sorted.filter((achievement) => achievement.isUnlocked).length;

  return (
    <section
      aria-labelledby="trophy-heading"
      className="rounded-2xl border border-zinc-800 bg-zinc-900/50 p-5 shadow-sm shadow-black/20"
    >
      <div className="mb-5 flex items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <div className="grid h-9 w-9 place-items-center rounded-lg border border-zinc-800 bg-zinc-950/50 text-violet-300">
            <Trophy className="h-4 w-4" />
          </div>
          <div>
            <h2 id="trophy-heading" className="text-sm font-semibold text-zinc-100">
              Trophy case
            </h2>
          </div>
        </div>
        <span className="font-mono text-xs tabular-nums text-zinc-400">
          Unlocked {unlocked} / {sorted.length}
        </span>
      </div>

      {isError ? (
        <p className="py-10 text-center text-sm text-rose-300">
          Achievements are temporarily unavailable.
        </p>
      ) : sorted.length === 0 ? (
        <p className="py-10 text-center text-sm text-zinc-500">
          No achievements available yet.
        </p>
      ) : (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-1">
          {sorted.map((achievement, index) => (
            <BadgeCard
              key={achievement.id}
              achievement={achievement}
              index={index}
            />
          ))}
        </div>
      )}
    </section>
  );
}
