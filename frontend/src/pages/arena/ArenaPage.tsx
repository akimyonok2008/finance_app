import { AnimatePresence, motion } from "framer-motion";
import { useState } from "react";

import { AppNav } from "@/components/layout/AppNav";
import { Button } from "@/components/ui/button";
import { type ArenaBucket } from "@/api/arenaApi";
import { useAchievements, useArenaCatalogueBucket } from "@/hooks/useArena";
import { ArenaEmptyState } from "@/pages/arena/ArenaEmptyState";
import { ArenaSkeleton } from "@/pages/arena/ArenaSkeleton";
import { CompetitionCard } from "@/pages/arena/CompetitionCard";
import { TrophyCase } from "@/pages/arena/TrophyCase";
import { cn } from "@/utils/cn";

type MobileTab = "competitions" | "trophies";

const CATALOGUE_TABS: { label: string; value: ArenaBucket }[] = [
  { label: "Live", value: "live" },
  { label: "My competitions", value: "mine" },
  { label: "Upcoming", value: "upcoming" },
  { label: "Completed", value: "completed" },
];

function CatalogueTab({ bucket }: { bucket: ArenaBucket }) {
  const query = useArenaCatalogueBucket(bucket);
  const cards = query.data?.pages.flatMap((page) => page.items) ?? [];

  if (query.isLoading) {
    return (
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {[0, 1, 2].map((i) => (
          <div key={i} className="rounded-2xl border border-zinc-800 bg-zinc-900/40 p-5">
            <div className="h-4 w-16 animate-pulse rounded bg-zinc-800" />
            <div className="mt-3 h-5 w-40 animate-pulse rounded bg-zinc-800" />
            <div className="mt-3 h-8 w-full animate-pulse rounded bg-zinc-800" />
          </div>
        ))}
      </div>
    );
  }

  if (query.isError) {
    return <ArenaEmptyState error onRetry={() => query.refetch()} />;
  }

  if (cards.length === 0) {
    return <ArenaEmptyState />;
  }

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {cards.map((competition, index) => (
          <CompetitionCard key={competition.id} competition={competition} index={index} />
        ))}
      </div>
      {query.hasNextPage && (
        <div className="flex justify-center">
          <Button
            variant="outline"
            onClick={() => query.fetchNextPage()}
            disabled={query.isFetchingNextPage}
          >
            {query.isFetchingNextPage ? "Loading…" : "Load more"}
          </Button>
        </div>
      )}
    </div>
  );
}

export function ArenaPage() {
  const achievementsQuery = useAchievements();
  const [mobileTab, setMobileTab] = useState<MobileTab>("competitions");
  const [catalogueTab, setCatalogueTab] = useState<ArenaBucket>("live");

  const competitionsView = (
    <div className="space-y-6">
      <div
        role="tablist"
        aria-label="Arena catalogue tabs"
        className="flex flex-wrap gap-2 border-b border-zinc-800 pb-3"
      >
        {CATALOGUE_TABS.map((tab) => (
          <button
            key={tab.value}
            type="button"
            role="tab"
            aria-selected={catalogueTab === tab.value}
            onClick={() => setCatalogueTab(tab.value)}
            className={cn(
              "rounded-full px-4 py-1.5 text-sm font-medium transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-500",
              catalogueTab === tab.value
                ? "bg-zinc-50 text-zinc-950"
                : "text-zinc-400 hover:bg-zinc-800/60 hover:text-zinc-100",
            )}
          >
            {tab.label}
          </button>
        ))}
      </div>
      <CatalogueTab key={catalogueTab} bucket={catalogueTab} />
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

        {achievementsQuery.isLoading ? (
          <ArenaSkeleton />
        ) : (
          <>
            <div className="hidden lg:grid lg:grid-cols-3 lg:gap-8">
              <div className="lg:col-span-2">{competitionsView}</div>
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
