import { useEffect, useMemo, useRef } from "react";
import { ArrowRight, PieChart, TrendingUp } from "lucide-react";
import { Link } from "react-router-dom";

import { AssetTypeBadge } from "@/components/portfolio/AssetTypeBadge";
import { AutomaticAdjustments } from "@/components/portfolio/AutomaticAdjustments";
import { AutomaticIncome } from "@/components/portfolio/AutomaticIncome";
import { CashBalancesCard } from "@/components/portfolio/CashBalancesCard";
import { PortfolioSummaryCards } from "@/components/portfolio/PortfolioSummaryCards";
import { PositionCardList } from "@/components/portfolio/PositionCardList";
import { PositionsTable } from "@/components/portfolio/PositionsTable";
import {
  episodeElementId,
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
  /**
   * `?episode=<id>` from the Transactions tab. Episode identity is the
   * `positions` row id, so it matches an open position's id or a closed
   * episode's id; the correct subview is resolved automatically.
   */
  episodeId?: string;
  onViewChange: (view: StateView) => void;
  /** Deep-links a position into the Transactions tab's sell flow. */
  onSell: (positionId: string) => void;
  onAddPosition: () => void;
};

/** Portfolio State tab: "what exists now" — positions, closed episodes, cash. */
export function PortfolioStateTab({
  view,
  episodeId,
  onViewChange,
  onSell,
  onAddPosition,
}: Props) {
  const { rows, isLoading, isError, error } = usePositionRows();
  const closed = useClosedPositions();
  const errorMessage = error?.message;

  // An episode link arrives without knowing whether the episode is still open.
  // Resolve it from the data instead of guessing, and switch subview once.
  const resolvedView = useRef<string | null>(null);
  useEffect(() => {
    if (!episodeId || resolvedView.current === episodeId) return;
    const isOpen = rows.some((row) => row.id === episodeId);
    const isClosed = (closed.data ?? []).some((row) => row.id === episodeId);
    if (!isOpen && !isClosed) return;
    resolvedView.current = episodeId;
    const target: StateView = isOpen ? "open" : "closed";
    if (view !== target) onViewChange(target);
  }, [episodeId, rows, closed.data, view, onViewChange]);

  return (
    <div>
      <PortfolioSummaryCards />
      <RankedReturnSummaryCard />

      <div
        role="group"
        aria-label="Portfolio view"
        className="mb-6 mt-6 inline-flex flex-wrap gap-1 rounded-xl border border-indigo-300/10 bg-zinc-900/45 p-1 shadow-lg shadow-black/10"
      >
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
          {episodeId && (
            <EpisodeSpotlight
              episodeId={episodeId}
              symbol={rows.find((row) => row.id === episodeId)?.symbol}
              found={rows.some((row) => row.id === episodeId)}
            />
          )}
          <PositionsTable
            rows={rows}
            highlightedId={episodeId}
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
        <ClosedPositionsView episodeId={episodeId} />
      ) : view === "allocation" ? (
        <AllocationView />
      ) : (
        <CashBalancesCard />
      )}
    </div>
  );
}

/**
 * Confirms which episode a `?episode=` deep link landed on, and focuses its
 * card. The link target is announced in text rather than signalled only by the
 * temporary highlight, so it works without sighted scanning.
 */
function EpisodeSpotlight({
  episodeId,
  symbol,
  found,
}: {
  episodeId: string;
  symbol?: string;
  found: boolean;
}) {
  useEffect(() => {
    if (!found) return;
    const element = document.getElementById(episodeElementId(episodeId));
    if (!element) return;
    // scrollIntoView is absent in some non-browser environments; focus alone is
    // the part that actually matters for keyboard and screen-reader users.
    element.scrollIntoView?.({ block: "center", behavior: "smooth" });
    element.focus({ preventScroll: true });
  }, [episodeId, found]);

  return (
    <p
      role="status"
      aria-live="polite"
      className="mb-3 rounded-lg border border-cyan-300/25 bg-cyan-300/[0.06] px-3 py-2 text-xs text-cyan-100/90"
    >
      {found
        ? `Showing the position episode this activity belongs to${symbol ? `: ${symbol}` : ""}.`
        : "That position episode is no longer in this list. It may have been closed or removed."}
    </p>
  );
}

/**
 * Allocation. Each holding's share of total holdings value, computed from the
 * position values the summary already returns — there is no new valuation here.
 * Cash is excluded and stated as such, because it is reported on its own tab.
 */
function AllocationView() {
  const { data, isLoading, isError } = usePortfolioSummary();

  const rows = useMemo(() => {
    const positions = data?.positions ?? [];
    const total = positions.reduce(
      (sum, position) => sum + (position.current_value_base ?? 0),
      0,
    );
    if (total <= 0) return [];
    return positions
      .map((position) => ({
        symbol: position.symbol,
        assetType: position.asset_type,
        value: position.current_value_base ?? 0,
        weight: ((position.current_value_base ?? 0) / total) * 100,
      }))
      .sort((a, b) => b.weight - a.weight);
  }, [data]);

  const currency = data?.base_currency ?? "USD";

  if (isLoading) {
    return (
      <Card
        role="status"
        aria-live="polite"
        className="border-cyan-300/10 bg-cyan-300/[0.025] p-6 text-sm text-zinc-400"
      >
        Loading allocation…
      </Card>
    );
  }
  if (isError) {
    return (
      <Card role="alert" className="p-6 text-sm text-rose-300">
        Could not load your allocation.
      </Card>
    );
  }
  if (rows.length === 0) {
    return (
      <Card className="border-violet-300/10 bg-violet-300/[0.025] p-8 text-center">
        <h2 className="portfolio-display text-xl font-semibold text-zinc-100">
          No allocation to show
        </h2>
        <p className="mt-2 text-sm text-zinc-400">
          Allocation appears once you hold at least one position with a current
          value.
        </p>
      </Card>
    );
  }

  return (
    <Card className="p-5">
      <div className="flex items-center gap-2">
        <PieChart className="h-4 w-4 text-zinc-400" />
        <h2 className="text-sm font-semibold text-zinc-100">
          Holdings allocation
        </h2>
      </div>
      <p className="mt-1 text-[11px] leading-4 text-zinc-500">
        Share of your holdings value. Cash is excluded — see the Cash view.
      </p>
      <ul className="mt-4 space-y-2">
        {rows.map((row) => (
          <li key={row.symbol}>
            <div className="flex items-center justify-between gap-3">
              <span className="flex items-center gap-2">
                <span className="font-mono text-sm text-zinc-100">
                  {row.symbol}
                </span>
                <AssetTypeBadge type={row.assetType} />
              </span>
              <span className="font-mono text-sm tabular-nums text-zinc-300">
                {row.weight.toFixed(1)}%
                <span className="ml-2 text-zinc-500">
                  {formatMoney(row.value, currency)}
                </span>
              </span>
            </div>
            <div
              className="mt-1 h-1.5 overflow-hidden rounded-full bg-zinc-800"
              role="presentation"
            >
              <div
                className="h-full rounded-full bg-gradient-to-r from-indigo-300 to-cyan-200"
                style={{ width: `${Math.min(100, row.weight)}%` }}
              />
            </div>
          </li>
        ))}
      </ul>
    </Card>
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

function ClosedPositionsView({ episodeId }: { episodeId?: string }) {
  const { data, isLoading, isError, error } = useClosedPositions();
  const rows = data ?? [];
  const highlighted = episodeId && rows.some((row) => row.id === episodeId);

  useEffect(() => {
    if (!highlighted || !episodeId) return;
    const element = document.getElementById(episodeElementId(episodeId));
    if (!element) return;
    // scrollIntoView is absent in some non-browser environments; focus alone is
    // the part that actually matters for keyboard and screen-reader users.
    element.scrollIntoView?.({ block: "center", behavior: "smooth" });
    element.focus({ preventScroll: true });
  }, [highlighted, episodeId]);

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
      {episodeId && !highlighted && (
        <p
          role="status"
          aria-live="polite"
          className="rounded-lg border border-cyan-300/25 bg-cyan-300/[0.06] px-3 py-2 text-xs text-cyan-100/90"
        >
          That position episode is not in your closed positions.
        </p>
      )}
      {rows.map((row) => (
        <ClosedPositionCard
          key={row.id}
          row={row}
          highlighted={row.id === episodeId}
        />
      ))}
    </div>
  );
}

function ClosedPositionCard({
  row,
  highlighted,
}: {
  row: ClosedPosition;
  highlighted?: boolean;
}) {
  const closedDate = row.closed_at
    ? new Date(row.closed_at).toLocaleDateString()
    : "—";
  return (
    <Card
      id={episodeElementId(row.id)}
      tabIndex={highlighted ? -1 : undefined}
      aria-current={highlighted ? "true" : undefined}
      className={cn(
        "border-amber-300/10 bg-[linear-gradient(110deg,rgba(251,191,36,0.035),rgba(24,24,27,0.5)_35%)] p-4 transition hover:border-amber-300/20",
        highlighted &&
          "border-cyan-300/50 ring-2 ring-cyan-300/30 focus-visible:outline-none",
      )}
    >
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
