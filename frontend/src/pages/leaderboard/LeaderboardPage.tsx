import { RefreshCw, ShieldCheck, Trophy } from "lucide-react";
import { useState } from "react";

import { useAuth } from "@/auth/useAuth";
import { RankedLeaderboard } from "@/components/leaderboard/RankedLeaderboard";
import { YourStandingCard } from "@/components/leaderboard/YourStandingCard";
import { AppNav } from "@/components/layout/AppNav";
import { useGlobalLeaderboard } from "@/hooks/useGlobalLeaderboard";
import { useLeaderboardStanding } from "@/hooks/useLeaderboardStanding";
import { LeaderboardSkeleton } from "@/pages/leaderboard/LeaderboardSkeleton";
import { TimeframeTabs } from "@/pages/leaderboard/TimeframeTabs";
import type { LeaderboardTimeframe } from "@/types/leaderboard";

export function LeaderboardPage() {
  const { user } = useAuth();
  const [timeframe, setTimeframe] = useState<LeaderboardTimeframe>("ALL");
  const query = useGlobalLeaderboard({ timeframe });
  const standingQuery = useLeaderboardStanding(timeframe);
  const isWindowed = timeframe !== "ALL";
  const entries = (query.data?.entries ?? []).map((entry) => ({
    ...entry,
    is_me:
      entry.is_me ||
      Boolean(
        user?.display_name &&
          entry.display_name.toLowerCase() === user.display_name.toLowerCase(),
      ),
  }));

  return (
    <div className="leaderboard-shell min-h-screen bg-[radial-gradient(circle_at_12%_18%,rgba(245,158,11,0.055),transparent_24%),radial-gradient(circle_at_88%_32%,rgba(99,102,241,0.045),transparent_26%),#09090b] text-zinc-50">
      <main className="mx-auto w-full max-w-6xl px-4 pb-20 pt-4 sm:px-6 lg:px-8">
        <AppNav
          actions={
            <button
              type="button"
              onClick={() => {
                void query.refetch();
                void standingQuery.refetch();
              }}
              disabled={query.isFetching || standingQuery.isFetching}
              aria-label="Refresh leaderboard"
              className="rounded-lg p-2 text-zinc-400 transition hover:bg-zinc-800/70 hover:text-zinc-100 disabled:opacity-50"
            >
              <RefreshCw className={`h-3.5 w-3.5 ${query.isFetching || standingQuery.isFetching ? "animate-spin" : ""}`} />
            </button>
          }
        />

        <header className="mb-6 flex flex-col justify-between gap-5 lg:flex-row lg:items-end">
          <div>
            <div className="mb-2 flex items-center gap-2 text-[11px] font-semibold uppercase tracking-[0.2em] text-amber-300">
              <ShieldCheck className="h-3.5 w-3.5" /> Fair, baseline-ranked performance
            </div>
            <h1 className="leaderboard-display text-4xl font-semibold tracking-tight">Leaderboard</h1>
            <p className="mt-2 max-w-2xl text-sm text-zinc-400">
              Compare strategy performance from a locked index of 100. No wealth rankings.
            </p>
          </div>
          <TimeframeTabs value={timeframe} onChange={setTimeframe} />
        </header>

        <div className="grid items-start gap-6 lg:grid-cols-[minmax(0,1fr)_19rem]">
          <section
            aria-labelledby="rankings-title"
            className="min-w-0 overflow-hidden rounded-2xl border border-amber-300/15 bg-[radial-gradient(circle_at_top_left,rgba(251,191,36,0.09),transparent_28%),radial-gradient(circle_at_bottom_right,rgba(129,140,248,0.055),transparent_34%),rgba(24,24,27,0.42)] shadow-xl shadow-black/20"
          >
            <div className="flex items-center justify-between gap-3 border-b border-zinc-800/90 px-5 py-4 sm:px-6">
              <div>
                <div className="flex items-center gap-2">
                  <Trophy className="h-4 w-4 text-amber-300" />
                  <h2 id="rankings-title" className="text-[15px] font-semibold tracking-tight text-zinc-100">
                    Ranked strategies
                  </h2>
                </div>
                <p className="mt-1 text-xs text-zinc-500">{timeframe} performance from each strategy baseline</p>
              </div>
              <span className="rounded-full border border-amber-300/15 bg-amber-300/[0.05] px-2.5 py-1 font-mono text-[11px] text-amber-100/70">
                {entries.length} ranked
              </span>
            </div>
            <div className="p-3 sm:p-4">
              {query.isLoading ? (
                <LeaderboardSkeleton />
              ) : query.isError ? (
                <div className="rounded-xl border border-rose-400/15 bg-rose-400/[0.05] px-5 py-12 text-center">
                  <p className="text-sm text-rose-200">Leaderboard is temporarily unavailable.</p>
                  <button type="button" onClick={() => query.refetch()} className="mt-4 text-xs font-medium text-zinc-300 underline underline-offset-4">Retry</button>
                </div>
              ) : (
                <RankedLeaderboard
                  entries={entries}
                  emptyTitle={
                    isWindowed ? "Not enough ranked history yet" : "No ranked strategies yet"
                  }
                  emptyDescription={
                    isWindowed
                      ? "This timeframe will fill in as leaderboard snapshots accrue."
                      : "Create a public strategy baseline to enter the board."
                  }
                />
              )}
            </div>
          </section>

          <aside className="space-y-6">
            <YourStandingCard
              standing={standingQuery.data}
              isLoading={standingQuery.isLoading}
              isError={standingQuery.isError}
              onRetry={() => {
                void standingQuery.refetch();
              }}
            />
          </aside>
        </div>
      </main>
    </div>
  );
}
