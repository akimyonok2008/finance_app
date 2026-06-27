import { RefreshCw } from "lucide-react";
import { useMemo, useState } from "react";
import { useSearchParams } from "react-router-dom";

import { AppNav } from "@/components/layout/AppNav";
import { ExploreEmptyState } from "@/components/explore/ExploreEmptyState";
import { ExploreFilterBar } from "@/components/explore/ExploreFilterBar";
import { ExplorePrivacyNote } from "@/components/explore/ExplorePrivacyNote";
import { ExploreSkeleton } from "@/components/explore/ExploreSkeleton";
import { FeaturedStrategies } from "@/components/explore/FeaturedStrategies";
import { SimilarStrategies } from "@/components/explore/SimilarStrategies";
import { TopPerformersList } from "@/components/explore/TopPerformersList";
import { TrendingHoldingsCard } from "@/components/explore/TrendingHoldingsCard";
import { Button } from "@/components/ui/button";
import { useExplore } from "@/hooks/useExplore";
import type { ExploreSort } from "@/types/explore";

const PAGE_SIZE = 20;
const SORTS: ExploreSort[] = ["top", "return", "rank", "recent"];

export function ExplorePage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const q = searchParams.get("q") ?? "";
  const symbol = searchParams.get("symbol") ?? "";
  const sortParam = searchParams.get("sort") as ExploreSort | null;
  const sort = sortParam && SORTS.includes(sortParam) ? sortParam : "top";
  const offset = Math.max(0, Number(searchParams.get("offset")) || 0);

  const [draftQuery, setDraftQuery] = useState(q);
  const [draftSymbol, setDraftSymbol] = useState(symbol);

  const params = useMemo(
    () => ({ q, symbol, sort, limit: PAGE_SIZE, offset }),
    [q, symbol, sort, offset],
  );
  const query = useExplore(params);
  const profiles = query.data?.top_performers ?? [];
  const filtered = q.length > 0 || symbol.length > 0 || sort !== "top";

  const updateParams = (updates: Record<string, string | number | undefined>) => {
    const next = new URLSearchParams(searchParams);
    for (const [key, value] of Object.entries(updates)) {
      if (value === undefined || value === "" || (key === "sort" && value === "top") || (key === "offset" && value === 0)) {
        next.delete(key);
      } else {
        next.set(key, String(value));
      }
    }
    setSearchParams(next);
  };

  return (
    <div className="min-h-screen bg-zinc-950 text-zinc-50">
      <main className="mx-auto w-full max-w-7xl px-4 pb-16 pt-4 sm:px-6 lg:px-8">
        <AppNav
          actions={
            <button
              type="button"
              onClick={() => query.refetch()}
              disabled={query.isFetching}
              aria-label="Refresh Explore"
              className="rounded-lg p-2 text-zinc-400 transition hover:bg-zinc-800/70 hover:text-zinc-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-500 disabled:opacity-50"
            >
              <RefreshCw className={`h-3.5 w-3.5 ${query.isFetching ? "animate-spin" : ""}`} />
            </button>
          }
        />

        <header className="mb-6">
          <h1 className="text-2xl font-medium tracking-tight sm:text-3xl">Explore Strategies</h1>
          <p className="mt-2 max-w-2xl text-sm text-zinc-400">Discover public portfolios by performance, symbols, and weights without seeing anyone’s net worth.</p>
          <p className="mt-2 text-xs text-zinc-600">Profiles show strategy weights, not quantities or values.</p>
        </header>

        <ExploreFilterBar
          query={draftQuery}
          symbol={draftSymbol}
          sort={sort}
          onQueryChange={setDraftQuery}
          onSymbolChange={setDraftSymbol}
          onSortChange={(value) => updateParams({ sort: value, offset: 0 })}
          onSubmit={() => updateParams({ q: draftQuery.trim(), symbol: draftSymbol.trim().toUpperCase(), offset: 0 })}
          onClear={() => {
            setDraftQuery("");
            setDraftSymbol("");
            setSearchParams({});
          }}
        />

        {query.isLoading ? (
          <ExploreSkeleton />
        ) : query.isError ? (
          <div className="rounded-2xl border border-rose-400/15 bg-rose-400/[0.04] px-6 py-14 text-center">
            <h2 className="text-lg font-semibold text-zinc-100">Could not load Explore.</h2>
            <p className="mt-2 text-sm text-zinc-500">Try refreshing or changing filters.</p>
            <Button variant="outline" className="mt-5" onClick={() => query.refetch()}><RefreshCw /> Retry</Button>
          </div>
        ) : profiles.length === 0 && (query.data?.featured.length ?? 0) === 0 ? (
          <div className="space-y-5">
            <ExploreEmptyState filtered={filtered} />
            <ExplorePrivacyNote />
          </div>
        ) : (
          <div className="grid items-start gap-6 xl:grid-cols-[minmax(0,1fr)_300px]">
            <div className="order-1 space-y-8 xl:col-start-1 xl:row-start-1">
              <FeaturedStrategies profiles={query.data?.featured ?? []} />
              <SimilarStrategies profiles={query.data?.similar ?? []} />
            </div>
            <div className="order-2 xl:col-start-2 xl:row-start-1">
              <TrendingHoldingsCard
                holdings={query.data?.trending_holdings ?? []}
                selectedSymbol={symbol}
                onSelectSymbol={(value) => {
                  setDraftSymbol(value);
                  updateParams({ symbol: value, offset: 0 });
                }}
              />
            </div>
            <div className="order-3 xl:col-start-1 xl:row-start-2">
              {profiles.length > 0 ? <TopPerformersList profiles={profiles} /> : <ExploreEmptyState filtered={filtered} />}
              <Pagination
                offset={offset}
                hasMore={query.data?.pagination?.has_more ?? false}
                onPrevious={() => updateParams({ offset: Math.max(0, offset - PAGE_SIZE) })}
                onNext={() => updateParams({ offset: offset + PAGE_SIZE })}
              />
            </div>
            <div className="order-4 xl:col-start-2 xl:row-start-2">
              <ExplorePrivacyNote />
            </div>
          </div>
        )}
      </main>
    </div>
  );
}

function Pagination({ offset, hasMore, onPrevious, onNext }: { offset: number; hasMore: boolean; onPrevious: () => void; onNext: () => void }) {
  if (offset === 0 && !hasMore) return null;
  return (
    <div className="mt-5 flex items-center justify-between">
      <Button variant="outline" size="sm" disabled={offset === 0} onClick={onPrevious}>Previous</Button>
      <span className="font-mono text-xs tabular-nums text-zinc-600">Page {Math.floor(offset / PAGE_SIZE) + 1}</span>
      <Button variant="outline" size="sm" disabled={!hasMore} onClick={onNext}>Next</Button>
    </div>
  );
}
