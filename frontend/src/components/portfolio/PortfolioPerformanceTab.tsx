import { useState } from "react";
import {
  ArrowUpRight,
  Award,
  Percent,
  Scale,
  TrendingDown,
  Trophy,
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

import { Card } from "@/components/ui/card";
import { useLeaderboardStanding } from "@/hooks/useLeaderboardStanding";
import {
  usePerformanceHistory,
  usePerformanceSummary,
  usePortfolioValueHistory,
} from "@/hooks/usePerformance";
import { usePortfolioSummary } from "@/hooks/usePortfolioSummary";
import type { LeaderboardTimeframe } from "@/types/leaderboard";
import {
  PERFORMANCE_TIMEFRAMES,
  type BenchmarkComparison,
  type ContributionAnalysis,
  type EconomicBreakdown,
  type InstrumentContribution,
  type PerformanceTimeframe,
  type RiskConsistency,
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
export function PortfolioPerformanceTab({
  timeframe = "1M",
  onTimeframeChange,
}: {
  timeframe?: PerformanceTimeframe;
  onTimeframeChange?: (timeframe: PerformanceTimeframe) => void;
}) {
  const [mode, setMode] = useState<ChartMode>("return");
  const setTimeframe = onTimeframeChange ?? (() => undefined);

  const history = usePerformanceHistory(timeframe);
  const valueHistory = usePortfolioValueHistory(timeframe);
  const { data: summary } = usePortfolioSummary();
  const performance = usePerformanceSummary();
  // The leaderboard's timeframes are the SAME set as the chart's, so the
  // competitive standing shown here is measured over the selected period and
  // not silently over all time.
  const standing = useLeaderboardStanding(timeframe as LeaderboardTimeframe);

  const ranked = history.data;
  const currency = performance.data?.base_currency ?? summary?.base_currency ?? "USD";
  // One source for the headline total and the breakdown below it: the
  // performance layer's own DTO. Reading it from two places invites drift.
  const economicPnl =
    performance.data?.economic_breakdown.total_economic_pnl_base ?? null;

  return (
    <div className="space-y-6">
      <div>
        <p className="text-[10px] font-semibold uppercase tracking-[0.2em] text-cyan-200/60">
          Selected period
        </p>
        <p className="mt-1 text-xs text-zinc-500">
          Ranked performance and competition metrics for {timeframe}.
        </p>
      </div>

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
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
          <div
            role="group"
            aria-label="Chart mode"
            className="inline-flex flex-wrap gap-1 rounded-xl border border-indigo-300/10 bg-zinc-900/45 p-1"
          >
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

          <div
            role="group"
            aria-label="Timeframe"
            className="inline-flex flex-wrap gap-1 rounded-xl border border-indigo-300/10 bg-zinc-900/45 p-1"
          >
            {PERFORMANCE_TIMEFRAMES.map((item) => (
              <button
                key={item}
                type="button"
                onClick={() => setTimeframe(item)}
                aria-pressed={timeframe === item}
                aria-label={`Timeframe ${item}`}
                className={cn(
                  "rounded-lg px-2.5 py-1.5 text-xs font-semibold tabular-nums transition",
                  timeframe === item
                    ? "bg-zinc-50 text-zinc-950"
                    : "text-zinc-500 hover:bg-zinc-800/70 hover:text-zinc-100",
                )}
              >
                {item === "ALL" ? "All" : item}
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

      <RiskConsistencySection risk={ranked?.risk} isLoading={history.isLoading} />

      <BenchmarkAndCompetitionSection
        benchmark={ranked?.benchmark}
        timeframe={timeframe}
        rank={standing.data?.rank ?? null}
        percentile={standing.data?.percentile}
        participants={standing.data?.participant_count}
        standingReason={standing.data?.reason}
        isLoading={history.isLoading || standing.isLoading}
      />

      <div className="border-t border-indigo-300/10 pt-2">
        <p className="text-[10px] font-semibold uppercase tracking-[0.2em] text-violet-200/60">
          Since inception
        </p>
        <p className="mt-1 text-xs text-zinc-500">
          Ledger economics and attribution across the portfolio's full history.
        </p>
      </div>

      <OverviewCard
        label="Economic P&L"
        icon={Wallet}
        hint="Since inception"
        value={economicPnl === null ? "—" : signedMoney(economicPnl, currency)}
        valueClassName={gainLossColor(economicPnl ?? 0)}
        unavailableReason={
          economicPnl === null
            ? "Total portfolio P&L is unavailable: the ledger does not cover the full holding history."
            : undefined
        }
      />

      <EconomicBreakdownSection
        breakdown={performance.data?.economic_breakdown}
        currency={currency}
        isLoading={performance.isLoading}
        isError={performance.isError}
      />

      <ContributionSection
        contributions={performance.data?.contributions}
        currency={currency}
        timeframe={timeframe}
        isLoading={performance.isLoading}
      />
    </div>
  );
}

/** Shared shell so every analytic section looks and announces the same way. */
function Section({
  title,
  icon: Icon,
  subtitle,
  children,
}: {
  title: string;
  icon: typeof Percent;
  subtitle?: string;
  children: React.ReactNode;
}) {
  return (
    <Card
      role="region"
      aria-label={title}
      className="border-indigo-300/10 bg-indigo-300/[0.02] p-4 sm:p-5"
    >
      <div className="flex items-start gap-3">
        <span className="grid h-9 w-9 shrink-0 place-items-center rounded-xl border border-indigo-300/15 bg-zinc-950/35 text-indigo-200/80">
          <Icon className="h-4 w-4" />
        </span>
        <div className="min-w-0">
          <h2 className="text-sm font-semibold tracking-tight text-zinc-100">
            {title}
          </h2>
          {subtitle && (
            <p className="mt-0.5 text-[11px] leading-4 text-zinc-500">
              {subtitle}
            </p>
          )}
        </div>
      </div>
      <div className="mt-4">{children}</div>
    </Card>
  );
}

function Unavailable({ children }: { children: React.ReactNode }) {
  return (
    <p className="rounded-lg border border-zinc-800 bg-zinc-900/40 px-3 py-2.5 text-xs leading-5 text-zinc-400">
      {children}
    </p>
  );
}

/**
 * Economic breakdown — the SAME decomposition the backend's reconciliation
 * verifies (realized + unrealized + income - standalone fees). Nothing here is
 * summed in the browser, and every row deep-links to the activities that
 * produced it.
 */
function EconomicBreakdownSection({
  breakdown,
  currency,
  isLoading,
  isError,
}: {
  breakdown: EconomicBreakdown | undefined;
  currency: string;
  isLoading: boolean;
  isError: boolean;
}) {
  const rows: Array<{
    label: string;
    value: number;
    href: string;
    hint: string;
  }> = breakdown
    ? [
        {
          label: "Realized P&L",
          value: breakdown.realized_pnl_base,
          href: "/portfolio?tab=transactions&category=trades",
          hint: "Closed and partially-sold positions",
        },
        {
          label: "Unrealized P&L",
          value: breakdown.unrealized_pnl_base,
          href: "/portfolio?tab=state&view=open",
          hint: "Open holdings at current prices",
        },
        {
          label: "Net Income",
          value: breakdown.net_income_base,
          href: "/portfolio?tab=transactions&category=income",
          hint: "Dividends, distributions, interest",
        },
        {
          label: "Standalone Fees",
          value: -breakdown.standalone_fees_base,
          href: "/portfolio?tab=transactions&category=fees",
          hint: "Management and custody fees; trade fees are already inside the figures above",
        },
      ]
    : [];

  return (
    <Section
      title="Economic breakdown"
      icon={Wallet}
      subtitle="Ledger accounting in your base currency. Ranked return is never converted into money."
    >
      <div aria-live="polite" aria-busy={isLoading}>
        {isLoading && <Unavailable>Loading your economic breakdown…</Unavailable>}
        {isError && (
          <Unavailable>We could not load your economic breakdown.</Unavailable>
        )}
        {!isLoading && !isError && breakdown && (
          <>
            <ul className="divide-y divide-zinc-800/70">
              {rows.map((row) => (
                <li key={row.label}>
                  <Link
                    to={row.href}
                    className="group flex items-center justify-between gap-4 rounded-lg px-1 py-2.5 transition hover:bg-zinc-800/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-300/40"
                  >
                    <span className="min-w-0">
                      <span className="flex items-center gap-1 text-sm text-zinc-200">
                        {row.label}
                        <ArrowUpRight className="h-3 w-3 text-zinc-600 transition group-hover:text-cyan-300" />
                      </span>
                      <span className="block text-[11px] leading-4 text-zinc-500">
                        {row.hint}
                      </span>
                    </span>
                    <span
                      className={cn(
                        "shrink-0 font-mono text-sm tabular-nums",
                        gainLossColor(row.value),
                      )}
                    >
                      {signedMoney(row.value, currency)}
                    </span>
                  </Link>
                </li>
              ))}
            </ul>

            <div className="mt-3 flex items-center justify-between gap-4 rounded-lg border border-indigo-300/15 bg-zinc-950/40 px-3 py-3">
              <span className="text-xs font-semibold uppercase tracking-[0.14em] text-zinc-400">
                Total Economic P&amp;L
              </span>
              <span
                className={cn(
                  "font-mono text-base tabular-nums",
                  breakdown.total_economic_pnl_base === null
                    ? "text-zinc-500"
                    : gainLossColor(breakdown.total_economic_pnl_base),
                )}
              >
                {breakdown.total_economic_pnl_base === null
                  ? "—"
                  : signedMoney(breakdown.total_economic_pnl_base, currency)}
              </span>
            </div>

            {breakdown.total_economic_pnl_base === null && (
              <p className="mt-2 text-[11px] leading-4 text-amber-200/80">
                The ledger does not cover your full holding history
                {breakdown.calculation_status
                  ? ` (${breakdown.calculation_status.replace(/_/g, " ")})`
                  : ""}
                , so a total cannot be stated. The components above are still
                exact.
              </p>
            )}
            {breakdown.unattributed_base !== null &&
              breakdown.unattributed_base !== 0 && (
                <p className="mt-2 text-[11px] leading-4 text-amber-200/80">
                  {signedMoney(breakdown.unattributed_base, currency)} of the
                  total could not be traced to the components above and is shown
                  rather than hidden.
                </p>
              )}
          </>
        )}
      </div>
    </Section>
  );
}

/** Risk & Consistency. All four metrics come from the ranked index. */
function RiskConsistencySection({
  risk,
  isLoading,
}: {
  risk: RiskConsistency | undefined;
  isLoading: boolean;
}) {
  return (
    <Section
      title="Risk & consistency"
      icon={TrendingDown}
      subtitle="Derived from the ranked index, which is neutral to deposits and withdrawals."
    >
      <div
        aria-live="polite"
        aria-busy={isLoading}
        className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4"
      >
        {isLoading && <Unavailable>Loading risk statistics…</Unavailable>}
        {!isLoading && !risk && (
          <Unavailable>{EMPTY_HISTORY}</Unavailable>
        )}
        {!isLoading && risk && (
          <>
            <MetricTile
              label="Maximum drawdown"
              value={
                risk.max_drawdown_percentage === null
                  ? null
                  : formatPercent(risk.max_drawdown_percentage)
              }
              tone="text-rose-300"
              unavailable={risk.drawdown_reason ?? EMPTY_HISTORY}
            />
            <MetricTile
              label="Current drawdown"
              value={
                risk.current_drawdown_percentage === null
                  ? null
                  : formatPercent(risk.current_drawdown_percentage)
              }
              tone={
                risk.current_drawdown_percentage === 0
                  ? "text-emerald-400"
                  : "text-rose-300"
              }
              hint={
                risk.current_drawdown_percentage === 0
                  ? "At a new high"
                  : "Below the running peak"
              }
              unavailable={risk.drawdown_reason ?? EMPTY_HISTORY}
            />
            <MetricTile
              label="Positive weeks"
              value={
                risk.positive_weeks_percentage === null
                  ? null
                  : `${risk.positive_weeks_percentage.toFixed(0)}%`
              }
              tone="text-zinc-100"
              hint={`${risk.positive_weeks} of ${risk.complete_weeks} complete weeks`}
              unavailable={risk.weeks_reason ?? EMPTY_HISTORY}
            />
            <MetricTile
              label="Best / worst month"
              value={
                risk.best_month && risk.worst_month
                  ? `${formatPercent(risk.best_month.return_percentage)} / ${formatPercent(risk.worst_month.return_percentage)}`
                  : null
              }
              tone="text-zinc-100"
              hint={
                risk.best_month && risk.worst_month
                  ? `${risk.best_month.label} / ${risk.worst_month.label} · ${risk.complete_months} complete months`
                  : undefined
              }
              unavailable={risk.months_reason ?? EMPTY_HISTORY}
            />
          </>
        )}
      </div>
    </Section>
  );
}

function MetricTile({
  label,
  value,
  tone,
  hint,
  unavailable,
}: {
  label: string;
  value: string | null;
  tone: string;
  hint?: string;
  unavailable: string;
}) {
  return (
    <div className="rounded-xl border border-zinc-800 bg-zinc-950/35 p-3">
      <div className="text-[10px] font-semibold uppercase tracking-[0.14em] text-zinc-500">
        {label}
      </div>
      <div
        className={cn(
          "mt-2 font-mono text-lg tabular-nums",
          value === null ? "text-zinc-500" : tone,
        )}
      >
        {value ?? "—"}
      </div>
      <p className="mt-1 text-[11px] leading-4 text-zinc-500">
        {value === null ? unavailable : hint}
      </p>
    </div>
  );
}

/**
 * Benchmark & competition.
 *
 * The benchmark difference is only rendered when the backend confirms BOTH
 * returns were measured between the same boundary dates; otherwise the reason
 * is shown. The rank comes from the leaderboard over the same timeframe.
 */
function BenchmarkAndCompetitionSection({
  benchmark,
  timeframe,
  rank,
  percentile,
  participants,
  standingReason,
  isLoading,
}: {
  benchmark: BenchmarkComparison | undefined;
  timeframe: PerformanceTimeframe;
  rank: number | null;
  percentile: number | undefined;
  participants: number | undefined;
  standingReason: string | undefined;
  isLoading: boolean;
}) {
  return (
    <Section
      title="Benchmark & competition"
      icon={Trophy}
      subtitle={`Both figures cover the selected timeframe (${timeframe}).`}
    >
      <div
        aria-live="polite"
        aria-busy={isLoading}
        className="grid grid-cols-1 gap-3 sm:grid-cols-2"
      >
        <div className="rounded-xl border border-zinc-800 bg-zinc-950/35 p-3">
          <div className="flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-[0.14em] text-zinc-500">
            <Scale className="h-3 w-3" />
            Benchmark difference
          </div>
          {benchmark?.available &&
          benchmark.difference_percentage_points !== undefined ? (
            <>
              <div
                className={cn(
                  "mt-2 font-mono text-lg tabular-nums",
                  gainLossColor(benchmark.difference_percentage_points),
                )}
              >
                {formatPercent(benchmark.difference_percentage_points)}
              </div>
              <p className="mt-1 text-[11px] leading-4 text-zinc-500">
                vs {benchmark.name || benchmark.recipe_id} (
                {formatPercent(benchmark.benchmark_return_percentage)}) over{" "}
                {benchmark.aligned_from} → {benchmark.aligned_to}
                {benchmark.is_synthetic ? " · simulated prices" : ""}
              </p>
            </>
          ) : (
            <>
              <div className="mt-2 font-mono text-lg tabular-nums text-zinc-500">
                —
              </div>
              <p className="mt-1 text-[11px] leading-4 text-zinc-500">
                {benchmark?.reason ?? EMPTY_HISTORY}
              </p>
            </>
          )}
        </div>

        <div className="rounded-xl border border-zinc-800 bg-zinc-950/35 p-3">
          <div className="flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-[0.14em] text-zinc-500">
            <Award className="h-3 w-3" />
            Global rank
          </div>
          {rank !== null ? (
            <>
              <div className="mt-2 font-mono text-lg tabular-nums text-zinc-100">
                #{rank}
                {participants ? (
                  <span className="text-zinc-500"> / {participants}</span>
                ) : null}
              </div>
              <p className="mt-1 text-[11px] leading-4 text-zinc-500">
                {percentile !== undefined
                  ? `Top ${(100 - percentile).toFixed(0)}% over ${timeframe}`
                  : `Over ${timeframe}`}
              </p>
            </>
          ) : (
            <>
              <div className="mt-2 font-mono text-lg tabular-nums text-zinc-500">
                —
              </div>
              <p className="mt-1 text-[11px] leading-4 text-zinc-500">
                {standingReason ??
                  "You are not ranked over this timeframe yet."}
              </p>
            </>
          )}
        </div>
      </div>
    </Section>
  );
}

/**
 * Contributors & detractors.
 *
 * The backend ranks by weight x instrument return (contribution in percentage
 * points), never by standalone instrument return. Its scope is SINCE INCEPTION
 * because no per-instrument daily valuation history exists; that limitation is
 * stated on screen instead of being papered over with the selected timeframe.
 */
function ContributionSection({
  contributions,
  currency,
  timeframe,
  isLoading,
}: {
  contributions: ContributionAnalysis | undefined;
  currency: string;
  timeframe: PerformanceTimeframe;
  isLoading: boolean;
}) {
  const sinceInception = contributions?.basis === "since_inception";
  return (
    <Section
      title="Contributors & detractors"
      icon={Percent}
      subtitle="Ranked by contribution to portfolio return (weight × return), not by standalone instrument return."
    >
      <div aria-live="polite" aria-busy={isLoading}>
        {isLoading && <Unavailable>Loading contribution analysis…</Unavailable>}

        {!isLoading && contributions && !contributions.available && (
          <Unavailable>
            {contributions.reason ??
              "Contribution analysis is not available yet."}
          </Unavailable>
        )}

        {!isLoading && contributions?.available && (
          <>
            {sinceInception && timeframe !== "ALL" && (
              <p className="mb-3 rounded-lg border border-amber-300/20 bg-amber-300/[0.06] px-3 py-2 text-[11px] leading-4 text-amber-100/85">
                These figures cover your whole history, not the selected{" "}
                {timeframe} window. Period attribution needs per-instrument daily
                valuations, which are not recorded yet.
              </p>
            )}

            <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
              <ContributionList
                heading="Top contributors"
                rows={contributions.contributors}
                currency={currency}
                emptyMessage="No instrument added to your return yet."
              />
              <ContributionList
                heading="Top detractors"
                rows={contributions.detractors}
                currency={currency}
                emptyMessage="No instrument has cost you return yet."
              />
            </div>

            {contributions.unattributed_percentage_points !== 0 && (
              <p className="mt-3 text-[11px] leading-4 text-amber-200/80">
                {formatPercent(contributions.unattributed_percentage_points)} of
                your return belongs to no single instrument (cash interest,
                management and custody fees) and is left unattributed rather
                than assigned to one.
              </p>
            )}
          </>
        )}
      </div>
    </Section>
  );
}

function ContributionList({
  heading,
  rows,
  currency,
  emptyMessage,
}: {
  heading: string;
  rows: InstrumentContribution[];
  currency: string;
  emptyMessage: string;
}) {
  return (
    <div>
      <h3 className="text-[10px] font-semibold uppercase tracking-[0.14em] text-zinc-500">
        {heading}
      </h3>
      {rows.length === 0 ? (
        <p className="mt-2 text-[11px] leading-4 text-zinc-500">
          {emptyMessage}
        </p>
      ) : (
        <ul className="mt-2 space-y-1.5">
          {rows.map((row) => (
            <li key={row.symbol}>
              <Link
                to={`/portfolio?tab=transactions&symbol=${encodeURIComponent(row.symbol)}`}
                className="flex items-center justify-between gap-3 rounded-lg border border-zinc-800 bg-zinc-950/35 px-3 py-2 transition hover:border-zinc-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-300/40"
              >
                <span className="min-w-0">
                  <span className="block font-mono text-sm text-zinc-100">
                    {row.symbol}
                  </span>
                  <span className="block text-[11px] leading-4 text-zinc-500">
                    {row.weight_percentage.toFixed(1)}% of capital ·{" "}
                    {signedMoney(row.economic_result_base, currency)}
                  </span>
                </span>
                <span
                  className={cn(
                    "shrink-0 font-mono text-sm tabular-nums",
                    gainLossColor(row.contribution_percentage_points),
                  )}
                >
                  {formatPercent(row.contribution_percentage_points)}
                </span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

/**
 * Money with an explicit sign, so gain/loss is never signalled by colour alone.
 */
function signedMoney(value: number | null | undefined, currency: string) {
  if (value === null || value === undefined || !Number.isFinite(value)) {
    return "—";
  }
  const formatted = formatMoney(Math.abs(value), currency);
  if (value > 0) return `+${formatted}`;
  if (value < 0) return `-${formatted}`;
  return formatted;
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
      <div
        role="status"
        aria-live="polite"
        className="flex h-64 items-center justify-center"
      >
        <div className="h-5 w-5 animate-spin rounded-full border border-zinc-700 border-t-zinc-300" />
        <span className="sr-only">Loading performance history…</span>
      </div>
    );
  }
  if (isError) {
    return (
      <div
        role="alert"
        className="flex h-64 items-center justify-center text-sm text-rose-300"
      >
        We could not load your performance history.
      </div>
    );
  }
  if (series.length === 0) {
    return (
      <div
        role="status"
        aria-live="polite"
        className="flex h-64 items-center justify-center px-6 text-center text-sm text-zinc-400"
      >
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
