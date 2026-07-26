import { ArrowRight, TrendingUp } from "lucide-react";
import { Link } from "react-router-dom";

import { AssetTypeBadge } from "@/components/portfolio/AssetTypeBadge";
import { AutomaticAdjustments } from "@/components/portfolio/AutomaticAdjustments";
import { AutomaticIncome } from "@/components/portfolio/AutomaticIncome";
import { CashBalancesCard } from "@/components/portfolio/CashBalancesCard";
import { PortfolioSummaryCards } from "@/components/portfolio/PortfolioSummaryCards";
import { PositionCardList } from "@/components/portfolio/PositionCardList";
import { PositionsTable } from "@/components/portfolio/PositionsTable";
import {
  STATE_VIEW_LABELS,
  STATE_VIEWS,
  type StateView,
} from "@/components/portfolio/portfolioTabs";
import { Card } from "@/components/ui/card";
import { usePortfolioSummary } from "@/hooks/usePortfolioSummary";
import { usePositionRows } from "@/hooks/usePositionRows";
import { useClosedPositions } from "@/hooks/usePositions";
import type { ClosedPosition } from "@/types/portfolio";
import { cn } from "@/utils/cn";
import { formatMoney } from "@/utils/formatMoney";
import { formatPercent } from "@/utils/formatPercent";
import { gainLossColor } from "@/utils/gainLoss";

type Props = {
  view: StateView;
  onViewChange: (view: StateView) => void;
  /** Deep-links a position into the Transactions tab's sell flow. */
  onSell: (positionId: string) => void;
  onAddPosition: () => void;
};

/** Portfolio State tab: "what exists now" — positions, closed episodes, cash. */
export function PortfolioStateTab({
  view,
  onViewChange,
  onSell,
  onAddPosition,
}: Props) {
  const { rows, isLoading, isError, error } = usePositionRows();
  const errorMessage = error?.message;

  return (
    <div>
      <PortfolioSummaryCards />
      <RankedReturnSummaryCard />

      <div className="mb-6 mt-6 inline-flex flex-wrap gap-1 rounded-xl border border-indigo-300/10 bg-zinc-900/45 p-1 shadow-lg shadow-black/10">
        {STATE_VIEWS.map((item) => (
          <button
            key={item}
            type="button"
            onClick={() => onViewChange(item)}
            aria-pressed={view === item}
            className={cn(
              "rounded-lg px-3.5 py-2 text-xs font-semibold tracking-wide transition",
              view === item
                ? "bg-gradient-to-r from-indigo-200 to-cyan-100 text-zinc-950 shadow-sm shadow-cyan-950/20"
                : "text-zinc-500 hover:bg-zinc-800/70 hover:text-zinc-100",
            )}
          >
            {STATE_VIEW_LABELS[item]}
          </button>
        ))}
      </div>

      {view === "open" ? (
        <div>
          <PositionsTable
            rows={rows}
            isLoading={isLoading}
            isError={isError}
            errorMessage={errorMessage}
            onClose={(row) => onSell(row.id)}
          />
          <PositionCardList
            rows={rows}
            isLoading={isLoading}
            isError={isError}
            errorMessage={errorMessage}
            onClose={(row) => onSell(row.id)}
            onAdd={onAddPosition}
          />
          <AutomaticIncome />
          <AutomaticAdjustments />
        </div>
      ) : view === "closed" ? (
        <ClosedPositionsView />
      ) : (
        <CashBalancesCard />
      )}
    </div>
  );
}

/**
 * Compact ranked-return summary with a link into the Performance tab. The value
 * is the backend's ranked performance; nothing is recomputed here.
 */
function RankedReturnSummaryCard() {
  const { data } = usePortfolioSummary();
  const ranked = data?.ranked_performance;
  const unavailable = !ranked || ranked.tracking_status === "unavailable";

  return (
    <Card className="mt-4 flex flex-wrap items-center justify-between gap-4 border-emerald-300/10 bg-emerald-300/[0.02] p-4">
      <div className="flex items-center gap-3">
        <span className="grid h-9 w-9 place-items-center rounded-xl border border-emerald-300/15 bg-zinc-950/35 text-emerald-300/80">
          <TrendingUp className="h-4 w-4" />
        </span>
        <div>
          <p className="text-[10px] font-semibold uppercase tracking-[0.16em] text-zinc-500">
            Ranked Return
          </p>
          <p
            className={cn(
              "font-mono text-xl font-medium tabular-nums",
              unavailable
                ? "text-zinc-500"
                : gainLossColor(ranked.return_percentage),
            )}
          >
            {unavailable ? "—" : formatPercent(ranked.return_percentage)}
          </p>
          <p className="mt-0.5 text-[11px] text-zinc-500">
            {unavailable
              ? "Ranked tracking has not started yet."
              : `Index ${ranked.index.toFixed(2)} · ${ranked.tracking_status}`}
          </p>
        </div>
      </div>
      <Link
        to="/portfolio?tab=performance"
        className="group inline-flex items-center gap-1 rounded-lg px-2 py-1 text-xs font-medium text-emerald-300/90 transition hover:text-emerald-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-emerald-300/40"
      >
        View performance
        <ArrowRight className="h-3.5 w-3.5 transition group-hover:translate-x-0.5" />
      </Link>
    </Card>
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
        <h2 className="portfolio-display text-xl font-semibold text-zinc-100">
          No closed positions
        </h2>
        <p className="mt-2 text-sm text-zinc-400">
          Closed positions will appear here after you mark an asset as sold. No
          trade is placed.
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
      <div
        className={cn(
          "mt-1 font-mono text-sm tabular-nums text-zinc-200",
          className,
        )}
      >
        {value}
      </div>
    </div>
  );
}
