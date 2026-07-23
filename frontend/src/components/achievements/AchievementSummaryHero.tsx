import { Trophy } from "lucide-react";

import type { AchievementsSummary } from "@/types/achievements";

export function AchievementSummaryHero({
  summary,
}: {
  summary: AchievementsSummary;
}) {
  const pct =
    summary.total > 0 ? Math.round((summary.unlocked / summary.total) * 100) : 0;

  return (
    <header className="relative overflow-hidden rounded-2xl border border-violet-300/15 bg-[radial-gradient(circle_at_8%_0%,rgba(167,139,250,0.13),transparent_36%),radial-gradient(circle_at_92%_100%,rgba(34,211,238,0.07),transparent_38%),rgba(24,24,27,0.46)] p-5 shadow-xl shadow-black/15 sm:flex sm:items-end sm:justify-between sm:gap-5 sm:p-6">
      <div>
        <div className="mb-2 flex items-center gap-2 text-[10px] font-semibold uppercase tracking-[0.2em] text-violet-300/70">
          <Trophy className="h-3.5 w-3.5" /> Benchmark achievements
        </div>
        <h1 className="achievements-display text-3xl font-semibold tracking-tight text-zinc-50 sm:text-4xl">
          Achievements
        </h1>
        <p className="mt-2 text-sm leading-6 text-zinc-400">
          Track benchmark wins and investor-inspired badges.
        </p>
      </div>

      <div className="mt-5 w-full rounded-xl border border-cyan-300/10 bg-zinc-950/30 px-4 py-3 sm:mt-0 sm:w-56 sm:text-right">
        <p className="font-mono text-xl font-medium tabular-nums text-zinc-100">
          {summary.unlocked} / {summary.total} unlocked
        </p>
        <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-zinc-800">
          <div
            className="h-full rounded-full bg-gradient-to-r from-violet-400 via-cyan-400 to-emerald-400 transition-all"
            style={{ width: `${pct}%` }}
          />
        </div>
      </div>
    </header>
  );
}
