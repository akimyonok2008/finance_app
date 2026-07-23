import { motion } from "framer-motion";
import {
  Activity,
  ArrowRight,
  Coins,
  TrendingDown,
  TrendingUp,
  Wallet,
} from "lucide-react";
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
      className="relative min-w-0 overflow-hidden rounded-3xl border border-indigo-300/15 bg-[radial-gradient(circle_at_50%_0%,rgba(129,140,248,0.13),transparent_34%),radial-gradient(circle_at_15%_75%,rgba(34,211,238,0.055),transparent_28%),rgba(24,24,27,0.48)] px-4 pb-4 pt-5 shadow-2xl shadow-black/25 sm:px-7 sm:pb-6 sm:pt-6"
      whileHover={{ y: -2 }}
      transition={{ duration: 0.18 }}
    >
      <div className="flex items-center justify-between gap-4">
        <p className="text-[10px] font-semibold uppercase tracking-[0.2em] text-indigo-100/55">
          Portfolio performance
        </p>
        <Link
          to="/portfolio"
          className="group inline-flex items-center gap-1 text-xs font-medium text-zinc-500 transition hover:text-zinc-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-indigo-300/35"
        >
          View portfolio
          <ArrowRight className="h-3.5 w-3.5 transition group-hover:translate-x-0.5" />
        </Link>
      </div>

      <div className="mt-5 text-center sm:mt-6">
        <p className="text-xs font-medium text-zinc-500">Portfolio Index</p>
        <div className={cn("mt-1.5 font-mono text-4xl font-medium tabular-nums tracking-[-0.04em] sm:text-5xl", palette.text)}>
          {index.toFixed(2)}
        </div>
        <div className="mt-3 flex flex-wrap items-center justify-center gap-3">
          <span className="text-xs text-zinc-500">Baseline 100.00</span>
          <span className="h-1 w-1 rounded-full bg-zinc-700" />
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
      </div>

      <div className="mt-5 grid grid-cols-2 gap-2 sm:grid-cols-4">
        <DashboardMetricCard
          label="Value"
          value={formatMoney(summary?.current_value, currency)}
          icon={<Wallet className="h-3.5 w-3.5" />}
          className="border-cyan-300/10 bg-cyan-300/[0.035]"
        />
        <DashboardMetricCard
          label="Cost basis"
          value={formatMoney(summary?.total_cost_basis, currency)}
          icon={<Coins className="h-3.5 w-3.5" />}
          className="border-violet-300/10 bg-violet-300/[0.035]"
        />
        <DashboardMetricCard
          label="Gain / Loss"
          value={formatMoney(summary?.gain_loss, currency)}
          tone={tone === "neutral" ? "default" : tone}
          className={cn(
            tone === "positive" && "border-emerald-300/10 bg-emerald-300/[0.035]",
            tone === "negative" && "border-rose-300/10 bg-rose-300/[0.035]",
          )}
        />
        <DashboardMetricCard
          label="Gain %"
          value={formatPercent(gainPct)}
          tone={tone === "neutral" ? "default" : tone}
          className="border-amber-300/10 bg-amber-300/[0.035]"
        />
      </div>

      <div className="mt-4 h-44 min-h-44 min-w-0 sm:h-52 sm:min-h-52">
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
          <ResponsiveContainer
            width="100%"
            height="100%"
            minWidth={1}
            minHeight={1}
          >
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
                  stroke="rgba(255,255,255,0.045)"
                  vertical={false}
                />
                <XAxis
                  dataKey="label"
                  tick={{ fill: "#71717a", fontSize: 10 }}
                  axisLine={false}
                  tickLine={false}
                />
                <YAxis
                  tick={{ fill: "#71717a", fontSize: 10 }}
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
                  strokeWidth={2.25}
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
