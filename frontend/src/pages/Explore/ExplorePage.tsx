import { Compass, MessageCircle, RefreshCw, Sparkles, UsersRound } from "lucide-react";
import { useMemo, useState } from "react";
import { useSearchParams } from "react-router-dom";

import { AppNav } from "@/components/layout/AppNav";
import { ExploreFilterBar } from "@/components/explore/ExploreFilterBar";
import { ExploreSearchResults } from "@/components/explore/ExploreSearchResults";
import { FriendsPanel } from "@/components/explore/FriendsPanel";
import { ExploreSkeleton } from "@/components/explore/ExploreSkeleton";
import { FeaturedStrategies } from "@/components/explore/FeaturedStrategies";
import { MessagesPanel } from "@/components/explore/MessagesPanel";
import { SimilarStrategies } from "@/components/explore/SimilarStrategies";
import { TrendingHoldingsCard } from "@/components/explore/TrendingHoldingsCard";
import { Button } from "@/components/ui/button";
import { useExplore } from "@/hooks/useExplore";
import { TimeframeTabs } from "@/pages/leaderboard/TimeframeTabs";
import type { ExploreSort, ExploreTimeframe } from "@/types/explore";

const PAGE_SIZE = 20;
const SORTS: ExploreSort[] = ["top", "return", "rank", "recent"];
const TIMEFRAMES: ExploreTimeframe[] = ["1W", "1M", "3M", "6M", "1Y", "ALL"];
const EXPLORE_TABS = ["strategies", "friends", "messages"] as const;
type ExploreTab = (typeof EXPLORE_TABS)[number];
const TAB_ICONS = {
  strategies: Compass,
  friends: UsersRound,
  messages: MessageCircle,
} satisfies Record<ExploreTab, typeof Compass>;

export function ExplorePage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const tabParam = searchParams.get("tab") as ExploreTab | null;
  const activeTab =
    tabParam && EXPLORE_TABS.includes(tabParam) ? tabParam : "strategies";

  const setActiveTab = (tab: ExploreTab) => {
    const next = new URLSearchParams(searchParams);
    next.set("tab", tab);
    if (tab !== "messages") {
      next.delete("conversationId");
      next.delete("handle");
    }
    setSearchParams(next);
  };

  return (
    <div className="explore-shell min-h-screen bg-zinc-950 text-zinc-50">
      <main className="mx-auto w-full max-w-7xl px-4 pb-16 pt-4 sm:px-6 lg:px-8">
        <AppNav />

        <header className="relative mb-5 overflow-hidden rounded-2xl border border-violet-300/10 bg-[radial-gradient(circle_at_top_left,rgba(167,139,250,0.11),transparent_34%),radial-gradient(circle_at_85%_20%,rgba(56,189,248,0.07),transparent_25%),rgba(24,24,27,0.38)] px-5 py-6 shadow-[0_24px_80px_rgba(0,0,0,0.18)] sm:px-7">
          <div className="pointer-events-none absolute right-8 top-5 h-24 w-24 rounded-full border border-violet-300/[0.06]" />
          <div className="relative">
            <div className="flex items-center gap-2 text-[9px] font-medium uppercase tracking-[0.2em] text-violet-200/55">
              <Sparkles className="h-3 w-3" />
              Strategy network
            </div>
            <h1 className="explore-display mt-2 text-3xl font-semibold tracking-tight text-zinc-50 sm:text-4xl">
              Discover distinct investment thinking.
            </h1>
            <p className="mt-2 max-w-2xl text-sm leading-6 text-zinc-400">
              Find public strategies, study comparable approaches, and connect with investors you follow.
            </p>
          </div>
        </header>

        <div className="mb-7 inline-flex max-w-full gap-1 overflow-x-auto rounded-xl border border-zinc-800/90 bg-zinc-900/55 p-1 shadow-[0_10px_30px_rgba(0,0,0,0.14)]">
          {EXPLORE_TABS.map((tab) => (
            (() => {
              const Icon = TAB_ICONS[tab];
              return (
                <button
                  key={tab}
                  type="button"
                  onClick={() => setActiveTab(tab)}
                  className={`flex items-center gap-2 rounded-lg px-3.5 py-2 text-sm font-medium capitalize transition ${
                    activeTab === tab
                      ? "bg-zinc-100 text-zinc-950 shadow-sm"
                      : "text-zinc-400 hover:bg-zinc-800/70 hover:text-zinc-100"
                  }`}
                >
                  <Icon className="h-3.5 w-3.5" />
                  {tab}
                </button>
              );
            })()
          ))}
        </div>

        {activeTab === "strategies" ? (
          <StrategyExplorePanel />
        ) : activeTab === "friends" ? (
          <FriendsPanel />
        ) : (
          <MessagesPanel />
        )}
      </main>
    </div>
  );
}

function StrategyExplorePanel() {
  const [searchParams, setSearchParams] = useSearchParams();
  const q = searchParams.get("q") ?? "";
  const symbol = searchParams.get("symbol") ?? "";
  const sortParam = searchParams.get("sort") as ExploreSort | null;
  const sort = sortParam && SORTS.includes(sortParam) ? sortParam : "top";
  const timeframeParam = searchParams.get("timeframe") as ExploreTimeframe | null;
  const timeframe =
    timeframeParam && TIMEFRAMES.includes(timeframeParam) ? timeframeParam : "ALL";
  const offset = Math.max(0, Number(searchParams.get("offset")) || 0);

  const [draftQuery, setDraftQuery] = useState(q);
  const [draftSymbol, setDraftSymbol] = useState(symbol);

  const params = useMemo(
    () => ({ q, symbol, sort, timeframe, limit: PAGE_SIZE, offset }),
    [q, symbol, sort, timeframe, offset],
  );
  const query = useExplore(params);
  const searchProfiles = query.data?.top_performers ?? [];
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
    <section className="space-y-5">
      <div className="flex flex-col justify-between gap-3 border-b border-zinc-800/80 pb-4 sm:flex-row sm:items-start">
        <div>
          <h2 className="explore-display text-xl font-semibold text-zinc-100">Strategy discovery</h2>
          <p className="mt-1 max-w-2xl text-sm text-zinc-400">
            Curated profiles remain stable while search gives you direct, independent results.
          </p>
        </div>
        <button
          type="button"
          onClick={() => query.refetch()}
          disabled={query.isFetching}
          aria-label="Refresh Explore"
          className="self-start rounded-lg border border-zinc-800 p-2 text-zinc-400 transition hover:bg-zinc-800/70 hover:text-zinc-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-500 disabled:opacity-50"
        >
          <RefreshCw className={`h-3.5 w-3.5 ${query.isFetching ? "animate-spin" : ""}`} />
        </button>
      </div>

        <div className="mb-5 flex flex-col justify-between gap-3 rounded-xl border border-zinc-800/70 bg-zinc-900/25 px-4 py-3 sm:flex-row sm:items-center">
          <div>
            <h2 className="text-sm font-semibold text-zinc-100">Discovery timeframe</h2>
            <p className="mt-1 text-xs text-zinc-500">Find strategies by selected ranked window.</p>
          </div>
          <TimeframeTabs
            value={timeframe}
            onChange={(value) => updateParams({ timeframe: value, offset: 0 })}
          />
        </div>

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
            updateParams({ q: "", symbol: "", sort: "top", offset: 0 });
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
        ) : (
          <div className="space-y-7">
            {filtered ? (
              <ExploreSearchResults
                profiles={searchProfiles}
                query={q}
                symbol={symbol}
              />
            ) : null}
            <div className="grid items-start gap-6 lg:grid-cols-[minmax(0,1fr)_280px]">
              <div className="min-w-0 space-y-7">
                <FeaturedStrategies profiles={query.data?.featured ?? []} />
                <SimilarStrategies profiles={query.data?.similar ?? []} />
              </div>
              <aside className="lg:sticky lg:top-4">
                <TrendingHoldingsCard
                  holdings={query.data?.trending_holdings ?? []}
                  selectedSymbol={symbol}
                  onSelectSymbol={(value) => {
                    setDraftSymbol(value);
                    updateParams({ symbol: value, timeframe, offset: 0 });
                  }}
                />
              </aside>
            </div>
          </div>
        )}
    </section>
  );
}
