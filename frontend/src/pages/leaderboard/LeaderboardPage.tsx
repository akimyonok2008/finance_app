import { AnimatePresence, motion } from "framer-motion";
import { ArrowLeft, LockKeyhole, RefreshCw } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";

import { useAuth } from "@/auth/useAuth";
import { useGlobalLeaderboard } from "@/hooks/useGlobalLeaderboard";
import { LeaderboardEmptyState } from "@/pages/leaderboard/LeaderboardEmptyState";
import { LeaderboardMobileList } from "@/pages/leaderboard/LeaderboardMobileList";
import { LeaderboardPodium } from "@/pages/leaderboard/LeaderboardPodium";
import { LeaderboardSkeleton } from "@/pages/leaderboard/LeaderboardSkeleton";
import { LeaderboardTable } from "@/pages/leaderboard/LeaderboardTable";
import { TimeframeTabs } from "@/pages/leaderboard/TimeframeTabs";
import { UserStickyRow } from "@/pages/leaderboard/UserStickyRow";
import type { LeaderboardTimeframe } from "@/types/leaderboard";

const PAGE_SIZE = 50;

export function LeaderboardPage() {
  const { user } = useAuth();
  const [timeframe, setTimeframe] =
    useState<LeaderboardTimeframe>("daily");
  const [page, setPage] = useState(1);
  const query = useGlobalLeaderboard({
    timeframe,
    page,
    page_size: PAGE_SIZE,
  });

  const source = query.data?.entries ?? [];
  const entries = source.map((entry) => ({
    ...entry,
    is_me:
      entry.is_me ||
      Boolean(
        user?.display_name &&
          entry.display_name.toLowerCase() === user.display_name.toLowerCase(),
      ),
  }));
  const visibleMe = entries.find((entry) => entry.is_me);
  const responseMe = query.data?.me
    ? { ...query.data.me, is_me: true }
    : null;
  const me = responseMe ?? visibleMe ?? null;
  const total = query.data?.total ?? 0;

  const podium = page === 1 ? entries.slice(0, 3) : [];
  const remaining = page === 1 ? entries.slice(3) : entries;
  const hasNextPage = page * PAGE_SIZE < total;

  const handleTimeframeChange = (next: LeaderboardTimeframe) => {
    setTimeframe(next);
    setPage(1);
  };

  return (
    <div className="min-h-screen bg-zinc-950 text-zinc-50">
      <main className="mx-auto w-full max-w-7xl px-4 pb-28 pt-6 sm:px-6 lg:px-8">
        <div className="mb-8 flex items-center justify-between gap-3">
          <Link
            to="/dashboard"
            className="inline-flex items-center gap-2 rounded-lg border border-zinc-800 bg-zinc-900/40 px-3 py-2 text-xs font-medium text-zinc-400 transition hover:border-zinc-700 hover:bg-zinc-900/70 hover:text-zinc-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-500"
          >
            <ArrowLeft className="h-3.5 w-3.5" />
            Back to dashboard
          </Link>
          <button
            type="button"
            onClick={() => query.refetch()}
            disabled={query.isFetching}
            className="inline-flex items-center gap-2 rounded-lg border border-zinc-800 bg-zinc-900/40 px-3 py-2 text-xs font-medium text-zinc-400 transition hover:border-zinc-700 hover:bg-zinc-900/70 hover:text-zinc-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-500 disabled:opacity-50"
          >
            <RefreshCw
              className={`h-3.5 w-3.5 ${query.isFetching ? "animate-spin" : ""}`}
            />
            Refresh
          </button>
        </div>

        <motion.header
          initial={{ opacity: 0, y: -10 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.35 }}
          className="mb-8 flex flex-col justify-between gap-6 lg:flex-row lg:items-end"
        >
          <div>
            <h1 className="text-3xl font-medium tracking-tight">
              Global Leaderboard
            </h1>
            <p className="mt-2 text-sm text-zinc-400">Anonymous performance rankings.</p>
          </div>
          <TimeframeTabs
            value={timeframe}
            onChange={handleTimeframeChange}
          />
        </motion.header>

        <div className="mb-6 flex items-center gap-3 rounded-xl border border-zinc-800 bg-zinc-900/40 px-4 py-3 text-xs text-zinc-500">
          <LockKeyhole className="h-4 w-4 shrink-0" />
          Percentage performance only. Holdings and wealth stay private.
        </div>

        {query.isLoading ? (
          <LeaderboardSkeleton />
        ) : query.isError ? (
          <div className="rounded-2xl border border-rose-400/15 bg-rose-400/[0.05] px-6 py-14 text-center">
            <h2 className="text-lg font-semibold text-zinc-100">
              Leaderboard is temporarily unavailable.
            </h2>
            <p className="mt-2 text-sm text-zinc-400">
              Please try again in a moment.
            </p>
            <button
              type="button"
              onClick={() => query.refetch()}
              className="mt-6 rounded-xl border border-white/10 bg-white/[0.04] px-4 py-2 text-sm font-medium text-zinc-200 transition hover:bg-white/[0.08] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-emerald-400/50"
            >
              Retry
            </button>
          </div>
        ) : entries.length === 0 ? (
          <LeaderboardEmptyState />
        ) : (
          <AnimatePresence mode="wait">
            <motion.div
              key={`${timeframe}-${page}`}
              initial={{ opacity: 0, y: 8 }}
              animate={{ opacity: query.isFetching ? 0.65 : 1, y: 0 }}
              exit={{ opacity: 0, y: -8 }}
              transition={{ duration: 0.18 }}
              className="space-y-6"
            >
              {podium.length > 0 && <LeaderboardPodium entries={podium} />}

              {remaining.length > 0 && (
                <>
                  <LeaderboardTable entries={remaining} />
                  <LeaderboardMobileList entries={remaining} />
                </>
              )}

              <div className="flex items-center justify-between rounded-2xl border border-zinc-800 bg-zinc-950/60 px-4 py-3">
                <button
                  type="button"
                  disabled={page === 1}
                  onClick={() => setPage((current) => Math.max(1, current - 1))}
                  className="rounded-xl border border-white/10 px-4 py-2 text-xs font-medium text-zinc-300 transition hover:bg-white/[0.05] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-emerald-400/50 disabled:cursor-not-allowed disabled:opacity-35"
                >
                  Previous
                </button>
                <span className="font-mono text-xs tabular-nums text-zinc-500">
                  Page {page}
                </span>
                <button
                  type="button"
                  disabled={!hasNextPage}
                  onClick={() => setPage((current) => current + 1)}
                  className="rounded-xl border border-white/10 px-4 py-2 text-xs font-medium text-zinc-300 transition hover:bg-white/[0.05] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-emerald-400/50 disabled:cursor-not-allowed disabled:opacity-35"
                >
                  Next
                </button>
              </div>

              {/* TODO: Add virtualized infinite scrolling when backend supports cursor pagination. */}
            </motion.div>
          </AnimatePresence>
        )}
      </main>

      <UserStickyRow me={me} visibleEntries={entries} />
    </div>
  );
}
