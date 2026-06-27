import { Medal } from "lucide-react";
import { Link } from "react-router-dom";

import type { LeaderboardEntry } from "@/types/leaderboard";
import { cn } from "@/utils/cn";
import { formatPercent } from "@/utils/formatPercent";

function rankTone(rank: number): string {
  if (rank === 1) return "border-amber-300/30 bg-amber-300/10 text-amber-200";
  if (rank === 2) return "border-zinc-300/20 bg-zinc-300/10 text-zinc-200";
  if (rank === 3) return "border-orange-400/25 bg-orange-400/10 text-orange-200";
  return "border-zinc-800 bg-zinc-950/60 text-zinc-500";
}

export function RankedLeaderboard({ entries }: { entries: LeaderboardEntry[] }) {
  if (entries.length === 0) {
    return (
      <div className="rounded-2xl border border-dashed border-zinc-800 bg-zinc-900/25 px-5 py-14 text-center">
        <Medal className="mx-auto h-6 w-6 text-zinc-600" />
        <h3 className="mt-4 font-medium text-zinc-200">No ranked strategies yet</h3>
        <p className="mt-1 text-sm text-zinc-500">Create a public strategy baseline to enter the board.</p>
      </div>
    );
  }

  return (
    <div className="overflow-hidden rounded-2xl border border-zinc-800 bg-zinc-900/35">
      <div className="hidden grid-cols-[5rem_1fr_8rem_8rem] border-b border-zinc-800 px-5 py-3 text-[11px] uppercase tracking-[0.14em] text-zinc-600 sm:grid">
        <span>Rank</span><span>Strategy</span><span className="text-right">Index</span><span className="text-right">Return</span>
      </div>
      {entries.map((entry) => {
        const name = (
          <span className="font-medium text-zinc-100">
            {entry.display_name}
            {entry.is_me && <span className="ml-2 text-[10px] uppercase tracking-widest text-violet-300">You</span>}
          </span>
        );
        return (
          <article key={`${entry.rank}-${entry.display_name}`} className={cn("grid gap-3 border-b border-zinc-900/90 px-4 py-4 last:border-b-0 sm:grid-cols-[5rem_1fr_8rem_8rem] sm:items-center sm:px-5", entry.is_me && "bg-violet-400/[0.04]")}>
            <span className={cn("w-fit rounded-lg border px-2 py-1 font-mono text-xs font-semibold tabular-nums", rankTone(entry.rank))}>#{entry.rank}</span>
            <div className="min-w-0">
              {entry.handle ? <Link to={`/profiles/${entry.handle}`} className="hover:underline">{name}</Link> : name}
              <div className="mt-1 flex flex-wrap gap-1.5">
                {entry.strategy_tag && <span className="text-xs text-zinc-500">{entry.strategy_tag.replaceAll("_", " ")}</span>}
                {entry.public_weights.slice(0, 3).map((weight) => (
                  <span key={weight.symbol} className="rounded-full bg-zinc-800/70 px-2 py-0.5 font-mono text-[10px] text-zinc-400">
                    {weight.symbol} {weight.weight_percentage.toFixed(0)}%
                  </span>
                ))}
              </div>
            </div>
            <div className="flex items-center justify-between sm:block sm:text-right">
              <span className="text-xs text-zinc-600 sm:hidden">Index</span>
              <span className="font-mono text-sm tabular-nums text-zinc-300">{entry.ranked_index.toFixed(2)}</span>
            </div>
            <div className="flex items-center justify-between sm:block sm:text-right">
              <span className="text-xs text-zinc-600 sm:hidden">Return</span>
              <span className={cn("font-mono text-sm font-semibold tabular-nums", entry.ranked_return_percentage >= 0 ? "text-emerald-400" : "text-rose-400")}>
                {formatPercent(entry.ranked_return_percentage)}
              </span>
            </div>
          </article>
        );
      })}
    </div>
  );
}
