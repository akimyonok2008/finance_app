import { motion } from "framer-motion";
import { Users } from "lucide-react";

import {
  formatSignedPercent,
  getPercentClassName,
} from "@/pages/arena/arenaUtils";
import type { CohortLeaderboardEntry } from "@/types/arena";
import { cn } from "@/utils/cn";

export function CohortLeaderboard({
  entries,
  isError,
}: {
  entries: CohortLeaderboardEntry[] | undefined;
  isLoading?: boolean;
  isError?: boolean;
}) {
  return (
    <section
      aria-labelledby="cohort-heading"
      className="overflow-hidden rounded-2xl border border-zinc-800 bg-zinc-900/50 shadow-sm shadow-black/20"
    >
      <div className="flex items-center justify-between border-b border-zinc-800 px-5 py-4">
        <div className="flex items-center gap-2">
          <div className="grid h-9 w-9 place-items-center rounded-lg border border-zinc-800 bg-zinc-950/50 text-zinc-400">
            <Users className="h-4 w-4" />
          </div>
          <div>
            <h2 id="cohort-heading" className="text-sm font-semibold text-zinc-100">
              Cohort leaderboard
            </h2>
            <p className="text-xs text-zinc-500">Percentage only</p>
          </div>
        </div>
        <span className="font-mono text-xs tabular-nums text-zinc-500">
          {entries?.length ?? 0} ranked
        </span>
      </div>

      {isError ? (
        <div className="px-5 py-12 text-center text-sm text-rose-300">
          Cohort rankings are temporarily unavailable.
        </div>
      ) : !entries?.length ? (
        <div className="px-5 py-12 text-center">
          <p className="text-sm font-medium text-zinc-300">
            No cohort rankings yet
          </p>
          <p className="mt-1 text-xs text-zinc-500">
            Join the sprint to appear here.
          </p>
        </div>
      ) : (
        <div>
          <div className="grid grid-cols-[4rem_1fr_auto] border-b border-zinc-800 px-4 py-3 text-xs uppercase tracking-[0.18em] text-zinc-500">
            <span>Rank</span>
            <span>User</span>
            <span>ROI</span>
          </div>
          {entries.map((entry, index) => (
            <motion.div
              key={`${entry.rank}-${entry.username}`}
              initial={{ opacity: 0, y: 8 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ duration: 0.18, delay: Math.min(index * 0.03, 0.25) }}
              className={cn(
                "grid grid-cols-[4rem_1fr_auto] items-center border-b border-zinc-800/70 px-4 py-3 transition last:border-0 hover:bg-white/[0.03]",
                entry.isCurrentUser &&
                  "bg-violet-500/[0.035] ring-1 ring-inset ring-violet-500/20",
              )}
            >
              <span className="font-mono text-sm font-semibold tabular-nums text-zinc-400">
                #{entry.rank}
              </span>
              <span className="truncate text-sm font-medium text-zinc-200">
                {entry.username}
                {entry.isCurrentUser && (
                  <span className="ml-2 text-[10px] uppercase tracking-widest text-violet-300">
                    You
                  </span>
                )}
              </span>
              <span
                className={cn(
                  "font-mono text-sm font-semibold tabular-nums",
                  getPercentClassName(entry.roi),
                )}
              >
                {formatSignedPercent(entry.roi)}
              </span>
            </motion.div>
          ))}
        </div>
      )}
    </section>
  );
}
