import { motion } from "framer-motion";
import { Crown, Medal, TrendingDown, TrendingUp, Trophy } from "lucide-react";

import {
  formatSignedPercent,
  getAvatarSymbol,
  getPercentClassName,
  getPercentTone,
} from "@/pages/leaderboard/leaderboardUtils";
import { StrategyTagBadge } from "@/pages/leaderboard/StrategyTagBadge";
import type { LeaderboardEntry } from "@/types/leaderboard";
import { cn } from "@/utils/cn";

const rankStyles = {
  1: {
    card: "border-amber-500/20",
    icon: Crown,
    iconClass: "text-amber-300",
  },
  2: {
    card: "border-zinc-500/30",
    icon: Trophy,
    iconClass: "text-slate-300",
  },
  3: {
    card: "border-amber-700/20",
    icon: Medal,
    iconClass: "text-amber-600",
  },
} as const;

export function LeaderboardPodium({
  entries,
}: {
  entries: LeaderboardEntry[];
  isLoading?: boolean;
}) {
  return (
    <section aria-labelledby="podium-heading">
      <h2 id="podium-heading" className="sr-only">
        Top three investors
      </h2>
      <div className="grid gap-4 md:grid-cols-3">
        {entries.map((entry, index) => {
          const style =
            rankStyles[entry.rank as keyof typeof rankStyles] ?? rankStyles[3];
          const RankIcon = style.icon;
          const changeTone = getPercentTone(entry.change_24h_percentage);
          const ChangeIcon =
            changeTone === "positive"
              ? TrendingUp
              : changeTone === "negative"
                ? TrendingDown
                : null;

          return (
            <motion.article
              key={`${entry.rank}-${entry.display_name}`}
              initial={{ opacity: 0, y: 20, scale: 0.96 }}
              animate={{ opacity: 1, y: 0, scale: 1 }}
              transition={{
                type: "spring",
                stiffness: 260,
                damping: 24,
                delay: index * 0.06,
              }}
              whileHover={{ y: -4, scale: 1.015 }}
              className={cn(
                "relative overflow-hidden rounded-2xl border bg-zinc-900/50 p-4 shadow-sm shadow-black/20",
                style.card,
                entry.is_me && "ring-1 ring-violet-400/40",
              )}
            >
              <div className="absolute right-4 top-4 flex items-center gap-1 text-xs font-bold text-zinc-400">
                <RankIcon className={cn("h-4 w-4", style.iconClass)} />
                #{entry.rank}
              </div>
              <div className="mb-4 w-fit">
                <div className="grid h-14 w-14 place-items-center rounded-xl border border-zinc-800 bg-zinc-950/60 text-2xl">
                  {getAvatarSymbol(entry.avatar_key)}
                </div>
              </div>
              <h3 className="truncate text-lg font-semibold tracking-tight text-zinc-50">
                {entry.display_name}
              </h3>
              <div className="mt-2">
                <StrategyTagBadge strategy={entry.strategy_tag} />
              </div>
              <div className="mt-5 grid grid-cols-2 gap-2">
                <div className="rounded-xl border border-zinc-800 bg-zinc-950/40 p-3">
                  <div className="text-[10px] uppercase tracking-widest text-zinc-500">
                    24h change
                  </div>
                  <div
                    className={cn(
                      "mt-1 flex items-center gap-1 text-sm font-semibold tabular-nums",
                      getPercentClassName(entry.change_24h_percentage)
                        .split(" ")
                        .find((className) => className.startsWith("text-")),
                    )}
                  >
                    {ChangeIcon && <ChangeIcon className="h-3.5 w-3.5" />}
                    {formatSignedPercent(entry.change_24h_percentage)}
                  </div>
                </div>
                <div className="rounded-xl border border-zinc-800 bg-zinc-950/40 p-3">
                  <div className="text-[10px] uppercase tracking-widest text-zinc-500">
                    Total ROI
                  </div>
                  <div
                    className={cn(
                      "mt-1 text-sm font-semibold tabular-nums",
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
          );
        })}
      </div>
    </section>
  );
}
