import { AnimatePresence, motion } from "framer-motion";
import { ArrowLeft } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";

import { useArena } from "@/hooks/useArena";
import { ArenaEmptyState } from "@/pages/arena/ArenaEmptyState";
import { ArenaSkeleton } from "@/pages/arena/ArenaSkeleton";
import { CohortLeaderboard } from "@/pages/arena/CohortLeaderboard";
import { SprintLobby } from "@/pages/arena/SprintLobby";
import { TrophyCase } from "@/pages/arena/TrophyCase";
import { cn } from "@/utils/cn";

type MobileTab = "sprint" | "trophies";

export function ArenaPage() {
  const arena = useArena();
  const [mobileTab, setMobileTab] = useState<MobileTab>("sprint");

  const liveSprint = (
    <div className="space-y-8">
      <SprintLobby
        sprint={arena.sprint}
        isError={Boolean(arena.errors.sprint)}
        onJoin={arena.joinSprint}
        isJoining={arena.isJoiningSprint}
      />
      <CohortLeaderboard
        entries={arena.leaderboard}
        isError={Boolean(arena.errors.leaderboard)}
      />
    </div>
  );

  const trophies = (
    <TrophyCase
      achievements={arena.achievements}
      isError={Boolean(arena.errors.achievements)}
    />
  );

  return (
    <div className="min-h-screen bg-zinc-950 text-zinc-50">
      <main className="mx-auto w-full max-w-7xl px-4 py-6 sm:px-6 lg:px-8">
        <Link
          to="/dashboard"
          className="mb-8 inline-flex items-center gap-2 rounded-lg border border-zinc-800 bg-zinc-900/40 px-3 py-2 text-xs font-medium text-zinc-400 transition hover:border-zinc-700 hover:bg-zinc-900/70 hover:text-zinc-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-500"
        >
          <ArrowLeft className="h-3.5 w-3.5" />
          Back to dashboard
        </Link>

        <motion.header
          initial={{ opacity: 0, y: -10 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.35 }}
          className="mb-8"
        >
          <h1 className="text-3xl font-medium tracking-tight">
            Arena
          </h1>
          <p className="mt-2 text-sm text-zinc-400">Sprints, rankings, and trophies.</p>
        </motion.header>

        {arena.isLoading ? (
          <ArenaSkeleton />
        ) : arena.isError ? (
          <ArenaEmptyState error onRetry={arena.refetch} />
        ) : (
          <>
            <div className="hidden lg:grid lg:grid-cols-3 lg:gap-8">
              <section className="space-y-8 lg:col-span-2">
                {liveSprint}
              </section>
              <aside className="lg:col-span-1">{trophies}</aside>
            </div>

            <div className="lg:hidden">
              <div
                role="tablist"
                aria-label="Arena sections"
                className="mb-5 grid grid-cols-2 rounded-lg border border-zinc-800 bg-zinc-900/40 p-1"
              >
                {[
                  { label: "Live Sprint", value: "sprint" },
                  { label: "My Trophies", value: "trophies" },
                ].map((tab) => (
                  <button
                    key={tab.value}
                    type="button"
                    role="tab"
                    aria-selected={mobileTab === tab.value}
                    onClick={() => setMobileTab(tab.value as MobileTab)}
                    className={cn(
                      "rounded-md px-4 py-2.5 text-xs font-medium transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-500",
                      mobileTab === tab.value
                        ? "bg-zinc-50 text-zinc-950"
                        : "text-zinc-400 hover:bg-zinc-800/60 hover:text-zinc-100",
                    )}
                  >
                    {tab.label}
                  </button>
                ))}
              </div>
              <AnimatePresence mode="wait">
                <motion.div
                  key={mobileTab}
                  initial={{ opacity: 0, y: 8 }}
                  animate={{ opacity: 1, y: 0 }}
                  exit={{ opacity: 0, y: -8 }}
                  transition={{ duration: 0.18 }}
                >
                  {mobileTab === "sprint" ? liveSprint : trophies}
                </motion.div>
              </AnimatePresence>
            </div>
          </>
        )}
      </main>
    </div>
  );
}
