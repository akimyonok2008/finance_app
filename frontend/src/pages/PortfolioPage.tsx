import type { LucideIcon } from "lucide-react";
import { ArrowLeftRight, WalletCards } from "lucide-react";
import { useSearchParams } from "react-router-dom";

import { useAuth } from "@/auth/useAuth";
import { AppNav } from "@/components/layout/AppNav";
import { AssetTypeBadge } from "@/components/portfolio/AssetTypeBadge";
import { CashBalancesCard } from "@/components/portfolio/CashBalancesCard";
import { Card } from "@/components/ui/card";
import { PortfolioSummaryCards } from "@/components/portfolio/PortfolioSummaryCards";
import { PositionCardList } from "@/components/portfolio/PositionCardList";
import { PositionsTable } from "@/components/portfolio/PositionsTable";
import { AutomaticIncome } from "@/components/portfolio/AutomaticIncome";
import { AutomaticAdjustments } from "@/components/portfolio/AutomaticAdjustments";
import { usePositionRows } from "@/hooks/usePositionRows";
import { useClosedPositions } from "@/hooks/usePositions";
import { ActivityTabContent } from "@/pages/ActivityPage";
import {
  type ClosedPosition,
} from "@/types/portfolio";
import { cn } from "@/utils/cn";
import { formatMoney } from "@/utils/formatMoney";
import { formatPercent } from "@/utils/formatPercent";
import { gainLossColor } from "@/utils/gainLoss";

const PORTFOLIO_TABS = ["transactions", "state"] as const;
type PortfolioTab = (typeof PORTFOLIO_TABS)[number];

const TAB_META: Record<PortfolioTab, { label: string; hint: string; icon: LucideIcon }> = {
  transactions: { label: "Transactions", hint: "What happened", icon: ArrowLeftRight },
  state: { label: "Portfolio", hint: "What you own", icon: WalletCards },
};

const STATE_VIEWS = ["open", "closed", "cash"] as const;
type StateView = (typeof STATE_VIEWS)[number];
const STATE_VIEW_LABELS: Record<StateView, string> = {
  open: "Open positions",
  closed: "Closed positions",
  cash: "Cash",
};

export function PortfolioPage() {
  const { rows, isLoading, isError, error } = usePositionRows();
  const { user } = useAuth();
  const [searchParams, setSearchParams] = useSearchParams();

  const tabParam = searchParams.get("tab") as PortfolioTab | null;
  const activeTab =
    tabParam && PORTFOLIO_TABS.includes(tabParam) ? tabParam : "state";

  const viewParam = searchParams.get("view") as StateView | null;
  const activeView =
    viewParam && STATE_VIEWS.includes(viewParam) ? viewParam : "open";

  const errorMessage = error?.message;

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
              ? `${user.display_name}'s holdings and portfolio history.`
              : "Track your holdings and review your portfolio history."}
          </p>
        </div>

        <div className="mb-8 grid grid-cols-1 gap-2 sm:grid-cols-2">
          {PORTFOLIO_TABS.map((tab) => {
            const meta = TAB_META[tab];
            const Icon = meta.icon;
            const active = activeTab === tab;
            return (
              <button
                key={tab}
                type="button"
                onClick={() => setActiveTab(tab)}
                aria-current={active ? "page" : undefined}
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
                  <span className="block text-xs text-zinc-500">{meta.hint}</span>
                </span>
              </button>
            );
          })}
        </div>

        {activeTab === "state" ? (
          <div>
            <PortfolioSummaryCards />

            <div className="mb-6 mt-6 inline-flex flex-wrap gap-1 rounded-xl border border-indigo-300/10 bg-zinc-900/45 p-1 shadow-lg shadow-black/10">
              {STATE_VIEWS.map((view) => (
                <button
                  key={view}
                  type="button"
                  onClick={() => setActiveView(view)}
                  className={cn(
                    "rounded-lg px-3.5 py-2 text-xs font-semibold tracking-wide transition",
                    activeView === view
                      ? "bg-gradient-to-r from-indigo-200 to-cyan-100 text-zinc-950 shadow-sm shadow-cyan-950/20"
                      : "text-zinc-500 hover:bg-zinc-800/70 hover:text-zinc-100",
                  )}
                >
                  {STATE_VIEW_LABELS[view]}
                </button>
              ))}
            </div>

            {activeView === "open" ? (
              <div>
                <PositionsTable
                  rows={rows}
                  isLoading={isLoading}
                  isError={isError}
                  errorMessage={errorMessage}
                  onClose={(row) => goToSell(row.id)}
                />
                <PositionCardList
                  rows={rows}
                  isLoading={isLoading}
                  isError={isError}
                  errorMessage={errorMessage}
                  onClose={(row) => goToSell(row.id)}
                  onAdd={() => setActiveTab("transactions")}
                />
                <AutomaticIncome />
                <AutomaticAdjustments />
              </div>
            ) : activeView === "closed" ? (
              <ClosedPositionsView />
            ) : (
              <CashBalancesCard />
            )}
          </div>
        ) : (
          <ActivityTabContent />
        )}
      </main>
    </div>
  );
}

function ClosedPositionsView() {
  const { data, isLoading, isError, error } = useClosedPositions();
  const rows = data ?? [];

  if (isLoading) {
    return (
      <Card className="border-cyan-300/10 bg-cyan-300/[0.025] p-6 text-sm text-zinc-400">
        Loading closed positions…
      </Card>
    );
  }
  if (isError) {
    return (
      <Card className="p-6 text-sm text-rose-300">
        {(error as Error)?.message ?? "Could not load closed positions."}
      </Card>
    );
  }
  if (rows.length === 0) {
    return (
      <Card className="border-violet-300/10 bg-violet-300/[0.025] p-8 text-center">
        <h2 className="portfolio-display text-xl font-semibold text-zinc-100">No closed positions</h2>
        <p className="mt-2 text-sm text-zinc-400">
          Closed positions will appear here after you mark an asset as sold.
          No trade is placed.
        </p>
      </Card>
    );
  }
  return (
    <div className="grid gap-3">
      {rows.map((row) => (
        <ClosedPositionCard key={row.id} row={row} />
      ))}
    </div>
  );
}

function ClosedPositionCard({ row }: { row: ClosedPosition }) {
  const closedDate = row.closed_at
    ? new Date(row.closed_at).toLocaleDateString()
    : "—";
  return (
    <Card className="border-amber-300/10 bg-[linear-gradient(110deg,rgba(251,191,36,0.035),rgba(24,24,27,0.5)_35%)] p-4 transition hover:border-amber-300/20">
      <div className="grid gap-4 lg:grid-cols-[1fr_repeat(5,minmax(0,0.8fr))] lg:items-center">
        <div>
          <div className="flex items-center gap-2">
            <span className="font-mono font-medium tracking-wide text-amber-100">
              {row.symbol}
            </span>
            <AssetTypeBadge type={row.asset_type} />
          </div>
          <p className="mt-1 text-xs text-zinc-500">Closed {closedDate}</p>
        </div>
        <ClosedStat label="Quantity" value={String(row.quantity)} />
        <ClosedStat
          label="Baseline"
          value={formatMoney(row.baseline_price, row.baseline_currency)}
        />
        <ClosedStat
          label="Close price"
          value={formatMoney(row.close_price, row.close_price_currency)}
        />
        <ClosedStat
          label="Realized P&L"
          value={formatMoney(row.realized_gain_loss_base, row.base_currency)}
          className={gainLossColor(row.realized_gain_loss_base)}
        />
        <ClosedStat
          label="Realized %"
          value={formatPercent(row.realized_gain_loss_percentage)}
          className={gainLossColor(row.realized_gain_loss_percentage)}
        />
      </div>
    </Card>
  );
}

function ClosedStat({
  label,
  value,
  className,
}: {
  label: string;
  value: string;
  className?: string;
}) {
  return (
    <div>
      <div className="text-[11px] uppercase tracking-wide text-zinc-500">
        {label}
      </div>
      <div className={cn("mt-1 font-mono text-sm tabular-nums text-zinc-200", className)}>
        {value}
      </div>
    </div>
  );
}
