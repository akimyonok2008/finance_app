import { motion } from "framer-motion";
import { Activity, Coins, TrendingDown, TrendingUp, Wallet } from "lucide-react";
import { Link } from "react-router-dom";
import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";

import { DashboardMetricCard } from "@/pages/Dashboard/components/DashboardMetricCard";
import { buildPrototypeIndexSeries } from "@/pages/Dashboard/utils/buildPrototypeIndexSeries";
import { getPerformanceTone } from "@/pages/Dashboard/utils/dashboardFormatters";
import type { DashboardPortfolioSummary } from "@/types/dashboard";
import { formatMoney } from "@/utils/formatMoney";
import { formatPercent } from "@/utils/formatPercent";
import { cn } from "@/utils/cn";

const PALETTE = {
  positive: {
    stroke: "#34d399",
    fillStart: "rgba(52,211,153,0.12)",
    fillEnd: "rgba(52,211,153,0.02)",
    text: "text-emerald-400",
    bg: "bg-emerald-400/10",
    border: "border-emerald-400/20",
  },
  negative: {
    stroke: "#fb7185",
    fillStart: "rgba(251,113,133,0.10)",
    fillEnd: "rgba(251,113,133,0.02)",
    text: "text-rose-400",
    bg: "bg-rose-400/10",
    border: "border-rose-400/20",
  },
  neutral: {
    stroke: "#a1a1aa",
    fillStart: "rgba(161,161,170,0.10)",
    fillEnd: "rgba(161,161,170,0.01)",
    text: "text-zinc-300",
    bg: "bg-zinc-800/50",
    border: "border-zinc-700",
  },
};

type Props = {
  summary: DashboardPortfolioSummary | null;
  isLoading: boolean;
  isError: boolean;
  onRetry?: () => void;
};

function CustomTooltip({
  active,
  payload,
  label,
}: {
  active?: boolean;
  payload?: { value: number }[];
  label?: string;
}) {
  if (!active || !payload?.length) return null;
  return (
    <div className="rounded-lg border border-zinc-800 bg-zinc-900 px-3 py-2 text-xs">
      <div className="text-slate-400">{label}</div>
      <div className="tabular-nums font-semibold text-slate-50">
        {payload[0].value.toFixed(2)}
      </div>
    </div>
  );
}

export function PerformanceChartCard({
  summary,
  isLoading,
  isError,
  onRetry,
}: Props) {
  const index = summary?.portfolio_index ?? 100;
  const gainPct = summary?.gain_loss_percentage ?? 0;
  const tone = getPerformanceTone(gainPct);
  const palette = PALETTE[tone];
  const series = buildPrototypeIndexSeries(index);
  const currency = summary?.base_currency ?? "USD";
  const isEmpty = !summary || summary.current_value === 0;

  const TrendIcon =
    tone === "positive" ? TrendingUp : tone === "negative" ? TrendingDown : Activity;

  return (
    <motion.div
      className="min-w-0 rounded-2xl border border-zinc-800 bg-zinc-900/50 p-5 shadow-sm shadow-black/20 transition-colors hover:border-zinc-700 hover:bg-zinc-900/70"
      whileHover={{ y: -2 }}
      transition={{ duration: 0.18 }}
    >
      {/* Header */}
      <div className="mb-4 flex items-start justify-between gap-4">
        <div>
          <div className="text-xs font-medium text-zinc-500">
            Portfolio Index
          </div>
          <div className="mt-1 text-xs text-slate-500">
            Illustrative index path
          </div>
        </div>
        <div
          className={cn(
            "flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs font-medium tabular-nums",
            palette.bg,
            palette.border,
            palette.text,
          )}
        >
          <TrendIcon className="h-3.5 w-3.5" />
          {formatPercent(gainPct)}
        </div>
      </div>

      {/* Primary metric */}
      <div className={cn("font-mono text-4xl font-medium tabular-nums tracking-tight", palette.text)}>
        {index.toFixed(2)}
      </div>
      <div className="mt-1 text-sm text-slate-400">Index · baseline 100.00</div>

      {/* Metric chips */}
      <div className="mt-4 grid grid-cols-2 gap-2 sm:grid-cols-4">
        <DashboardMetricCard
          label="Value"
          value={formatMoney(summary?.current_value, currency)}
          icon={<Wallet className="h-3.5 w-3.5" />}
        />
        <DashboardMetricCard
          label="Cost basis"
          value={formatMoney(summary?.total_cost_basis, currency)}
          icon={<Coins className="h-3.5 w-3.5" />}
        />
        <DashboardMetricCard
          label="Gain / Loss"
          value={formatMoney(summary?.gain_loss, currency)}
          tone={tone === "neutral" ? "default" : tone}
        />
        <DashboardMetricCard
          label="Gain %"
          value={formatPercent(gainPct)}
          tone={tone === "neutral" ? "default" : tone}
        />
      </div>

      {/* Chart */}
      <div className="mt-5 h-52 min-w-0">
        {isError ? (
          <div className="flex h-full flex-col items-center justify-center gap-2 text-sm text-slate-500">
            <span>We could not load your portfolio summary.</span>
            {onRetry && (
              <button
                onClick={onRetry}
                className="text-xs text-indigo-400 underline underline-offset-2 hover:text-indigo-300 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-indigo-400/40"
              >
                Retry
              </button>
            )}
          </div>
        ) : isEmpty && !isLoading ? (
          <div className="flex h-full flex-col items-center justify-center gap-3 text-center">
            <p className="text-sm text-slate-400">No positions yet</p>
            <Link
              to="/portfolio"
              className="text-xs font-medium text-indigo-400 underline underline-offset-2 hover:text-indigo-300"
            >
              Add your first position
            </Link>
          </div>
        ) : (
          <ResponsiveContainer width="100%" height="100%">
              <AreaChart
                data={series}
                margin={{ top: 4, right: 4, left: -20, bottom: 0 }}
              >
                <defs>
                  <linearGradient id="chartGrad" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="5%" stopColor={palette.fillStart} />
                    <stop offset="95%" stopColor={palette.fillEnd} />
                  </linearGradient>
                </defs>
                <CartesianGrid
                  strokeDasharray="3 3"
                  stroke="rgba(255,255,255,0.05)"
                  vertical={false}
                />
                <XAxis
                  dataKey="label"
                  tick={{ fill: "#64748b", fontSize: 11 }}
                  axisLine={false}
                  tickLine={false}
                />
                <YAxis
                  tick={{ fill: "#64748b", fontSize: 11 }}
                  axisLine={false}
                  tickLine={false}
                  domain={["auto", "auto"]}
                  tickFormatter={(v: number) => v.toFixed(0)}
                />
                <Tooltip content={<CustomTooltip />} />
                <Area
                  type="monotone"
                  dataKey="index"
                  stroke={palette.stroke}
                  strokeWidth={2}
                  fill="url(#chartGrad)"
                  dot={false}
                  activeDot={{
                    r: 4,
                    fill: palette.stroke,
                    strokeWidth: 0,
                  }}
                />
              </AreaChart>
            </ResponsiveContainer>
        )}
      </div>
    </motion.div>
  );
}
