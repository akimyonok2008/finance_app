import { AnimatePresence, motion } from "framer-motion";
import { useState } from "react";

import { AppNav } from "@/components/layout/AppNav";
import { useAchievements, useArenaCatalogue } from "@/hooks/useArena";
import { ArenaEmptyState } from "@/pages/arena/ArenaEmptyState";
import { ArenaSkeleton } from "@/pages/arena/ArenaSkeleton";
import { CompetitionCard } from "@/pages/arena/CompetitionCard";
import { TrophyCase } from "@/pages/arena/TrophyCase";
import type { ArenaCompetitionCard } from "@/types/arena";
import { cn } from "@/utils/cn";

type MobileTab = "competitions" | "trophies";

function CompetitionSection({
  title,
  subtitle,
  competitions,
}: {
  title: string;
  subtitle?: string;
  competitions: ArenaCompetitionCard[];
}) {
  if (competitions.length === 0) return null;
  return (
    <section aria-label={title}>
      <div className="mb-4 flex items-baseline justify-between">
        <h2 className="text-sm font-semibold uppercase tracking-[0.14em] text-zinc-400">
          {title}
        </h2>
        {subtitle && <span className="text-xs text-zinc-500">{subtitle}</span>}
      </div>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {competitions.map((competition, index) => (
          <CompetitionCard key={competition.id} competition={competition} index={index} />
        ))}
      </div>
    </section>
  );
}

export function ArenaPage() {
  const catalogueQuery = useArenaCatalogue();
  const achievementsQuery = useAchievements();
  const [mobileTab, setMobileTab] = useState<MobileTab>("competitions");

  const cards = catalogueQuery.data ?? [];
  const live = cards.filter((c) => c.status === "active");
  const upcoming = cards.filter(
    (c) =>
      c.status !== "active" &&
      c.status !== "completed" &&
      c.status !== "cancelled" &&
      !c.joined,
  );
  const mine = cards.filter((c) => c.joined && c.status !== "completed");
  const completed = cards.filter((c) => c.status === "completed");

  const isLoading = catalogueQuery.isLoading || achievementsQuery.isLoading;
  const isError = catalogueQuery.isError;

  const competitionsView = (
    <div className="space-y-10">
      <CompetitionSection title="Live" competitions={live} />
      <CompetitionSection title="My competitions" competitions={mine} />
      <CompetitionSection title="Upcoming" competitions={upcoming} />
      <CompetitionSection title="Completed" competitions={completed} />
      {cards.length === 0 && !isError && <ArenaEmptyState />}
    </div>
  );

  const trophies = (
    <TrophyCase
      achievements={achievementsQuery.data}
      isError={Boolean(achievementsQuery.error)}
    />
  );

  return (
    <div className="min-h-screen bg-zinc-950 text-zinc-50">
      <main className="mx-auto w-full max-w-7xl px-4 py-4 sm:px-6 lg:px-8">
        <AppNav />

        <motion.header
          initial={{ opacity: 0, y: -10 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.35 }}
          className="mb-8"
        >
          <h1 className="text-3xl font-medium tracking-tight">Arena</h1>
          <p className="mt-2 text-sm text-zinc-400">
            Time-bound competitions, independent from the global leaderboard.
          </p>
        </motion.header>

        {isLoading ? (
          <ArenaSkeleton />
        ) : isError ? (
          <ArenaEmptyState error onRetry={() => catalogueQuery.refetch()} />
        ) : (
          <>
            <div className="hidden lg:grid lg:grid-cols-3 lg:gap-8">
              <div className="space-y-10 lg:col-span-2">{competitionsView}</div>
              <aside className="lg:col-span-1">{trophies}</aside>
            </div>

            <div className="lg:hidden">
              <div
                role="tablist"
                aria-label="Arena sections"
                className="mb-5 grid grid-cols-2 rounded-lg border border-zinc-800 bg-zinc-900/40 p-1"
              >
                {[
                  { label: "Competitions", value: "competitions" },
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
                  {mobileTab === "competitions" ? competitionsView : trophies}
                </motion.div>
              </AnimatePresence>
            </div>
          </>
        )}
      </main>
    </div>
  );
}
