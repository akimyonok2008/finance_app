import { ArrowDownRight, ArrowUpRight, Medal, Target } from "lucide-react";

import { Skeleton } from "@/components/ui/skeleton";
import type { LeaderboardStanding } from "@/types/leaderboard";
import { cn } from "@/utils/cn";
import { formatPercent } from "@/utils/formatPercent";

type YourStandingCardProps = {
  standing: LeaderboardStanding | undefined;
  isLoading: boolean;
  isError: boolean;
  onRetry: () => void;
};

function rankDeltaLabel(delta: number | null): string {
  if (delta === null) return "No rank history";
  if (delta === 0) return "No change";
  return `${delta > 0 ? "+" : ""}${delta}`;
}

function topShare(standing: LeaderboardStanding): string {
  if (!standing.rank || standing.participant_count <= 0) return "--";
  return `${((standing.rank / standing.participant_count) * 100).toFixed(1)}%`;
}

function gapLabel(value: number): string {
  return `${Math.max(value, 0).toFixed(2)}%`;
}

export function YourStandingCard({
  standing,
  isLoading,
  isError,
  onRetry,
}: YourStandingCardProps) {
  if (isLoading) {
    return (
      <section className="rounded-2xl border border-sky-300/15 bg-zinc-900/35 p-5">
        <Skeleton className="h-4 w-32" />
        <Skeleton className="mt-5 h-24 rounded-xl" />
        <div className="mt-3 grid grid-cols-2 gap-3">
          <Skeleton className="h-16 rounded-xl" />
          <Skeleton className="h-16 rounded-xl" />
        </div>
      </section>
    );
  }

  if (isError) {
    return (
      <section className="rounded-2xl border border-rose-400/15 bg-rose-400/[0.05] p-5">
        <div className="flex items-center justify-between gap-3">
          <p className="text-sm font-medium text-rose-100">Your standing is unavailable.</p>
          <button
            type="button"
            onClick={onRetry}
            className="text-xs font-medium text-zinc-300 underline underline-offset-4"
          >
            Retry
          </button>
        </div>
      </section>
    );
  }

  if (!standing || !standing.eligible) {
    return (
      <section className="rounded-2xl border border-sky-300/15 bg-[radial-gradient(circle_at_top_left,rgba(56,189,248,0.07),transparent_45%),rgba(24,24,27,0.42)] p-5">
        <div className="flex items-center gap-2">
          <Medal className="h-4 w-4 text-sky-300" />
          <h2 className="text-sm font-semibold text-zinc-100">Your Standing</h2>
        </div>
        <p className="mt-3 text-sm text-zinc-400">
          {standing?.reason || "Not enough ranked history for this period yet."}
        </p>
      </section>
    );
  }

  const delta = standing.rank_delta;
  const deltaTone =
    delta === null || delta === 0
      ? "text-zinc-400"
      : delta > 0
        ? "text-emerald-300"
        : "text-rose-300";
  const DeltaIcon = delta !== null && delta < 0 ? ArrowDownRight : ArrowUpRight;

  return (
    <section className="rounded-2xl border border-sky-300/15 bg-[radial-gradient(circle_at_top_left,rgba(56,189,248,0.075),transparent_42%),radial-gradient(circle_at_bottom_right,rgba(129,140,248,0.04),transparent_35%),rgba(24,24,27,0.46)] p-5 shadow-lg shadow-black/15">
      <div className="flex flex-col justify-between gap-3 sm:flex-row sm:items-start">
        <div>
          <div className="flex items-center gap-2">
            <Medal className="h-4 w-4 text-amber-300" />
            <h2 className="text-sm font-semibold text-zinc-100">Your Standing</h2>
          </div>
          <p className="mt-2 text-xs text-zinc-500">
            {standing.timeframe} rank among {standing.participant_count} strategies
          </p>
        </div>
        <div className="rounded-full border border-sky-300/15 bg-sky-300/[0.05] px-2.5 py-1 font-mono text-[11px] tabular-nums text-sky-100/70">
          Percentile {standing.percentile.toFixed(1)}
        </div>
      </div>

      <div className="mt-5 rounded-xl border border-sky-300/10 bg-zinc-950/45 px-4 py-4">
        <p className="text-[10px] uppercase tracking-[0.16em] text-zinc-600">Current rank</p>
        <div className="mt-2 flex items-end justify-between gap-4">
          <p className="font-mono text-4xl font-medium tabular-nums text-amber-50">
            #{standing.rank}
          </p>
          <div className="pb-1 text-right text-xs text-zinc-500">
            <p>
              Index <span className="font-mono text-zinc-300">{standing.ranked_index.toFixed(2)}</span>
            </p>
            <p className="mt-1">
              Return{" "}
              <span className={cn("font-mono", standing.ranked_return_percentage >= 0 ? "text-emerald-300" : "text-rose-300")}>
                {formatPercent(standing.ranked_return_percentage)}
              </span>
            </p>
          </div>
        </div>
      </div>

      <div className="mt-3 grid grid-cols-2 gap-3">
        <div className="rounded-xl border border-zinc-800/80 bg-zinc-950/35 px-4 py-3">
          <p className="text-[10px] uppercase tracking-[0.14em] text-zinc-600">Movement</p>
          <p className={cn("mt-2 flex items-center gap-1 font-mono text-lg tabular-nums", deltaTone)}>
            {delta !== null && delta !== 0 && <DeltaIcon className="h-4 w-4" />}
            {rankDeltaLabel(delta)}
          </p>
        </div>
        <div className="rounded-xl border border-zinc-800/80 bg-zinc-950/35 px-4 py-3">
          <p className="text-[10px] uppercase tracking-[0.14em] text-zinc-600">Position</p>
          <p className="mt-2 font-mono text-lg tabular-nums text-zinc-100">
            Top {topShare(standing)}
          </p>
          {standing.best_rank && (
            <p className="mt-1 text-xs text-zinc-500">Best #{standing.best_rank}</p>
          )}
        </div>
      </div>

      <div className="mt-3 flex items-start gap-2 rounded-xl border border-zinc-800/80 bg-zinc-950/30 px-3 py-3 text-xs leading-5 text-zinc-400">
        <Target className="mt-0.5 h-3.5 w-3.5 shrink-0 text-violet-300/65" />
        <span>
          {standing.next_milestone
            ? `${gapLabel(standing.next_milestone.return_gap_percentage)} to ${standing.next_milestone.label}`
            : "Top rank reached"}
        </span>
      </div>
    </section>
  );
}
