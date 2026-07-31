import { motion } from "framer-motion";
import { useState } from "react";

import { type ArenaBucket } from "@/api/arenaApi";
import { AppNav } from "@/components/layout/AppNav";
import { Button } from "@/components/ui/button";
import { useArenaCatalogueBucket } from "@/hooks/useArena";
import { ArenaEmptyState } from "@/pages/arena/ArenaEmptyState";
import { ArenaSkeleton } from "@/pages/arena/ArenaSkeleton";
import { CompetitionCard } from "@/pages/arena/CompetitionCard";
import { cn } from "@/utils/cn";

const CATALOGUE_TABS: { label: string; value: ArenaBucket; accent: string }[] = [
  { label: "Live", value: "live", accent: "from-emerald-300 to-teal-400" },
  { label: "My competitions", value: "mine", accent: "from-violet-300 to-fuchsia-400" },
  { label: "Upcoming", value: "upcoming", accent: "from-sky-300 to-indigo-400" },
  { label: "Completed", value: "completed", accent: "from-amber-200 to-orange-400" },
];

function CatalogueTab({ bucket }: { bucket: ArenaBucket }) {
  const query = useArenaCatalogueBucket(bucket);
  const cards = query.data?.pages.flatMap((page) => page.items) ?? [];

  if (query.isLoading) return <ArenaSkeleton />;
  if (query.isError) return <ArenaEmptyState error onRetry={() => query.refetch()} />;
  if (cards.length === 0) return <ArenaEmptyState />;

  return (
    <div className="space-y-8">
      <div className="grid grid-cols-1 gap-5 md:grid-cols-2 xl:grid-cols-3">
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
            className="rounded-full border-white/15 bg-white/[0.04] px-6 text-zinc-200 hover:bg-white/[0.08]"
          >
            {query.isFetchingNextPage ? "Loading…" : "Load more competitions"}
          </Button>
        </div>
      )}
    </div>
  );
}

export function ArenaPage() {
  const [catalogueTab, setCatalogueTab] = useState<ArenaBucket>("live");

  return (
    <div className="arena-shell min-h-screen bg-[radial-gradient(circle_at_12%_5%,rgba(124,58,237,0.13),transparent_25%),radial-gradient(circle_at_88%_12%,rgba(14,165,233,0.10),transparent_24%),radial-gradient(circle_at_52%_94%,rgba(16,185,129,0.07),transparent_28%),#09090b] text-zinc-50">
      <main className="mx-auto w-full max-w-[1440px] px-4 py-4 sm:px-6 lg:px-8">
        <AppNav />

        <motion.header
          initial={{ opacity: 0, y: -10 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.35 }}
          className="mb-8 border-b border-white/[0.07] pb-8 pt-3 sm:mb-10 sm:pb-10"
        >
          <p className="font-mono text-[10px] font-medium uppercase tracking-[0.28em] text-violet-300">
            Alarvest competitions
          </p>
          <div className="mt-3 flex flex-col justify-between gap-5 lg:flex-row lg:items-end">
            <div>
              <h1 className="arena-display bg-gradient-to-r from-white via-violet-100 to-sky-200 bg-clip-text text-4xl font-semibold tracking-[-0.035em] text-transparent sm:text-5xl">
                Arena
              </h1>
              <p className="mt-3 max-w-2xl text-sm leading-6 text-zinc-400 sm:text-base">
                Focused, time-bound portfolio competitions with transparent rules and
                independent rankings.
              </p>
            </div>
            <p className="max-w-sm border-l border-amber-300/30 pl-4 text-xs leading-5 text-zinc-500">
              Competition returns are isolated from the global leaderboard and scored
              under each edition&apos;s published rules.
            </p>
          </div>
        </motion.header>

        <section aria-labelledby="arena-catalogue-heading">
          <div className="mb-6 flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
            <div>
              <p className="font-mono text-[10px] uppercase tracking-[0.22em] text-emerald-300/80">
                Competition catalogue
              </p>
              <h2 id="arena-catalogue-heading" className="arena-display mt-1 text-2xl font-semibold text-zinc-100">
                Choose your field
              </h2>
            </div>
            <div
              role="tablist"
              aria-label="Arena catalogue tabs"
              className="flex max-w-full gap-1 overflow-x-auto rounded-2xl border border-white/[0.08] bg-black/25 p-1.5 shadow-[inset_0_1px_0_rgba(255,255,255,0.04)]"
            >
              {CATALOGUE_TABS.map((tab) => {
                const selected = catalogueTab === tab.value;
                return (
                  <button
                    key={tab.value}
                    type="button"
                    role="tab"
                    aria-selected={selected}
                    onClick={() => setCatalogueTab(tab.value)}
                    className={cn(
                      "relative shrink-0 rounded-xl px-4 py-2.5 text-xs font-semibold transition duration-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-300 focus-visible:ring-offset-2 focus-visible:ring-offset-zinc-950 sm:text-sm",
                      selected ? "bg-white/[0.09] text-white shadow-sm" : "text-zinc-500 hover:bg-white/[0.04] hover:text-zinc-200",
                    )}
                  >
                    {tab.label}
                    {selected && <span className={cn("absolute inset-x-4 -bottom-0.5 h-px bg-gradient-to-r", tab.accent)} />}
                  </button>
                );
              })}
            </div>
          </div>
          <CatalogueTab key={catalogueTab} bucket={catalogueTab} />
        </section>
      </main>
    </div>
  );
}
