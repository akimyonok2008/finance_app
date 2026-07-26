import { useState } from "react";
import { Percent, TrendingDown, Wallet } from "lucide-react";
import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";

import { Card } from "@/components/ui/card";
import {
  usePerformanceHistory,
  usePortfolioValueHistory,
} from "@/hooks/usePerformance";
import { usePortfolioSummary } from "@/hooks/usePortfolioSummary";
import {
  PERFORMANCE_TIMEFRAMES,
  type PerformanceTimeframe,
} from "@/types/performance";
import { cn } from "@/utils/cn";
import { formatMoney } from "@/utils/formatMoney";
import { formatPercent } from "@/utils/formatPercent";
import { gainLossColor } from "@/utils/gainLoss";

const CHART_MODES = ["return", "value", "drawdown"] as const;
type ChartMode = (typeof CHART_MODES)[number];

const CHART_MODE_LABELS: Record<ChartMode, string> = {
  return: "Return",
  value: "Portfolio Value",
  drawdown: "Drawdown",
};

const EMPTY_HISTORY =
  "Performance history will appear after the first trusted snapshot.";

/**
 * Performance tab: "how did it perform".
 *
 * Every number here is computed by the backend from the canonical ranked
 * snapshot history. Nothing on this screen is recalculated in the browser, and
 * missing analytics render as "—" rather than as zero.
 */
export function PortfolioPerformanceTab() {
  const [timeframe, setTimeframe] = useState<PerformanceTimeframe>("1M");
  const [mode, setMode] = useState<ChartMode>("return");

  const history = usePerformanceHistory(timeframe);
  const valueHistory = usePortfolioValueHistory(timeframe);
  const { data: summary } = usePortfolioSummary();

  const ranked = history.data;
  const currency = summary?.base_currency ?? "USD";
  const economic = summary?.economic_performance;
  const economicPnl =
    economic && economic.is_complete && economic.total_pnl_base !== null
      ? economic.total_pnl_base
      : null;

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <OverviewCard
          label="Ranked Return"
          icon={Percent}
          hint={`Selected timeframe (${timeframe})`}
          value={
            ranked?.available && ranked.timeframe_return_percentage !== undefined
              ? formatPercent(ranked.timeframe_return_percentage)
              : "—"
          }
          valueClassName={gainLossColor(
            ranked?.timeframe_return_percentage ?? 0,
          )}
          unavailableReason={
            ranked?.available ? undefined : (ranked?.reason ?? EMPTY_HISTORY)
          }
        />
        <OverviewCard
          label="Economic P&L"
          icon={Wallet}
          hint="Ledger reconciliation, since inception"
          value={economicPnl === null ? "—" : formatMoney(economicPnl, currency)}
          valueClassName={gainLossColor(economicPnl ?? 0)}
          unavailableReason={
            economicPnl === null
              ? "Total portfolio P&L is unavailable: the ledger does not cover the full holding history."
              : undefined
          }
        />
        <OverviewCard
          label="Maximum Drawdown"
          icon={TrendingDown}
          hint="Worst peak-to-trough of the ranked index"
          value={
            ranked?.available && ranked.max_drawdown_percentage !== undefined
              ? formatPercent(ranked.max_drawdown_percentage)
              : "—"
          }
          valueClassName="text-rose-300"
          unavailableReason={
            ranked?.available ? undefined : (ranked?.reason ?? EMPTY_HISTORY)
          }
        />
      </div>

      <Card className="border-indigo-300/10 bg-indigo-300/[0.02] p-4 sm:p-5">
        <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
          <div className="inline-flex flex-wrap gap-1 rounded-xl border border-indigo-300/10 bg-zinc-900/45 p-1">
            {CHART_MODES.map((item) => (
              <button
                key={item}
                type="button"
                onClick={() => setMode(item)}
                aria-pressed={mode === item}
                className={cn(
                  "rounded-lg px-3 py-1.5 text-xs font-semibold transition",
                  mode === item
                    ? "bg-zinc-50 text-zinc-950"
                    : "text-zinc-500 hover:bg-zinc-800/70 hover:text-zinc-100",
                )}
              >
                {CHART_MODE_LABELS[item]}
              </button>
            ))}
          </div>

          <div className="inline-flex flex-wrap gap-1 rounded-xl border border-indigo-300/10 bg-zinc-900/45 p-1">
            {PERFORMANCE_TIMEFRAMES.map((item) => (
              <button
                key={item}
                type="button"
                onClick={() => setTimeframe(item)}
                aria-pressed={timeframe === item}
                className={cn(
                  "rounded-lg px-2.5 py-1.5 text-xs font-semibold tabular-nums transition",
                  timeframe === item
                    ? "bg-zinc-50 text-zinc-950"
                    : "text-zinc-500 hover:bg-zinc-800/70 hover:text-zinc-100",
                )}
              >
                {item}
              </button>
            ))}
          </div>
        </div>

        <PerformanceChart
          mode={mode}
          currency={currency}
          isLoading={
            mode === "value" ? valueHistory.isLoading : history.isLoading
          }
          isError={mode === "value" ? valueHistory.isError : history.isError}
          series={
            mode === "value"
              ? (valueHistory.data?.points ?? []).map((point) => ({
                  label: formatDay(point.captured_at),
                  value: point.total_value_base,
                }))
              : ranked?.available
                ? ranked.points.map((point) => ({
                    label: formatDay(point.captured_at),
                    value:
                      mode === "return"
                        ? point.return_percentage
                        : point.drawdown_percentage,
                  }))
                : []
          }
          emptyMessage={
            mode === "value"
              ? "Portfolio value history will appear after the first daily snapshot."
              : (ranked?.reason ?? EMPTY_HISTORY)
          }
        />

        <p className="mt-3 text-[11px] leading-5 text-zinc-500">
          {mode === "value"
            ? "Portfolio value includes deposits and withdrawals, so it is not a return measure."
            : "Return and drawdown come from the ranked index, which is neutral to deposits and withdrawals."}
        </p>
      </Card>
    </div>
  );
}

type OverviewCardProps = {
  label: string;
  value: string;
  hint: string;
  icon: typeof Percent;
  valueClassName?: string;
  unavailableReason?: string;
};

function OverviewCard({
  label,
  value,
  hint,
  icon: Icon,
  valueClassName,
  unavailableReason,
}: OverviewCardProps) {
  return (
    <Card className="border-indigo-300/10 bg-indigo-300/[0.02] p-5">
      <div className="flex items-center justify-between">
        <span className="text-[10px] font-semibold uppercase tracking-[0.16em] text-zinc-500">
          {label}
        </span>
        <span className="grid h-9 w-9 place-items-center rounded-xl border border-indigo-300/15 bg-zinc-950/35 text-indigo-200/80">
          <Icon className="h-4 w-4" />
        </span>
      </div>
      <div
        className={cn(
          "mt-4 font-mono text-2xl font-medium tabular-nums tracking-tight",
          unavailableReason ? "text-zinc-500" : valueClassName,
        )}
      >
        {value}
      </div>
      <p className="mt-1.5 text-[11px] leading-4 text-zinc-500">
        {unavailableReason ?? hint}
      </p>
    </Card>
  );
}

type ChartPoint = { label: string; value: number };

function PerformanceChart({
  mode,
  currency,
  series,
  isLoading,
  isError,
  emptyMessage,
}: {
  mode: ChartMode;
  currency: string;
  series: ChartPoint[];
  isLoading: boolean;
  isError: boolean;
  emptyMessage: string;
}) {
  const stroke =
    mode === "drawdown"
      ? "#fb7185"
      : mode === "value"
        ? "#a78bfa"
        : "#34d399";

  if (isLoading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <div className="h-5 w-5 animate-spin rounded-full border border-zinc-700 border-t-zinc-300" />
      </div>
    );
  }
  if (isError) {
    return (
      <div className="flex h-64 items-center justify-center text-sm text-rose-300">
        We could not load your performance history.
      </div>
    );
  }
  if (series.length === 0) {
    return (
      <div className="flex h-64 items-center justify-center px-6 text-center text-sm text-zinc-400">
        {emptyMessage}
      </div>
    );
  }

  const format = (value: number) =>
    mode === "value" ? formatMoney(value, currency) : formatPercent(value);

  return (
    <div className="h-64 min-w-0">
      <ResponsiveContainer width="100%" height="100%">
        <AreaChart data={series} margin={{ top: 8, right: 8, bottom: 0, left: 0 }}>
          <defs>
            <linearGradient id={`perf-${mode}`} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={stroke} stopOpacity={0.22} />
              <stop offset="100%" stopColor={stroke} stopOpacity={0.02} />
            </linearGradient>
          </defs>
          <CartesianGrid stroke="rgba(255,255,255,0.05)" vertical={false} />
          <XAxis
            dataKey="label"
            tick={{ fill: "#71717a", fontSize: 11 }}
            tickLine={false}
            axisLine={false}
            minTickGap={24}
          />
          <YAxis
            tick={{ fill: "#71717a", fontSize: 11 }}
            tickLine={false}
            axisLine={false}
            width={64}
            tickFormatter={format}
          />
          <Tooltip
            contentStyle={{
              background: "#18181b",
              border: "1px solid #27272a",
              borderRadius: 8,
              fontSize: 12,
            }}
            formatter={(value) => format(Number(value))}
          />
          <Area
            type="monotone"
            dataKey="value"
            stroke={stroke}
            strokeWidth={2}
            fill={`url(#perf-${mode})`}
            name={CHART_MODE_LABELS[mode]}
          />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  );
}

function formatDay(iso: string) {
  return new Date(iso).toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
  });
}
