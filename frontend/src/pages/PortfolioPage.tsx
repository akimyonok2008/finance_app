import type { KeyboardEvent } from "react";
import type { LucideIcon } from "lucide-react";
import { ArrowLeftRight, LineChart, WalletCards } from "lucide-react";
import { useSearchParams } from "react-router-dom";

import { useAuth } from "@/auth/useAuth";
import { AppNav } from "@/components/layout/AppNav";
import { PortfolioPerformanceTab } from "@/components/portfolio/PortfolioPerformanceTab";
import { PortfolioStateTab } from "@/components/portfolio/PortfolioStateTab";
import {
  PORTFOLIO_TABS,
  STATE_VIEWS,
  type PortfolioTab,
  type StateView,
} from "@/components/portfolio/portfolioTabs";
import { PortfolioTransactionsTab } from "@/components/portfolio/PortfolioTransactionsTab";
import { cn } from "@/utils/cn";

/**
 * The three separately-owned financial logics presented as one product area:
 *   Transactions    — what happened     (Activity ledger)
 *   Portfolio State — what exists now   (materialized positions/cash)
 *   Performance     — how did it perform (canonical ranked history)
 *
 * They stay separate services and DTOs on the backend; this page is only the
 * unified shell. Tab state lives in the URL so links, bookmarks, and browser
 * back/forward all work.
 */
const TAB_META: Record<
  PortfolioTab,
  { label: string; hint: string; icon: LucideIcon }
> = {
  transactions: {
    label: "Transactions",
    hint: "What happened",
    icon: ArrowLeftRight,
  },
  state: { label: "Portfolio", hint: "What you own", icon: WalletCards },
  performance: {
    label: "Performance",
    hint: "How it performed",
    icon: LineChart,
  },
};

export function PortfolioPage() {
  const { user } = useAuth();
  const [searchParams, setSearchParams] = useSearchParams();

  const tabParam = searchParams.get("tab") as PortfolioTab | null;
  const activeTab =
    tabParam && PORTFOLIO_TABS.includes(tabParam) ? tabParam : "state";

  const viewParam = searchParams.get("view") as StateView | null;
  const activeView =
    viewParam && STATE_VIEWS.includes(viewParam) ? viewParam : "open";

  const setActiveTab = (tab: PortfolioTab) => {
    const next = new URLSearchParams(searchParams);
    if (tab === "state") {
      next.delete("tab");
    } else {
      next.set("tab", tab);
    }
    setSearchParams(next);
  };
  const setActiveView = (view: StateView) => {
    const next = new URLSearchParams(searchParams);
    if (view === "open") {
      next.delete("view");
    } else {
      next.set("view", view);
    }
    setSearchParams(next);
  };
  /**
   * `?episode=<positions.id>` deep-links one position episode. Episode identity
   * IS the positions row id (see migration 0018), so the same value matches an
   * open position's `position_id` and a closed episode's `id`.
   */
  const episodeId = searchParams.get("episode") ?? undefined;

  const onTabKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    const index = PORTFOLIO_TABS.indexOf(activeTab);
    let next: number;
    if (event.key === "ArrowRight") next = (index + 1) % PORTFOLIO_TABS.length;
    else if (event.key === "ArrowLeft")
      next = (index - 1 + PORTFOLIO_TABS.length) % PORTFOLIO_TABS.length;
    else if (event.key === "Home") next = 0;
    else if (event.key === "End") next = PORTFOLIO_TABS.length - 1;
    else return;

    event.preventDefault();
    const target = PORTFOLIO_TABS[next];
    setActiveTab(target);
    document.getElementById(`portfolio-tab-${target}`)?.focus();
  };

  const goToSell = (positionId: string) => {
    const next = new URLSearchParams(searchParams);
    next.set("tab", "transactions");
    next.set("sell", positionId);
    setSearchParams(next);
  };

  return (
    <div className="portfolio-shell min-h-screen bg-[radial-gradient(circle_at_12%_8%,rgba(56,189,248,0.055),transparent_26%),radial-gradient(circle_at_88%_18%,rgba(167,139,250,0.06),transparent_28%),radial-gradient(circle_at_55%_85%,rgba(45,212,191,0.035),transparent_26%),#09090b] text-zinc-50">
      <main className="mx-auto w-full max-w-7xl px-4 pb-20 pt-4 sm:px-6 lg:px-8">
        <AppNav />

        <div className="mb-6 flex flex-col gap-1.5 border-b border-indigo-200/[0.06] pb-7">
          <span className="text-[10px] font-semibold uppercase tracking-[0.22em] text-cyan-200/55">
            Private portfolio
          </span>
          <h1 className="portfolio-display text-3xl font-semibold tracking-tight text-zinc-50 sm:text-4xl">
            Your portfolio
          </h1>
          <p className="max-w-2xl text-sm leading-6 text-zinc-400">
            {user?.display_name
              ? `${user.display_name}'s holdings, transactions, and performance.`
              : "Track your holdings, transactions, and performance."}
          </p>
        </div>

        {/*
          A real ARIA tablist: each control is a native <button role="tab"> with
          aria-selected, roving tabindex, and Left/Right/Home/End keyboard
          navigation, so the tab strip is operable without a mouse.
        */}
        <div
          role="tablist"
          aria-label="Portfolio sections"
          onKeyDown={onTabKeyDown}
          className="mb-8 grid grid-cols-1 gap-2 sm:grid-cols-3"
        >
          {PORTFOLIO_TABS.map((tab) => {
            const meta = TAB_META[tab];
            const Icon = meta.icon;
            const active = activeTab === tab;
            return (
              <button
                key={tab}
                type="button"
                role="tab"
                id={`portfolio-tab-${tab}`}
                aria-controls={`portfolio-tabpanel-${tab}`}
                aria-selected={active}
                tabIndex={active ? 0 : -1}
                onClick={() => setActiveTab(tab)}
                className={cn(
                  "flex items-center gap-3 rounded-2xl border px-4 py-3.5 text-left transition",
                  active
                    ? "border-cyan-300/30 bg-[radial-gradient(circle_at_top_left,rgba(34,211,238,0.12),transparent_60%),rgba(24,24,27,0.65)] shadow-lg shadow-cyan-950/10"
                    : "border-zinc-800 bg-zinc-900/40 hover:border-zinc-700 hover:bg-zinc-900/60",
                )}
              >
                <span
                  className={cn(
                    "grid h-10 w-10 shrink-0 place-items-center rounded-xl border",
                    active
                      ? "border-cyan-300/30 bg-cyan-300/10 text-cyan-200"
                      : "border-zinc-800 bg-zinc-950/40 text-zinc-500",
                  )}
                >
                  <Icon className="h-4.5 w-4.5" />
                </span>
                <span className="min-w-0">
                  <span
                    className={cn(
                      "block text-sm font-semibold tracking-tight",
                      active ? "text-zinc-50" : "text-zinc-200",
                    )}
                  >
                    {meta.label}
                  </span>
                  <span className="block text-xs text-zinc-500">
                    {meta.hint}
                  </span>
                </span>
              </button>
            );
          })}
        </div>

        <div
          role="tabpanel"
          id={`portfolio-tabpanel-${activeTab}`}
          aria-labelledby={`portfolio-tab-${activeTab}`}
          tabIndex={0}
        >
          {activeTab === "state" ? (
            <PortfolioStateTab
              view={activeView}
              episodeId={episodeId}
              onViewChange={setActiveView}
              onSell={goToSell}
              onAddPosition={() => setActiveTab("transactions")}
            />
          ) : activeTab === "transactions" ? (
            <PortfolioTransactionsTab />
          ) : (
            <PortfolioPerformanceTab />
          )}
        </div>
      </main>
    </div>
  );
}
