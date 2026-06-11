import { AnimatePresence, motion } from "framer-motion";

import {
  formatSignedPercent,
  isEntryVisible,
} from "@/pages/leaderboard/leaderboardUtils";
import { StrategyTagBadge } from "@/pages/leaderboard/StrategyTagBadge";
import type { LeaderboardEntry } from "@/types/leaderboard";
import { cn } from "@/utils/cn";

export function UserStickyRow({
  me,
  visibleEntries,
}: {
  me: LeaderboardEntry | null | undefined;
  visibleEntries: LeaderboardEntry[];
}) {
  const visible = me ? isEntryVisible(visibleEntries, me) : false;

  return (
    <AnimatePresence>
      {me && !visible && (
        <motion.aside
          aria-label="Your leaderboard position"
          initial={{ opacity: 0, y: 24 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, y: 24 }}
          transition={{ type: "spring", stiffness: 260, damping: 24 }}
          className="fixed inset-x-0 bottom-0 z-40 border-t border-zinc-800 bg-zinc-950/90 px-4 py-3 backdrop-blur"
        >
          <div className="mx-auto flex max-w-7xl flex-wrap items-center justify-between gap-3 rounded-xl border border-zinc-800 bg-zinc-900/80 px-4 py-3">
            <div>
              <div className="text-[10px] uppercase tracking-widest text-zinc-500">
                Your rank
              </div>
              <div className="font-mono text-xl font-bold tabular-nums text-zinc-50">
                #{me.rank}
              </div>
            </div>
            <div className="hidden sm:block">
              <div className="text-[10px] uppercase tracking-widest text-zinc-500">
                Strategy
              </div>
              <div className="mt-1">
                <StrategyTagBadge strategy={me.strategy_tag} />
              </div>
            </div>
            <div>
              <div className="text-[10px] uppercase tracking-widest text-zinc-500">
                24h change
              </div>
              <div className="font-mono text-sm font-semibold tabular-nums text-zinc-300">
                {formatSignedPercent(me.change_24h_percentage)}
              </div>
            </div>
            <div>
              <div className="text-[10px] uppercase tracking-widest text-zinc-500">
                Total ROI
              </div>
              <div
                className={cn(
                  "font-mono text-sm font-semibold tabular-nums",
                  me.total_roi_percentage >= 0
                    ? "text-emerald-400"
                    : "text-rose-400",
                )}
              >
                {formatSignedPercent(me.total_roi_percentage)}
              </div>
            </div>
          </div>
        </motion.aside>
      )}
    </AnimatePresence>
  );
}
