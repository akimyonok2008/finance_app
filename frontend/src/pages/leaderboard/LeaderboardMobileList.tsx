import { motion } from "framer-motion";

import {
  formatSignedPercent,
  getAvatarSymbol,
  getPercentClassName,
} from "@/pages/leaderboard/leaderboardUtils";
import { StrategyTagBadge } from "@/pages/leaderboard/StrategyTagBadge";
import type { LeaderboardEntry } from "@/types/leaderboard";
import { cn } from "@/utils/cn";

export function LeaderboardMobileList({
  entries,
}: {
  entries: LeaderboardEntry[];
}) {
  return (
    <div className="space-y-3 lg:hidden">
      {entries.map((entry, index) => (
        <motion.article
          key={`${entry.rank}-${entry.display_name}`}
          initial={{ opacity: 0, y: 10 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.18, delay: Math.min(index * 0.03, 0.25) }}
          className={cn(
            "rounded-2xl border border-zinc-800 bg-zinc-900/50 p-4 shadow-sm shadow-black/20",
            entry.is_me && "border-violet-400/20 bg-violet-500/[0.035] ring-1 ring-violet-400/20",
          )}
        >
          <div className="flex items-start gap-3">
            <div className="grid h-11 w-11 shrink-0 place-items-center rounded-xl border border-white/10 bg-zinc-900 text-xl">
              {getAvatarSymbol(entry.avatar_key)}
            </div>
            <div className="min-w-0 flex-1">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <div className="truncate text-sm font-semibold text-zinc-100">
                    {entry.display_name}
                  </div>
                  <div className="mt-1">
                    <StrategyTagBadge strategy={entry.strategy_tag} />
                  </div>
                </div>
                <div className="font-mono text-sm font-bold tabular-nums text-zinc-400">
                  #{entry.rank}
                </div>
              </div>
            </div>
          </div>
          <div className="mt-4 grid grid-cols-2 gap-2">
            <div className="rounded-xl border border-white/[0.07] bg-white/[0.025] p-3">
              <div className="text-[10px] uppercase tracking-widest text-zinc-500">
                24h change
              </div>
              <div
                className={cn(
                  "mt-1 w-fit rounded-full border px-2 py-0.5 font-mono text-xs tabular-nums",
                  getPercentClassName(entry.change_24h_percentage),
                )}
              >
                {formatSignedPercent(entry.change_24h_percentage)}
              </div>
            </div>
            <div className="rounded-xl border border-white/[0.07] bg-white/[0.025] p-3">
              <div className="text-[10px] uppercase tracking-widest text-zinc-500">
                Total ROI
              </div>
              <div
                className={cn(
                  "mt-1 font-mono text-sm font-semibold tabular-nums",
                  entry.total_roi_percentage >= 0
                    ? "text-emerald-400"
                    : "text-rose-400",
                )}
              >
                {formatSignedPercent(entry.total_roi_percentage)}
              </div>
            </div>
          </div>
        </motion.article>
      ))}
    </div>
  );
}
