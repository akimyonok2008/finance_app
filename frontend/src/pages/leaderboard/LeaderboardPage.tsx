import { RefreshCw, ShieldCheck } from "lucide-react";
import { useState } from "react";

import { useAuth } from "@/auth/useAuth";
import { LeaderboardAchievements } from "@/components/leaderboard/LeaderboardAchievements";
import { RankedLeaderboard } from "@/components/leaderboard/RankedLeaderboard";
import { AppNav } from "@/components/layout/AppNav";
import { useGlobalLeaderboard } from "@/hooks/useGlobalLeaderboard";
import { LeaderboardSkeleton } from "@/pages/leaderboard/LeaderboardSkeleton";
import { TimeframeTabs } from "@/pages/leaderboard/TimeframeTabs";
import type { LeaderboardTimeframe } from "@/types/leaderboard";

export function LeaderboardPage() {
  const { user } = useAuth();
  const [timeframe, setTimeframe] = useState<LeaderboardTimeframe>("ALL");
  const query = useGlobalLeaderboard({ timeframe });
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
    <div className="min-h-screen bg-zinc-950 text-zinc-50">
      <main className="mx-auto w-full max-w-7xl px-4 pb-20 pt-4 sm:px-6 lg:px-8">
        <AppNav
          actions={
            <button
              type="button"
              onClick={() => query.refetch()}
              disabled={query.isFetching}
              aria-label="Refresh leaderboard"
              className="rounded-lg p-2 text-zinc-400 transition hover:bg-zinc-800/70 hover:text-zinc-100 disabled:opacity-50"
            >
              <RefreshCw className={`h-3.5 w-3.5 ${query.isFetching ? "animate-spin" : ""}`} />
            </button>
          }
        />

        <header className="mb-7 flex flex-col justify-between gap-5 lg:flex-row lg:items-end">
          <div>
            <div className="mb-2 flex items-center gap-2 text-xs font-medium uppercase tracking-[0.18em] text-violet-300">
              <ShieldCheck className="h-3.5 w-3.5" /> Fair, baseline-ranked performance
            </div>
            <h1 className="text-3xl font-medium tracking-tight">Leaderboard</h1>
            <p className="mt-2 max-w-2xl text-sm text-zinc-400">
              Compare strategy performance from a locked index of 100. No wealth rankings.
            </p>
          </div>
          <TimeframeTabs value={timeframe} onChange={setTimeframe} />
        </header>

        <div className="mt-6 grid gap-6 lg:grid-cols-[minmax(0,1fr)_21rem]">
          <section aria-labelledby="rankings-title">
            <div className="mb-3 flex items-center justify-between gap-3">
              <div>
                <h2 id="rankings-title" className="text-sm font-semibold text-zinc-100">Ranked strategies</h2>
                <p className="mt-1 text-xs text-zinc-500">{timeframe} performance from each strategy baseline</p>
              </div>
              <span className="font-mono text-xs text-zinc-600">{entries.length} ranked</span>
            </div>
            {query.isLoading ? (
              <LeaderboardSkeleton />
            ) : query.isError ? (
              <div className="rounded-2xl border border-rose-400/15 bg-rose-400/[0.05] px-5 py-12 text-center">
                <p className="text-sm text-rose-200">Leaderboard is temporarily unavailable.</p>
                <button type="button" onClick={() => query.refetch()} className="mt-4 text-xs font-medium text-zinc-300 underline underline-offset-4">Retry</button>
              </div>
            ) : (
              <RankedLeaderboard entries={entries} />
            )}
          </section>

          <aside className="space-y-6">
            <LeaderboardAchievements />
          </aside>
        </div>
      </main>
    </div>
  );
}
