import { useState, type ReactNode } from "react";
import {
  Activity,
  BarChart3,
  CalendarDays,
  ChevronRight,
  CircleUserRound,
  Gauge,
  Info,
  LineChart,
  LockKeyhole,
  Medal,
  Minus,
  Plus,
  Target,
  TrendingDown,
  TrendingUp,
} from "lucide-react";

import { ConcentrationCard } from "@/components/profile/ConcentrationCard";
import { ExposureBreakdownCard } from "@/components/profile/ExposureBreakdownCard";
import { FollowButton } from "@/components/social/FollowButton";
import { ProfileBadgesCard } from "@/components/profile/ProfileBadgesCard";
import { ProfilePerformanceHistoryCard } from "@/components/profile/ProfilePerformanceHistoryCard";
import { PublicClosedPositionsCard } from "@/components/profile/PublicClosedPositionsCard";
import { PublicWeightsCard } from "@/components/profile/PublicWeightsCard";
import { StrategyProfileActions } from "@/components/strategy/StrategyProfileActions";
import { useMyProfile } from "@/hooks/useProfile";
import type {
  ProfileContributor,
  ProfileDNAScores,
  ProfileInsights,
  PublicProfile,
} from "@/types/profile";
import { cn } from "@/utils/cn";
import { formatPercent } from "@/utils/formatPercent";
import { gainLossColor } from "@/utils/gainLoss";

const dnaLabels: {
  key: keyof ProfileDNAScores;
  label: string;
  hint: string;
}[] = [
  { key: "growth", label: "Growth", hint: "Exposure to high-growth assets" },
  { key: "income", label: "Income", hint: "Dividend and yield exposure" },
  {
    key: "commodities",
    label: "Commodities",
    hint: "Metals, energy, and resources",
  },
  {
    key: "defensive",
    label: "Defensive",
    hint: "Stability, adjusted for risk",
  },
  {
    key: "international",
    label: "International",
    hint: "Exposure outside the US",
  },
  {
    key: "concentration",
    label: "Concentration",
    hint: "How few positions dominate",
  },
  {
    key: "volatility",
    label: "Volatility",
    hint: "Asset risk plus concentration",
  },
];

type ProfileSectionKey =
  | "drivers"
  | "dna"
  | "contributors"
  | "benchmarks"
  | "composition"
  | "recognition";

const pretty = (value?: string) =>
  value ? value.replaceAll("_", " ").replace(/\b\w/g, (c) => c.toUpperCase()) : "";

function formatNumber(value?: number, digits = 2) {
  return typeof value === "number" && Number.isFinite(value)
    ? value.toFixed(digits)
    : "-";
}

function formatPoints(value?: number) {
  if (typeof value !== "number" || !Number.isFinite(value)) return "-";
  return `${value > 0 ? "+" : ""}${value.toFixed(2)} pts`;
}

function Panel({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <section
      className={cn(
        "rounded-lg border border-zinc-800 bg-zinc-900/45 p-5 shadow-sm shadow-black/20",
        className,
      )}
    >
      {children}
    </section>
  );
}

function HeroMetric({
  label,
  value,
  tone,
}: {
  label: string;
  value: string;
  tone?: string;
}) {
  return (
    <div className="min-w-0 rounded-lg border border-white/10 bg-zinc-950/35 px-4 py-3">
      <p className="text-[10px] font-medium uppercase tracking-widest text-zinc-500">
        {label}
      </p>
      <p
        className={cn(
          "mt-2 truncate font-mono text-lg font-semibold tabular-nums text-zinc-100",
          tone,
        )}
      >
        {value}
      </p>
    </div>
  );
}

function FocusPill({ children }: { children: ReactNode }) {
  return (
    <span className="rounded-full border border-zinc-700 bg-zinc-950/45 px-3 py-1 text-xs text-zinc-300">
      {children}
    </span>
  );
}

function InvestmentStyleHero({
  profile,
  insights,
  actions,
}: {
  profile: PublicProfile;
  insights: ProfileInsights;
  actions: ReactNode;
}) {
  const concentrationLabel =
    insights.dna.concentration >= 70
      ? "Concentrated"
      : insights.dna.concentration <= 35
        ? "Diversified"
        : "Balanced";
  const volatilityLabel =
    insights.dna.volatility >= 65
      ? "High volatility"
      : insights.dna.volatility <= 35
        ? "Lower volatility"
        : "Moderate volatility";
  const benchmarkEdge =
    insights.benchmark_context.benchmarks.find((item) => item.symbol === "SPY")
      ?.edge_points;

  return (
    <header className="overflow-hidden rounded-lg border border-zinc-800 bg-[radial-gradient(circle_at_top_left,rgba(14,165,233,0.18),transparent_34%),linear-gradient(135deg,rgba(24,24,27,0.96),rgba(9,9,11,0.98))] p-5 shadow-xl shadow-black/25 sm:p-6">
      <div className="flex flex-col gap-5 lg:flex-row lg:items-start lg:justify-between">
        <div className="flex min-w-0 gap-4">
          <div className="grid h-16 w-16 shrink-0 place-items-center rounded-lg border border-zinc-700 bg-zinc-950 text-zinc-300">
            <CircleUserRound className="h-7 w-7" />
          </div>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h1 className="break-words text-2xl font-semibold tracking-tight text-zinc-50 sm:text-3xl">
                {profile.display_name}
              </h1>
              {profile.strategy_tag && (
                <span className="rounded-full border border-cyan-300/20 bg-cyan-300/[0.07] px-2.5 py-1 text-xs text-cyan-100">
                  {pretty(profile.strategy_tag)}
                </span>
              )}
            </div>
            <p className="mt-1 break-all font-mono text-xs text-zinc-500">
              @{profile.handle}
            </p>
            <div className="mt-5">
              <p className="text-xs font-medium uppercase tracking-widest text-zinc-500">
                Investment style
              </p>
              <p className="mt-2 text-xl font-semibold text-zinc-50">
                {insights.investment_style}
              </p>
              <p className="mt-2 max-w-3xl text-sm leading-6 text-zinc-300">
                {insights.style_summary}
              </p>
              {profile.bio && (
                <p className="mt-3 max-w-3xl border-l border-zinc-700 pl-3 text-sm leading-6 text-zinc-400">
                  {profile.bio}
                </p>
              )}
            </div>
          </div>
        </div>
        <div className="flex flex-wrap justify-start gap-2 lg:justify-end">{actions}</div>
      </div>

      <div className="mt-6 flex flex-wrap gap-2">
        {insights.focus_areas.length === 0 ? (
          <FocusPill>Focus areas pending</FocusPill>
        ) : (
          insights.focus_areas.map((area) => (
            <FocusPill key={area}>{area}</FocusPill>
          ))
        )}
      </div>

      <div className="mt-6 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <HeroMetric
          label="Investor index"
          value={formatNumber(profile.portfolio_index)}
        />
        <HeroMetric
          label="Return"
          value={formatPercent(profile.return_percentage)}
          tone={gainLossColor(profile.return_percentage)}
        />
        <HeroMetric
          label="Benchmark edge"
          value={
            typeof benchmarkEdge === "number"
              ? formatPoints(benchmarkEdge)
              : "Pending"
          }
        />
        <HeroMetric
          label="Behavior"
          value={`${concentrationLabel} / ${volatilityLabel}`}
        />
      </div>

      <div className="mt-5 flex flex-wrap gap-x-4 gap-y-2 text-xs text-zinc-500">
        {profile.global_rank ? (
          <span className="flex items-center gap-1.5">
            <Medal className="h-3.5 w-3.5" /> Global #{profile.global_rank}
          </span>
        ) : null}
        {profile.sprint_rank ? (
          <span className="flex items-center gap-1.5">
            <Activity className="h-3.5 w-3.5" /> Sprint #{profile.sprint_rank}
          </span>
        ) : null}
        {profile.joined_at ? (
          <span className="flex items-center gap-1.5">
            <CalendarDays className="h-3.5 w-3.5" />
            Joined {new Date(profile.joined_at).toLocaleDateString()}
          </span>
        ) : null}
      </div>
    </header>
  );
}

function DNABar({
  label,
  hint,
  value,
  explanations,
}: {
  label: string;
  hint: string;
  value: number;
  explanations?: string[];
}) {
  const width = Math.min(100, Math.max(0, value));
  // A single, restrained accent keeps the card analytical rather than gamified;
  // intensity encodes the score instead of a rainbow of per-row colors.
  const intensity =
    width >= 66
      ? "bg-cyan-300"
      : width >= 33
        ? "bg-cyan-400/70"
        : "bg-zinc-500";

  return (
    <div>
      <div className="flex items-baseline justify-between gap-3">
        <span className="text-sm text-zinc-200">{label}</span>
        <span className="font-mono text-sm tabular-nums text-zinc-100">
          {Math.round(value)}
        </span>
      </div>
      <div className="mt-1.5 h-1.5 overflow-hidden rounded-full bg-zinc-800">
        <div
          className={cn("h-full rounded-full transition-all", intensity)}
          style={{ width: `${width}%` }}
        />
      </div>
      {explanations && explanations.length > 0 ? (
        <p className="mt-1.5 text-xs leading-5 text-zinc-500">
          {explanations[0]}
        </p>
      ) : (
        <p className="mt-1.5 text-xs leading-5 text-zinc-600">{hint}</p>
      )}
    </div>
  );
}

function PortfolioDNACard({ insights }: { insights: ProfileInsights }) {
  const hasDNA = dnaLabels.some(({ key }) => insights.dna[key] > 0);
  const explanations = insights.dna_explanations;

  return (
    <Panel>
      <div className="flex items-start justify-between gap-4">
        <div>
          <h2 className="text-sm font-semibold text-zinc-100">Portfolio DNA</h2>
          <p className="mt-1 text-xs leading-5 text-zinc-500">
            Seven deterministic 0-100 signals derived from public-safe portfolio
            structure.
          </p>
        </div>
        <Gauge className="h-4 w-4 text-zinc-500" />
      </div>
      {hasDNA ? (
        <>
          <div className="mt-5 grid gap-x-6 gap-y-5 sm:grid-cols-2">
            {dnaLabels.map((item) => (
              <DNABar
                key={item.key}
                label={item.label}
                hint={item.hint}
                value={insights.dna[item.key]}
                explanations={explanations?.[item.key]}
              />
            ))}
          </div>
          <p className="mt-5 rounded-lg border border-zinc-800 bg-zinc-950/35 px-4 py-3 text-sm leading-6 text-zinc-400">
            {insights.style_summary}
          </p>
        </>
      ) : (
        <div className="mt-5 rounded-lg border border-dashed border-zinc-800 px-4 py-8 text-center text-sm text-zinc-500">
          Portfolio DNA will appear after this investor adds enough positions.
        </div>
      )}
    </Panel>
  );
}

function DriverList({
  title,
  items,
  positive,
}: {
  title: string;
  items: string[];
  positive?: boolean;
}) {
  return (
    <div className="rounded-lg border border-zinc-800 bg-zinc-950/30 p-4">
      <div className="flex items-center gap-2 text-xs font-medium uppercase tracking-widest text-zinc-500">
        {positive ? (
          <Plus className="h-3.5 w-3.5 text-emerald-300" />
        ) : (
          <Minus className="h-3.5 w-3.5 text-rose-300" />
        )}
        {title}
      </div>
      <div className="mt-3 space-y-2">
        {items.map((item) => (
          <p key={item} className="text-sm leading-5 text-zinc-300">
            {item}
          </p>
        ))}
      </div>
    </div>
  );
}

function PerformanceDriversCard({ insights }: { insights: ProfileInsights }) {
  return (
    <Panel>
      <div className="flex items-start justify-between gap-4">
        <div>
          <h2 className="text-sm font-semibold text-zinc-100">
            Performance drivers
          </h2>
          <p className="mt-1 text-xs leading-5 text-zinc-500">
            How this portfolio won or lost, expressed in percentage/index terms.
          </p>
        </div>
        <Target className="h-4 w-4 text-zinc-500" />
      </div>
      <p className="mt-5 text-sm leading-6 text-zinc-300">
        {insights.performance_drivers.summary}
      </p>
      <div className="mt-5 grid gap-3 sm:grid-cols-2">
        <DriverList
          title="Positive"
          items={insights.performance_drivers.positive_drivers}
          positive
        />
        <DriverList
          title="Negative"
          items={insights.performance_drivers.negative_drivers}
        />
      </div>
    </Panel>
  );
}

function OpenClosedPerformanceCard({ insights }: { insights: ProfileInsights }) {
  const data = insights.open_closed_performance;
  const rows = [
    {
      label: "Open positions",
      returnValue: data.open_return_percentage,
      points: data.open_contribution_points,
    },
    {
      label: "Closed positions",
      returnValue: data.closed_return_percentage,
      points: data.closed_contribution_points,
      empty: !data.has_closed_positions,
    },
  ];

  return (
    <Panel>
      <div className="flex items-start justify-between gap-4">
        <div>
          <h2 className="text-sm font-semibold text-zinc-100">
            Open vs closed performance
          </h2>
          <p className="mt-1 text-xs leading-5 text-zinc-500">
            Current and realized performance without exposing absolute values.
          </p>
        </div>
        <BarChart3 className="h-4 w-4 text-zinc-500" />
      </div>
      <div className="mt-5 space-y-3">
        {rows.map((row) => (
          <div
            key={row.label}
            className="rounded-lg border border-zinc-800 bg-zinc-950/30 px-4 py-3"
          >
            <div className="flex items-center justify-between gap-4">
              <p className="text-sm font-medium text-zinc-200">{row.label}</p>
              {row.empty ? (
                <p className="text-xs text-zinc-500">
                  Closed-position history is not available yet.
                </p>
              ) : (
                <div className="text-right">
                  <p
                    className={cn(
                      "font-mono text-sm font-semibold tabular-nums",
                      gainLossColor(row.returnValue),
                    )}
                  >
                    {formatPercent(row.returnValue)}
                  </p>
                  <p className="mt-0.5 font-mono text-xs tabular-nums text-zinc-500">
                    {formatPoints(row.points)}
                  </p>
                </div>
              )}
            </div>
          </div>
        ))}
      </div>
      {data.includes_self_reported_prices ? (
        <div className="mt-4 flex items-start gap-2 rounded-lg border border-amber-400/20 bg-amber-400/[0.04] px-3 py-2.5">
          <Info className="mt-0.5 h-3.5 w-3.5 shrink-0 text-amber-300" />
          <p className="text-xs leading-5 text-amber-200/90">
            These two figures include at least one self-reported execution
            price and cannot be independently verified. The ranked
            performance index above is always priced from tracked market
            quotes and is not affected.
          </p>
        </div>
      ) : null}
    </Panel>
  );
}

function BenchmarkContextCard({ insights }: { insights: ProfileInsights }) {
  const context = insights.benchmark_context;

  return (
    <Panel>
      <div className="flex items-start justify-between gap-4">
        <div>
          <h2 className="text-sm font-semibold text-zinc-100">
            Benchmark context
          </h2>
          <p className="mt-1 text-xs leading-5 text-zinc-500">
            Index comparison frame for making return numbers easier to read.
          </p>
        </div>
        <LineChart className="h-4 w-4 text-zinc-500" />
      </div>
      <div className="mt-5 rounded-lg border border-zinc-800 bg-zinc-950/30 px-4 py-3">
        <div className="flex items-center justify-between gap-4">
          <p className="text-sm text-zinc-300">Investor index</p>
          <p className="font-mono text-lg font-semibold tabular-nums text-zinc-100">
            {formatNumber(context.investor_index)}
          </p>
        </div>
      </div>
      {context.benchmarks.length === 0 ? (
        <div className="mt-3 rounded-lg border border-dashed border-zinc-800 px-4 py-6 text-center text-sm text-zinc-500">
          {context.note ?? "Benchmark data is unavailable right now."}
        </div>
      ) : (
        <div className="mt-3 space-y-2">
          {context.benchmarks.map((benchmark) => (
            <div
              key={benchmark.symbol}
              className="flex items-center justify-between gap-4 rounded-lg border border-zinc-800 bg-zinc-950/30 px-4 py-3"
            >
              <div>
                <p className="font-mono text-sm font-semibold text-zinc-100">
                  {benchmark.symbol}
                </p>
                <p className="mt-0.5 text-xs text-zinc-500">
                  {benchmark.name}
                </p>
              </div>
              <div className="text-right">
                <p className="font-mono text-sm tabular-nums text-zinc-200">
                  {formatNumber(benchmark.index)}
                </p>
                <p
                  className={cn(
                    "mt-0.5 font-mono text-xs font-semibold tabular-nums",
                    gainLossColor(benchmark.edge_points),
                  )}
                >
                  {formatPoints(benchmark.edge_points)}
                </p>
              </div>
            </div>
          ))}
        </div>
      )}
    </Panel>
  );
}

function ContributorRows({
  title,
  items,
  positive,
}: {
  title: string;
  items: ProfileContributor[];
  positive?: boolean;
}) {
  return (
    <div>
      <div className="flex items-center gap-2 text-xs font-medium uppercase tracking-widest text-zinc-500">
        {positive ? (
          <TrendingUp className="h-3.5 w-3.5 text-emerald-300" />
        ) : (
          <TrendingDown className="h-3.5 w-3.5 text-rose-300" />
        )}
        {title}
      </div>
      <div className="mt-3 space-y-2">
        {items.length === 0 ? (
          <div className="rounded-lg border border-dashed border-zinc-800 px-4 py-6 text-center text-sm text-zinc-500">
            {positive
              ? "No positive contributor is visible yet."
              : "No detractor is visible yet."}
          </div>
        ) : (
          items.map((item) => (
            <div
              key={`${title}-${item.symbol}`}
              className="flex items-center justify-between gap-4 rounded-lg border border-zinc-800 bg-zinc-950/30 px-4 py-3"
            >
              <div className="min-w-0">
                <p className="truncate font-mono text-sm font-semibold text-zinc-100">
                  {item.symbol}
                </p>
                {item.name && (
                  <p className="mt-0.5 truncate text-xs text-zinc-500">
                    {item.name}
                  </p>
                )}
              </div>
              <p
                className={cn(
                  "font-mono text-sm font-semibold tabular-nums",
                  gainLossColor(item.contribution_points),
                )}
              >
                {formatPoints(item.contribution_points)}
              </p>
            </div>
          ))
        )}
      </div>
    </div>
  );
}

function ContributorsCard({ insights }: { insights: ProfileInsights }) {
  if (!insights.open_closed_performance.composition_visible) {
    return (
      <Panel>
        <div className="flex items-start justify-between gap-4">
          <div>
            <h2 className="text-sm font-semibold text-zinc-100">
              Contributors and detractors
            </h2>
            <p className="mt-1 text-xs leading-5 text-zinc-500">
              Symbol-level drivers are hidden when composition sharing is off.
            </p>
          </div>
          <LockKeyhole className="h-4 w-4 text-zinc-500" />
        </div>
        <div className="mt-5 rounded-lg border border-dashed border-zinc-800 px-4 py-8 text-center text-sm text-zinc-500">
          This profile keeps portfolio composition private.
        </div>
      </Panel>
    );
  }

  return (
    <Panel>
      <div className="flex items-start justify-between gap-4">
        <div>
          <h2 className="text-sm font-semibold text-zinc-100">
            Contributors and detractors
          </h2>
          <p className="mt-1 text-xs leading-5 text-zinc-500">
            Symbol-level contribution in index points, never money.
          </p>
        </div>
        <Activity className="h-4 w-4 text-zinc-500" />
      </div>
      <div className="mt-5 grid gap-5 sm:grid-cols-2">
        <ContributorRows
          title="Top contributors"
          items={insights.contributors}
          positive
        />
        <ContributorRows title="Top detractors" items={insights.detractors} />
      </div>
    </Panel>
  );
}

function ProfileSectionShell({
  activeSection,
  onSectionChange,
  children,
}: {
  activeSection: ProfileSectionKey;
  onSectionChange: (section: ProfileSectionKey) => void;
  children: ReactNode;
}) {
  const sections: {
    key: ProfileSectionKey;
    label: string;
    description: string;
    icon: ReactNode;
  }[] = [
    {
      key: "drivers",
      label: "Performance drivers",
      description: "How returns were made or lost",
      icon: <Target className="h-4 w-4" />,
    },
    {
      key: "dna",
      label: "Portfolio DNA",
      description: "Style, exposure, concentration",
      icon: <Gauge className="h-4 w-4" />,
    },
    {
      key: "contributors",
      label: "Contributors and detractors",
      description: "Symbol-level index impact",
      icon: <Activity className="h-4 w-4" />,
    },
    {
      key: "benchmarks",
      label: "Benchmark context",
      description: "Index comparison and history",
      icon: <LineChart className="h-4 w-4" />,
    },
    {
      key: "composition",
      label: "Public composition",
      description: "Shared weights and closed positions",
      icon: <BarChart3 className="h-4 w-4" />,
    },
    {
      key: "recognition",
      label: "Badges",
      description: "Public achievements",
      icon: <Medal className="h-4 w-4" />,
    },
  ];

  return (
    <section className="grid gap-5 lg:grid-cols-[320px_1fr]">
      <div className="rounded-lg border border-zinc-800 bg-zinc-900/45 p-3 shadow-sm shadow-black/20 lg:sticky lg:top-4 lg:self-start">
        <div className="px-2 pb-3 pt-1">
          <h2 className="text-sm font-semibold text-zinc-100">
            Profile sections
          </h2>
          <p className="mt-1 text-xs leading-5 text-zinc-500">
            Choose what you want to inspect.
          </p>
        </div>
        <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-1">
          {sections.map((section) => {
            const isActive = section.key === activeSection;
            return (
              <button
                key={section.key}
                type="button"
                onClick={() => onSectionChange(section.key)}
                className={cn(
                  "group flex min-h-16 items-center gap-3 rounded-lg border px-3 py-3 text-left transition",
                  isActive
                    ? "border-cyan-300/30 bg-cyan-300/[0.08] text-cyan-50"
                    : "border-zinc-800 bg-zinc-950/25 text-zinc-400 hover:border-zinc-700 hover:bg-zinc-900/70 hover:text-zinc-200",
                )}
                aria-pressed={isActive}
              >
                <span
                  className={cn(
                    "grid h-8 w-8 shrink-0 place-items-center rounded-lg border",
                    isActive
                      ? "border-cyan-300/25 bg-cyan-300/[0.10] text-cyan-100"
                      : "border-zinc-800 bg-zinc-950 text-zinc-500 group-hover:text-zinc-300",
                  )}
                >
                  {section.icon}
                </span>
                <span className="min-w-0 flex-1">
                  <span className="block text-sm font-medium">
                    {section.label}
                  </span>
                  <span className="mt-0.5 block text-xs leading-4 text-zinc-500">
                    {section.description}
                  </span>
                </span>
                <ChevronRight
                  className={cn(
                    "hidden h-4 w-4 shrink-0 transition sm:block",
                    isActive ? "text-cyan-100" : "text-zinc-700",
                  )}
                />
              </button>
            );
          })}
        </div>
      </div>
      <div className="min-w-0 space-y-5">{children}</div>
    </section>
  );
}

export function PublicProfileDisplay({ profile }: { profile: PublicProfile }) {
  const me = useMyProfile();
  const isSelf = me.data?.handle === profile.handle;
  const [activeSection, setActiveSection] =
    useState<ProfileSectionKey>("drivers");
  const actions = (
    <>
      <FollowButton handle={profile.handle} isSelf={isSelf} />
      {profile.public_weights.length > 0 && (
        <StrategyProfileActions
          handle={profile.handle}
          displayName={profile.display_name}
          canCopy
        />
      )}
    </>
  );

  return (
    <div className="space-y-5">
      <InvestmentStyleHero
        profile={profile}
        insights={profile.insights}
        actions={actions}
      />
      <ProfileSectionShell
        activeSection={activeSection}
        onSectionChange={setActiveSection}
      >
        {activeSection === "drivers" && (
          <>
            <PerformanceDriversCard insights={profile.insights} />
            <OpenClosedPerformanceCard insights={profile.insights} />
          </>
        )}
        {activeSection === "dna" && (
          <>
            <PortfolioDNACard insights={profile.insights} />
            <div className="grid gap-5 xl:grid-cols-2">
              <ExposureBreakdownCard
                assetTypes={profile.asset_type_exposure}
                currencies={profile.currency_exposure}
              />
              <ConcentrationCard concentration={profile.concentration} />
            </div>
          </>
        )}
        {activeSection === "contributors" && (
          <ContributorsCard insights={profile.insights} />
        )}
        {activeSection === "benchmarks" && (
          <>
            <BenchmarkContextCard insights={profile.insights} />
            <ProfilePerformanceHistoryCard history={profile.performance_history} />
          </>
        )}
        {activeSection === "composition" && (
          <div className="grid gap-5 xl:grid-cols-2">
            <PublicWeightsCard weights={profile.public_weights} />
            <PublicClosedPositionsCard
              positions={profile.public_closed_positions}
            />
          </div>
        )}
        {activeSection === "recognition" && (
          <ProfileBadgesCard badges={profile.badges} />
        )}
      </ProfileSectionShell>
    </div>
  );
}
