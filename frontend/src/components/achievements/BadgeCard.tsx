import { ArrowUpRight } from "lucide-react";

import { BadgeDifficultyPill } from "@/components/achievements/BadgeDifficultyPill";
import { BadgeMark } from "@/components/achievements/BadgeMark";
import { BadgeProgressBar } from "@/components/achievements/BadgeProgressBar";
import { BadgeStatusPill } from "@/components/achievements/BadgeStatusPill";
import type { AchievementProgress } from "@/types/achievements";
import { cn } from "@/utils/cn";

export function BadgeCard({
  badge,
  onSelect,
}: {
  badge: AchievementProgress;
  onSelect: (badge: AchievementProgress) => void;
}) {
  const unlocked = badge.status === "unlocked";
  const hasProgress = typeof badge.progressPct === "number";

  return (
    <button
      type="button"
      onClick={() => onSelect(badge)}
      className={cn(
        "group flex h-full w-full flex-col rounded-2xl border p-4 text-left transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-300/30",
        unlocked
          ? "border-emerald-300/15 bg-[radial-gradient(circle_at_top_right,rgba(52,211,153,0.07),transparent_48%),rgba(24,24,27,0.42)] hover:border-emerald-300/25"
          : "border-zinc-800 bg-[radial-gradient(circle_at_top_right,rgba(129,140,248,0.045),transparent_48%),rgba(24,24,27,0.38)] hover:-translate-y-0.5 hover:border-violet-300/20",
      )}
    >
      <div className="flex items-start gap-3">
        <BadgeMark badgeId={badge.id} />
        <div className="min-w-0 flex-1">
          <h3 className="achievements-card-title truncate text-base font-semibold text-zinc-100">
            {badge.name}
          </h3>
          <p className="mt-0.5 truncate text-xs text-zinc-600">
            Inspired by {badge.inspiredBy}
          </p>
        </div>
        <ArrowUpRight className="h-3.5 w-3.5 shrink-0 text-zinc-700 transition group-hover:text-zinc-400" />
      </div>

      <div className="mt-3 flex flex-wrap items-center gap-1.5">
        <BadgeDifficultyPill difficulty={badge.difficulty} />
        <span className="rounded-full border border-zinc-800 px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide text-zinc-500">
          {badge.period}
        </span>
        <BadgeStatusPill status={badge.status} />
      </div>

      <p className="mt-3 line-clamp-2 text-xs leading-5 text-zinc-500">
        {badge.explanation}
      </p>

      <div className="mt-auto pt-4">
        {hasProgress ? (
          <div className="space-y-2">
            <BadgeProgressBar
              pct={badge.progressPct ?? 0}
              status={badge.status}
            />
            <p
              className={cn(
                "font-mono text-[11px] tabular-nums",
                unlocked ? "text-emerald-300" : "text-sky-300",
              )}
            >
              {Math.round(badge.progressPct ?? 0)}% complete
            </p>
          </div>
        ) : (
          <p className="text-[11px] leading-4 text-zinc-600">
            Progress unavailable
          </p>
        )}
      </div>
    </button>
  );
}
