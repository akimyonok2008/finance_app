import { motion } from "framer-motion";
import { ArrowDownRight, ArrowRight, ArrowUpRight, Globe, Minus } from "lucide-react";
import { Link } from "react-router-dom";

import {
  formatParticipants,
  formatRank,
} from "@/pages/Dashboard/utils/dashboardFormatters";
import type { LeaderboardMe } from "@/types/dashboard";
import { cn } from "@/utils/cn";

type Props = {
  leaderboardMe: LeaderboardMe | null;
  isLoading: boolean;
  isError: boolean;
  onRetry?: () => void;
};

function RankDelta({ delta }: { delta: number | null | undefined }) {
  if (delta === null || delta === undefined) {
    return (
      <span className="flex items-center gap-1 text-xs text-slate-500">
        <Minus className="h-3 w-3" />
        No movement yet
      </span>
    );
  }
  // Negative delta = moved UP (lower rank number = better)
  if (delta < 0) {
    return (
      <span className="flex items-center gap-1 text-xs font-medium text-emerald-400">
        <ArrowUpRight className="h-3.5 w-3.5" />
        +{Math.abs(delta)} places
      </span>
    );
  }
  if (delta > 0) {
    return (
      <span className="flex items-center gap-1 text-xs font-medium text-rose-400">
        <ArrowDownRight className="h-3.5 w-3.5" />
        -{delta} places
      </span>
    );
  }
  return (
    <span className="flex items-center gap-1 text-xs text-slate-500">
      <Minus className="h-3 w-3" />
      No change
    </span>
  );
}

export function GlobalRankingWidget({
  leaderboardMe,
  isLoading,
  isError,
  onRetry,
}: Props) {
  const hasRank = leaderboardMe?.rank !== null && leaderboardMe?.rank !== undefined;

  return (
    <motion.div
      className={cn(
        "relative w-full overflow-hidden rounded-2xl border border-zinc-800 bg-zinc-900/50 p-5 text-left shadow-sm shadow-black/20 transition-colors hover:border-zinc-700 hover:bg-zinc-900/70 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-500",
      )}
      whileHover={{ y: -2 }}
      transition={{ duration: 0.18 }}
    >
      {/* Header */}
      <div className="mb-4 flex items-center gap-2">
        <div className="grid h-8 w-8 place-items-center rounded-lg border border-zinc-800 bg-zinc-950/50">
          <Globe className="h-4 w-4 text-zinc-400" />
        </div>
        <span className="text-xs font-medium uppercase tracking-widest text-slate-500">
          Global Ranking
        </span>
      </div>

      {isError ? (
        <div className="space-y-2">
          <p className="text-sm text-slate-400">
            Ranking is temporarily unavailable.
          </p>
          {onRetry && (
            <button
              onClick={(e) => {
                e.stopPropagation();
                onRetry();
              }}
              className="text-xs text-indigo-400 underline underline-offset-2"
            >
              Retry
            </button>
          )}
        </div>
      ) : !hasRank ? (
        <div className="space-y-1">
          <p className="font-mono text-2xl font-medium tracking-tight text-slate-500">—</p>
          <p className="text-sm text-slate-500">
            Ranking unavailable.
          </p>
        </div>
      ) : (
        <div className="space-y-3">
          <div>
            <div className="font-mono text-4xl font-medium tabular-nums tracking-tight text-slate-50">
              {formatRank(leaderboardMe.rank)}
            </div>
            <div className="mt-0.5 text-sm text-slate-400">
              {formatParticipants(leaderboardMe.total_participants)}
            </div>
          </div>
          <RankDelta delta={leaderboardMe.rank_delta} />
        </div>
      )}

      <Link
        to="/leaderboard"
        className="group mt-4 flex items-center gap-1 text-xs text-slate-500 transition hover:text-slate-300 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-emerald-400/50"
      >
        View full leaderboard
        <ArrowRight className="h-3 w-3 transition-transform group-hover:translate-x-0.5" />
      </Link>

      {isLoading && (
        <div className="absolute inset-0 rounded-2xl bg-zinc-950/60 backdrop-blur-sm" />
      )}
    </motion.div>
  );
}
