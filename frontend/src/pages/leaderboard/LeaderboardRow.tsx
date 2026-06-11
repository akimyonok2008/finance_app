import { motion } from "framer-motion";

import {
  formatSignedPercent,
  getAvatarSymbol,
  getPercentClassName,
} from "@/pages/leaderboard/leaderboardUtils";
import { StrategyTagBadge } from "@/pages/leaderboard/StrategyTagBadge";
import type { LeaderboardEntry } from "@/types/leaderboard";
import { cn } from "@/utils/cn";

export function LeaderboardRow({
  entry,
  index,
}: {
  entry: LeaderboardEntry;
  index: number;
}) {
  return (
    <motion.tr
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.18, delay: Math.min(index * 0.025, 0.25) }}
      className={cn(
        "border-b border-zinc-900/80 transition hover:bg-white/[0.03]",
        entry.is_me && "bg-violet-500/[0.035] ring-1 ring-inset ring-violet-400/20",
      )}
    >
      <td className="px-5 py-4 font-mono text-sm font-semibold tabular-nums text-zinc-400">
        #{entry.rank}
      </td>
      <td className="px-5 py-4">
        <div className="flex min-w-0 items-center gap-3">
          <div className="grid h-10 w-10 shrink-0 place-items-center rounded-xl border border-white/10 bg-zinc-900 text-xl">
            {getAvatarSymbol(entry.avatar_key)}
          </div>
          <div className="min-w-0">
            <div className="truncate text-sm font-medium text-zinc-100">
              {entry.display_name}
              {entry.is_me && (
                <span className="ml-2 text-[10px] uppercase tracking-widest text-violet-300">
                  You
                </span>
              )}
            </div>
          </div>
        </div>
      </td>
      <td className="px-5 py-4">
        <StrategyTagBadge strategy={entry.strategy_tag} />
      </td>
      <td className="px-5 py-4">
        <span
          className={cn(
            "inline-flex rounded-full border px-2.5 py-1 font-mono text-xs tabular-nums",
            getPercentClassName(entry.change_24h_percentage),
          )}
        >
          {formatSignedPercent(entry.change_24h_percentage)}
        </span>
      </td>
      <td
        className={cn(
          "px-5 py-4 text-right font-mono text-sm font-semibold tabular-nums",
          entry.total_roi_percentage >= 0
            ? "text-emerald-400"
            : "text-rose-400",
        )}
      >
        {formatSignedPercent(entry.total_roi_percentage)}
      </td>
    </motion.tr>
  );
}
