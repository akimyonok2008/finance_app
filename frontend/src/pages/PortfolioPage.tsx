import { useState } from "react";
import { Plus } from "lucide-react";
import { useSearchParams } from "react-router-dom";

import { useAuth } from "@/auth/useAuth";
import { AppNav } from "@/components/layout/AppNav";
import { AssetTypeBadge } from "@/components/portfolio/AssetTypeBadge";
import { ClosePositionDialog } from "@/components/portfolio/ClosePositionDialog";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
} from "@/components/ui/drawer";
import { AddPositionForm } from "@/components/portfolio/AddPositionForm";
import { DeletePositionDialog } from "@/components/portfolio/DeletePositionDialog";
import { EditPositionModal } from "@/components/portfolio/EditPositionModal";
import { PortfolioSummaryCards } from "@/components/portfolio/PortfolioSummaryCards";
import { PositionCardList } from "@/components/portfolio/PositionCardList";
import { PositionsTable } from "@/components/portfolio/PositionsTable";
import { usePositionRows, type PositionRow } from "@/hooks/usePositionRows";
import { useClosedPositions, usePortfolioArchives } from "@/hooks/usePositions";
import {
  PORTFOLIO_ARCHIVE_TIMEFRAMES,
  type ClosedPosition,
  type PortfolioArchiveTimeframe,
} from "@/types/portfolio";
import { cn } from "@/utils/cn";
import { formatMoney } from "@/utils/formatMoney";
import { formatPercent } from "@/utils/formatPercent";
import { gainLossColor } from "@/utils/gainLoss";

const PORTFOLIO_TABS = ["positions", "closed", "archive"] as const;
type PortfolioTab = (typeof PORTFOLIO_TABS)[number];

export function PortfolioPage() {
  const { rows, isLoading, isError, error } = usePositionRows();
  const { user } = useAuth();
  const [searchParams, setSearchParams] = useSearchParams();
  const tabParam = searchParams.get("tab") as PortfolioTab | null;
  const activeTab =
    tabParam && PORTFOLIO_TABS.includes(tabParam) ? tabParam : "positions";

  const [editTarget, setEditTarget] = useState<PositionRow | null>(null);
  const [closeTarget, setCloseTarget] = useState<PositionRow | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<PositionRow | null>(null);
  const [drawerOpen, setDrawerOpen] = useState(false);

  const errorMessage = error?.message;
  const setActiveTab = (tab: PortfolioTab) => {
    const next = new URLSearchParams(searchParams);
    if (tab === "positions") {
      next.delete("tab");
    } else {
      next.set("tab", tab);
    }
    setSearchParams(next);
  };

  return (
    <div className="portfolio-shell min-h-screen bg-[radial-gradient(circle_at_12%_8%,rgba(56,189,248,0.055),transparent_26%),radial-gradient(circle_at_88%_18%,rgba(167,139,250,0.06),transparent_28%),radial-gradient(circle_at_55%_85%,rgba(45,212,191,0.035),transparent_26%),#09090b] text-zinc-50">
      <main className="mx-auto w-full max-w-7xl px-4 pb-20 pt-4 sm:px-6 lg:px-8">
        <AppNav />

        <div className="mb-8 flex flex-col gap-1.5 border-b border-indigo-200/[0.06] pb-7">
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

        <div className="mb-6 inline-flex flex-wrap gap-1 rounded-xl border border-indigo-300/10 bg-zinc-900/45 p-1 shadow-lg shadow-black/10">
          {PORTFOLIO_TABS.map((tab) => (
            <button
              key={tab}
              type="button"
              onClick={() => setActiveTab(tab)}
              className={cn(
                "rounded-lg px-3.5 py-2 text-xs font-semibold capitalize tracking-wide transition",
                activeTab === tab
                  ? "bg-gradient-to-r from-indigo-200 to-cyan-100 text-zinc-950 shadow-sm shadow-cyan-950/20"
                  : "text-zinc-500 hover:bg-zinc-800/70 hover:text-zinc-100",
              )}
            >
              {tab}
            </button>
          ))}
        </div>

        <PortfolioSummaryCards />

        <p className="mt-4 rounded-lg border border-indigo-300/10 bg-zinc-900/40 px-4 py-3 text-xs leading-5 text-zinc-400">
          <span className="font-semibold text-zinc-300">Portfolio changes take effect prospectively.</span>{" "}
          Adding money or changing positions does not reset or improve your past
          ranked performance — only market moves while you hold positions do. Emptying
          your portfolio pauses ranked tracking and preserves your accumulated index.
        </p>

        {activeTab === "positions" ? (
          <div className="mt-8 grid gap-6 lg:grid-cols-[420px_1fr]">
            <div className="hidden lg:block">
              <AddPositionForm />
            </div>

            <div>
              <PositionsTable
                rows={rows}
                isLoading={isLoading}
                isError={isError}
                errorMessage={errorMessage}
                onEdit={setEditTarget}
                onClose={setCloseTarget}
                onDelete={setDeleteTarget}
              />
              <PositionCardList
                rows={rows}
                isLoading={isLoading}
                isError={isError}
                errorMessage={errorMessage}
                onEdit={setEditTarget}
                onClose={setCloseTarget}
                onDelete={setDeleteTarget}
                onAdd={() => setDrawerOpen(true)}
              />
            </div>
          </div>
        ) : activeTab === "closed" ? (
          <ClosedPositionsView />
        ) : (
          <ArchiveView />
        )}
      </main>

      <div
        className={cn(
          "fixed inset-x-0 bottom-0 z-30 justify-center pb-[max(1rem,env(safe-area-inset-bottom))] lg:hidden",
          activeTab === "positions" ? "flex" : "hidden",
        )}
      >
        <Button
          variant="accent"
          size="lg"
          className="border border-zinc-700 shadow-lg shadow-black/30"
          onClick={() => setDrawerOpen(true)}
        >
          <Plus />
          Add Position
        </Button>
      </div>

      <Drawer open={drawerOpen} onOpenChange={setDrawerOpen}>
        <DrawerContent>
          <DrawerHeader>
            <DrawerTitle>Add Position</DrawerTitle>
            <DrawerDescription>
              Add a holding to track.
            </DrawerDescription>
          </DrawerHeader>
          <div className="overflow-y-auto px-4 pb-[max(1.5rem,env(safe-area-inset-bottom))]">
            <AddPositionForm compact onSuccess={() => setDrawerOpen(false)} />
          </div>
        </DrawerContent>
      </Drawer>

      <EditPositionModal
        position={editTarget}
        open={editTarget !== null}
        onOpenChange={(open) => !open && setEditTarget(null)}
      />
      <ClosePositionDialog
        position={closeTarget}
        open={closeTarget !== null}
        onOpenChange={(open) => !open && setCloseTarget(null)}
      />
      <DeletePositionDialog
        position={deleteTarget}
        open={deleteTarget !== null}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
      />
    </div>
  );
}

function ClosedPositionsView() {
  const { data, isLoading, isError, error } = useClosedPositions();
  const rows = data ?? [];

  if (isLoading) {
    return (
          <Card className="mt-8 border-cyan-300/10 bg-cyan-300/[0.025] p-6 text-sm text-zinc-400">
        Loading closed positions…
      </Card>
    );
  }
  if (isError) {
    return (
      <Card className="mt-8 p-6 text-sm text-rose-300">
        {(error as Error)?.message ?? "Could not load closed positions."}
      </Card>
    );
  }
  if (rows.length === 0) {
    return (
      <Card className="mt-8 border-violet-300/10 bg-violet-300/[0.025] p-8 text-center">
        <h2 className="portfolio-display text-xl font-semibold text-zinc-100">No closed positions</h2>
        <p className="mt-2 text-sm text-zinc-400">
          Closed positions will appear here after you mark an asset as sold.
          No trade is placed.
        </p>
      </Card>
    );
  }
  return (
    <div className="mt-8 grid gap-3">
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

function ArchiveView() {
  const [timeframe, setTimeframe] = useState<PortfolioArchiveTimeframe>("1M");
  const { data, isLoading, isError, error } = usePortfolioArchives(timeframe);
  const latest = data?.latest_snapshot;
  const first = data?.earliest_snapshot;
  const movement =
    latest && first ? latest.portfolio_index - first.portfolio_index : undefined;

  return (
    <div className="mt-8 space-y-4">
      <div className="inline-flex flex-wrap gap-1 rounded-xl border border-violet-300/10 bg-zinc-900/45 p-1">
        {PORTFOLIO_ARCHIVE_TIMEFRAMES.map((tf) => (
          <button
            key={tf}
            type="button"
            onClick={() => setTimeframe(tf)}
            className={cn(
              "rounded-lg px-3 py-2 text-xs font-semibold transition",
              timeframe === tf
                ? "bg-violet-200 text-violet-950 shadow-sm shadow-violet-950/20"
                : "text-zinc-400 hover:bg-zinc-800/70 hover:text-zinc-100",
            )}
          >
            {tf}
          </button>
        ))}
      </div>

      {isLoading ? (
        <Card className="p-6 text-sm text-zinc-400">Loading archive…</Card>
      ) : isError ? (
        <Card className="p-6 text-sm text-rose-300">
          {(error as Error)?.message ?? "Could not load portfolio archive."}
        </Card>
      ) : !data || data.points.length === 0 ? (
        <Card className="p-8 text-center">
          <h2 className="text-lg font-semibold text-zinc-100">
            No archive snapshots yet
          </h2>
          <p className="mt-2 text-sm text-zinc-400">
            Add, update, or close a position to record owner-private portfolio
            archive points.
          </p>
        </Card>
      ) : (
        <>
          <div className="grid gap-4 md:grid-cols-4">
            <ArchiveStat
              label={`${timeframe} index movement`}
              value={
                movement !== undefined
                  ? `${movement >= 0 ? "+" : ""}${movement.toFixed(2)}`
                  : "—"
              }
              className={gainLossColor(movement)}
              tone="cyan"
            />
            <ArchiveStat
              label="Selected return"
              value={formatPercent(latest?.gain_loss_percentage)}
              className={gainLossColor(latest?.gain_loss_percentage)}
              tone="violet"
            />
            <ArchiveStat
              label="Unrealized"
              value={formatMoney(latest?.unrealized_gain_loss_base, "USD")}
              className={gainLossColor(latest?.unrealized_gain_loss_base)}
              tone="amber"
            />
            <ArchiveStat
              label="Realized"
              value={formatMoney(latest?.realized_gain_loss_base, "USD")}
              className={gainLossColor(latest?.realized_gain_loss_base)}
              tone="teal"
            />
          </div>
          <Card className="border-cyan-300/10 bg-[radial-gradient(circle_at_top_left,rgba(34,211,238,0.045),transparent_38%),rgba(24,24,27,0.5)] p-5">
            <h2 className="portfolio-display text-lg font-semibold text-zinc-100">
              Archive points
            </h2>
            <div className="mt-4 space-y-2">
              {data.points.map((point) => (
                <div
                  key={`${point.captured_at}-${point.portfolio_index}`}
                  className="flex items-center justify-between rounded-lg border border-cyan-300/[0.07] bg-zinc-950/45 px-3 py-2 text-sm transition hover:bg-cyan-300/[0.025]"
                >
                  <span className="text-zinc-400">
                    {new Date(point.captured_at).toLocaleString()}
                  </span>
                  <span className="font-mono tabular-nums text-zinc-100">
                    {point.portfolio_index.toFixed(2)}
                  </span>
                  <span
                    className={cn(
                      "font-mono tabular-nums",
                      gainLossColor(point.gain_loss_percentage),
                    )}
                  >
                    {formatPercent(point.gain_loss_percentage)}
                  </span>
                </div>
              ))}
            </div>
          </Card>
          <Card className="border-violet-300/10 bg-[radial-gradient(circle_at_top_right,rgba(167,139,250,0.05),transparent_36%),rgba(24,24,27,0.5)] p-5">
            <h2 className="portfolio-display text-lg font-semibold text-zinc-100">
              Latest archived composition
            </h2>
            <p className="mt-1 text-xs text-zinc-500">
              Owner-private archive data may include quantities, prices, and
              realized results. It is not shown on public profiles.
            </p>
            <div className="mt-4 grid gap-4 lg:grid-cols-2">
              <ArchiveCompositionList
                title="Active positions"
                rows={latest?.positions ?? []}
              />
              <ArchiveCompositionList
                title="Closed positions"
                rows={latest?.closed_positions ?? []}
                closed
              />
            </div>
          </Card>
        </>
      )}
    </div>
  );
}

function ArchiveStat({
  label,
  value,
  className,
  tone,
}: {
  label: string;
  value: string;
  className?: string;
  tone: "cyan" | "violet" | "amber" | "teal";
}) {
  return (
    <Card
      className={cn(
        "p-4",
        tone === "cyan" && "border-cyan-300/10 bg-cyan-300/[0.025]",
        tone === "violet" && "border-violet-300/10 bg-violet-300/[0.025]",
        tone === "amber" && "border-amber-300/10 bg-amber-300/[0.025]",
        tone === "teal" && "border-teal-300/10 bg-teal-300/[0.025]",
      )}
    >
      <div className="text-[11px] uppercase tracking-wide text-zinc-500">
        {label}
      </div>
      <div className={cn("mt-2 font-mono text-xl tabular-nums", className)}>
        {value}
      </div>
    </Card>
  );
}

function ArchiveCompositionList({
  title,
  rows,
  closed,
}: {
  title: string;
  rows: Array<{ symbol: string; quantity: number; gain_loss_percentage?: number; realized_gain_loss_percentage?: number }>;
  closed?: boolean;
}) {
  return (
    <div className="rounded-xl border border-violet-300/[0.08] bg-zinc-950/35 p-4">
      <h3 className="portfolio-display text-base font-semibold text-zinc-100">{title}</h3>
      {rows.length === 0 ? (
        <p className="mt-3 text-sm text-zinc-500">No archived rows.</p>
      ) : (
        <div className="mt-3 space-y-2">
          {rows.map((row) => {
            const pct = closed
              ? row.realized_gain_loss_percentage
              : row.gain_loss_percentage;
            return (
              <div
                key={`${title}-${row.symbol}-${row.quantity}`}
                className="flex items-center justify-between gap-3 text-sm"
              >
                <span className="font-medium text-zinc-200">{row.symbol}</span>
                <span className="text-zinc-500 tabular-nums">
                  qty {row.quantity}
                </span>
                <span className={cn("font-mono tabular-nums", gainLossColor(pct))}>
                  {formatPercent(pct)}
                </span>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
