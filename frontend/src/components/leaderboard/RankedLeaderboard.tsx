import { ArrowUpRight, Medal } from "lucide-react";
import { Link } from "react-router-dom";

import type { LeaderboardEntry } from "@/types/leaderboard";
import { cn } from "@/utils/cn";
import { formatPercent } from "@/utils/formatPercent";
import { compareDecimal, decimalToChartNumber } from "@/utils/decimal";

function rankTone(rank: number): string {
  if (rank === 1) return "border-amber-300/30 bg-amber-300/10 text-amber-200";
  if (rank === 2) return "border-sky-300/20 bg-sky-300/[0.07] text-sky-200";
  if (rank === 3) return "border-rose-300/20 bg-rose-300/[0.07] text-rose-200";
  return "border-zinc-800 bg-zinc-950/60 text-zinc-500";
}

export function RankedLeaderboard({
  entries,
  emptyTitle = "No ranked strategies yet",
  emptyDescription = "Create a public strategy baseline to enter the board.",
}: {
  entries: LeaderboardEntry[];
  emptyTitle?: string;
  emptyDescription?: string;
}) {
  if (entries.length === 0) {
    return (
      <div className="rounded-2xl border border-dashed border-zinc-800 bg-zinc-900/25 px-5 py-14 text-center">
        <Medal className="mx-auto h-6 w-6 text-zinc-600" />
        <h3 className="mt-4 font-medium text-zinc-200">{emptyTitle}</h3>
        <p className="mt-1 text-sm text-zinc-500">{emptyDescription}</p>
      </div>
    );
  }

  return (
    <div className="overflow-hidden rounded-xl border border-zinc-800/90 bg-zinc-950/35">
      <div className="hidden grid-cols-[5rem_1fr_8rem_8rem_8rem] border-b border-zinc-800 px-5 py-3 text-[11px] uppercase tracking-[0.14em] text-zinc-600 lg:grid">
        <span>Rank</span><span>Strategy</span><span className="text-right">Index</span><span className="text-right">Return</span><span className="text-right">Actions</span>
      </div>
      {entries.map((entry) => {
        const name = (
          <span className="text-[15px] font-semibold tracking-tight text-zinc-100">
            {entry.display_name}
            {entry.is_me && <span className="ml-2 rounded-full bg-indigo-300/10 px-1.5 py-0.5 text-[9px] uppercase tracking-widest text-indigo-200">You</span>}
          </span>
        );
        return (
          <article key={`${entry.rank}-${entry.display_name}`} className={cn("grid gap-3 border-b border-zinc-900/90 px-4 py-4 transition-colors last:border-b-0 hover:bg-white/[0.025] lg:grid-cols-[5rem_1fr_8rem_8rem_8rem] lg:items-center lg:px-5", entry.rank === 1 && "bg-amber-300/[0.025]", entry.is_me && "bg-indigo-300/[0.045] shadow-[inset_3px_0_0_rgba(165,180,252,0.55)]")}>
            <span className={cn("w-fit rounded-lg border px-2 py-1 font-mono text-xs font-semibold tabular-nums", rankTone(entry.rank))}>#{entry.rank}</span>
            <div className="min-w-0">
              {entry.handle ? <Link to={`/profiles/${entry.handle}`} className="hover:underline">{name}</Link> : name}
              {entry.strategy_tag && (
                <div className="mt-1 text-xs font-medium text-violet-200/55">
                  {entry.strategy_tag.replaceAll("_", " ")}
                </div>
              )}
            </div>
            <div className="flex items-center justify-between sm:block sm:text-right">
              <span className="text-xs text-zinc-600 sm:hidden">Index</span>
              <span className="font-mono text-sm font-medium tabular-nums text-sky-100/75">
                {decimalToChartNumber(entry.ranked_index)?.toFixed(2) ?? "—"}
              </span>
            </div>
            <div className="flex items-center justify-between sm:block sm:text-right">
              <span className="text-xs text-zinc-600 sm:hidden">Return</span>
              <span className={cn("font-mono text-sm font-semibold tabular-nums", compareDecimal(entry.ranked_return_percentage, "0") >= 0 ? "text-emerald-400" : "text-rose-400")}>
                {formatPercent(entry.ranked_return_percentage)}
              </span>
            </div>
            <div className="flex justify-start lg:justify-end">
              {entry.handle ? (
                <Link
                  to={`/profiles/${entry.handle}`}
                  className="inline-flex items-center gap-1.5 rounded-lg border border-zinc-800 bg-zinc-950/45 px-3 py-2 text-xs font-medium text-zinc-400 transition hover:border-indigo-300/20 hover:bg-indigo-300/[0.05] hover:text-indigo-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-indigo-300/30"
                >
                  View profile
                  <ArrowUpRight className="h-3.5 w-3.5" />
                </Link>
              ) : (
                <span className="text-xs text-zinc-700">Private</span>
              )}
            </div>
          </article>
        );
      })}
    </div>
  );
}
