import { useMemo, useState } from "react";
import {
  ArrowRight,
  BadgeCheck,
  RefreshCw,
  Sparkles,
  Target,
} from "lucide-react";

import { AchievementSummaryHero } from "@/components/achievements/AchievementSummaryHero";
import { BadgeDetailModal } from "@/components/achievements/BadgeDetailModal";
import { BadgeDifficultyPill } from "@/components/achievements/BadgeDifficultyPill";
import { BadgeGrid } from "@/components/achievements/BadgeGrid";
import { BadgeMark } from "@/components/achievements/BadgeMark";
import { BadgeProgressBar } from "@/components/achievements/BadgeProgressBar";
import { BadgeStatusPill } from "@/components/achievements/BadgeStatusPill";
import { AppNav } from "@/components/layout/AppNav";
import { useAchievementsProgress } from "@/hooks/useAchievementsProgress";
import type { AchievementProgress } from "@/types/achievements";
import { cn } from "@/utils/cn";

type AchievementTab = "overview" | "legendary" | "strategy" | "all";

const TABS: { value: AchievementTab; label: string; activeTone: string }[] = [
  {
    value: "overview",
    label: "Overview",
    activeTone:
      "border-violet-200/30 bg-gradient-to-r from-violet-200 to-indigo-100 text-violet-950 shadow-violet-950/20",
  },
  {
    value: "legendary",
    label: "Legendary",
    activeTone:
      "border-amber-200/30 bg-gradient-to-r from-amber-200 to-orange-100 text-amber-950 shadow-amber-950/20",
  },
  {
    value: "strategy",
    label: "Strategy",
    activeTone:
      "border-emerald-200/30 bg-gradient-to-r from-emerald-200 to-cyan-100 text-emerald-950 shadow-emerald-950/20",
  },
  {
    value: "all",
    label: "All",
    activeTone:
      "border-sky-200/30 bg-gradient-to-r from-sky-200 to-indigo-100 text-sky-950 shadow-sky-950/20",
  },
];

const LEGENDARY_IDS = [
  "oracle_badge_6m",
  "buffett_portfolio_6m",
  "all_weather_6m",
  "munger_6m",
  "graham_6m",
  "lynch_6m",
  "swensen_6m",
  "soros_1y",
  "quant_1y",
  "druckenmiller_1y",
];

const STRATEGY_IDS = [
  "cash_plus_30d",
  "first_market_edge_30d",
  "gold_check_30d",
  "balanced_start_30d",
  "bogle_badge_90d",
  "global_allocator_90d",
  "dividend_challenger_90d",
  "balanced_beater_90d",
  "inflation_shield_90d",
  "commodity_edge_90d",
];

function selectByIds(items: AchievementProgress[], ids: string[]) {
  const byId = new Map(items.map((item) => [item.id, item]));
  return ids.flatMap((id) => {
    const item = byId.get(id);
    return item ? [item] : [];
  });
}

function progressAvailable(badge: AchievementProgress) {
  return typeof badge.progressPct === "number";
}

function formatEdge(value: number) {
  return `${value >= 0 ? "+" : ""}${value.toFixed(1)} pts`;
}

function LoadingOverview() {
  return (
    <div className="space-y-6">
      <div className="grid gap-3 md:grid-cols-3">
        {Array.from({ length: 3 }).map((_, index) => (
          <div
            key={index}
            className="h-28 animate-pulse rounded-2xl border border-zinc-800 bg-zinc-900/40"
          />
        ))}
      </div>
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {Array.from({ length: 3 }).map((_, index) => (
          <div
            key={index}
            className="h-36 animate-pulse rounded-2xl border border-zinc-800 bg-zinc-900/40"
          />
        ))}
      </div>
    </div>
  );
}

export function AchievementsPage() {
  const { query, items, summary } = useAchievementsProgress();
  const [tab, setTab] = useState<AchievementTab>("overview");
  const [selected, setSelected] = useState<AchievementProgress | null>(null);

  const legendary = useMemo(
    () => selectByIds(items, LEGENDARY_IDS),
    [items],
  );
  const strategy = useMemo(
    () => selectByIds(items, STRATEGY_IDS),
    [items],
  );
  const recentlyUnlocked = useMemo(
    () =>
      items
        .filter((item) => item.status === "unlocked")
        .sort((a, b) => {
          const aTime = a.awardedAt ? new Date(a.awardedAt).getTime() : 0;
          const bTime = b.awardedAt ? new Date(b.awardedAt).getTime() : 0;
          return bTime - aTime;
        })
        .slice(0, 3),
    [items],
  );
  const closest = useMemo(
    () =>
      items
        .filter(
          (item) => item.status !== "unlocked" && progressAvailable(item),
        )
        .sort((a, b) => (b.progressPct ?? 0) - (a.progressPct ?? 0))
        .slice(0, 3),
    [items],
  );

  return (
    <div className="achievements-shell min-h-screen bg-[radial-gradient(circle_at_10%_12%,rgba(139,92,246,0.09),transparent_25%),radial-gradient(circle_at_90%_22%,rgba(245,158,11,0.065),transparent_27%),radial-gradient(circle_at_52%_88%,rgba(16,185,129,0.055),transparent_25%),#09090b] text-zinc-50">
      <main className="mx-auto w-full max-w-6xl px-4 pb-20 pt-4 sm:px-6 lg:px-8">
        <AppNav
          actions={
            <button
              type="button"
              onClick={() => void query.refetch()}
              disabled={query.isFetching}
              aria-label="Refresh achievements"
              className="rounded-lg p-2 text-zinc-400 transition hover:bg-zinc-800/70 hover:text-zinc-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-300/30 disabled:opacity-50"
            >
              <RefreshCw
                className={`h-3.5 w-3.5 ${query.isFetching ? "animate-spin" : ""}`}
              />
            </button>
          }
        />

        <AchievementSummaryHero summary={summary} />

        {query.isError && (
          <p className="mt-4 rounded-xl border border-amber-400/15 bg-amber-400/[0.045] px-4 py-3 text-xs leading-5 text-amber-100/80">
            Badge evaluation is unavailable right now. The catalogue remains
            available, and progress will appear when evaluation is restored.
          </p>
        )}

        <AchievementTabs value={tab} onChange={setTab} />

        <div className="mt-6">
          {query.isLoading ? (
            <LoadingOverview />
          ) : tab === "overview" ? (
            <Overview
              items={items}
              summary={summary}
              recentlyUnlocked={recentlyUnlocked}
              closest={closest}
              onSelect={setSelected}
              onChangeTab={setTab}
            />
          ) : tab === "legendary" ? (
            <CatalogueSection
              eyebrow="Investor-inspired badges"
              title="Legendary investors"
              description="Benchmarks inspired by influential investors and institutional allocation styles. Exact recipes stay inside each badge."
              tone="amber"
              badges={legendary}
              onSelect={setSelected}
            />
          ) : tab === "strategy" ? (
            <CatalogueSection
              eyebrow="Benchmark achievements"
              title="Strategy badges"
              description="Measure your portfolio against predefined market, defensive, income, and allocation benchmarks."
              tone="emerald"
              badges={strategy}
              onSelect={setSelected}
            />
          ) : (
            <AllBadgesList badges={items} onSelect={setSelected} />
          )}
        </div>
      </main>

      <BadgeDetailModal badge={selected} onClose={() => setSelected(null)} />
    </div>
  );
}

function AchievementTabs({
  value,
  onChange,
}: {
  value: AchievementTab;
  onChange: (value: AchievementTab) => void;
}) {
  return (
    <div
      role="tablist"
      aria-label="Achievement views"
      className="mt-6 flex max-w-full gap-1 overflow-x-auto rounded-xl border border-zinc-800 bg-zinc-900/35 p-1"
    >
      {TABS.map((item) => (
        <button
          key={item.value}
          type="button"
          role="tab"
          aria-selected={value === item.value}
          onClick={() => onChange(item.value)}
          className={cn(
            "shrink-0 rounded-lg border border-transparent px-4 py-2 text-xs font-semibold transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-300/30",
            value === item.value
              ? cn("shadow-sm", item.activeTone)
              : "text-zinc-500 hover:bg-zinc-800/70 hover:text-zinc-200",
          )}
        >
          {item.label}
        </button>
      ))}
    </div>
  );
}

function Overview({
  items,
  summary,
  recentlyUnlocked,
  closest,
  onSelect,
  onChangeTab,
}: {
  items: AchievementProgress[];
  summary: ReturnType<typeof useAchievementsProgress>["summary"];
  recentlyUnlocked: AchievementProgress[];
  closest: AchievementProgress[];
  onSelect: (badge: AchievementProgress) => void;
  onChangeTab: (tab: AchievementTab) => void;
}) {
  const closestBadge = closest[0];
  const bestUnlocked = summary.mostPrestigious;
  const nextMajor = items.find(
    (item) =>
      item.status !== "unlocked" &&
      (item.difficulty === "hard" || item.difficulty === "elite"),
  );
  return (
    <div className="space-y-9">
      <section aria-label="Achievement summary" className="grid gap-3 md:grid-cols-3">
        <SummaryCard
          icon={<Target className="h-4 w-4" />}
          tone="sky"
          label="Closest badge"
          value={closestBadge?.name ?? "Progress unavailable"}
          detail={
            closestBadge
              ? `${Math.round(closestBadge.progressPct ?? 0)}% complete`
              : "Benchmark tracking is active and building the required history."
          }
          onClick={closestBadge ? () => onSelect(closestBadge) : undefined}
        />
        <SummaryCard
          icon={<BadgeCheck className="h-4 w-4" />}
          tone="emerald"
          label="Best unlocked"
          value={bestUnlocked?.name ?? "No badge unlocked yet"}
          detail={
            bestUnlocked
              ? bestUnlocked.unlockRule
              : "Your first benchmark win will appear here."
          }
          onClick={bestUnlocked ? () => onSelect(bestUnlocked) : undefined}
        />
        <SummaryCard
          icon={<Sparkles className="h-4 w-4" />}
          tone="violet"
          label="Next major badge"
          value={nextMajor?.name ?? "Major set complete"}
          detail={
            nextMajor
              ? `${nextMajor.difficulty === "elite" ? "Elite" : "Hard"} • ${nextMajor.period}`
              : "Every hard and elite badge is unlocked."
          }
          onClick={nextMajor ? () => onSelect(nextMajor) : undefined}
        />
      </section>

      <OverviewSection
        title="Recently unlocked"
        description="Your latest benchmark wins."
      >
        {recentlyUnlocked.length > 0 ? (
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {recentlyUnlocked.map((badge) => (
              <OverviewBadgeCard
                key={badge.id}
                badge={badge}
                context="recent"
                onSelect={onSelect}
              />
            ))}
          </div>
        ) : (
          <CompactNotice text="No badges unlocked yet. Your first benchmark win will appear here." />
        )}
      </OverviewSection>

      <OverviewSection
        title="Closest to unlock"
        description="The few badges nearest to completion."
      >
        {closest.length > 0 ? (
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {closest.map((badge) => (
              <OverviewBadgeCard
                key={badge.id}
                badge={badge}
                context="closest"
                onSelect={onSelect}
              />
            ))}
          </div>
        ) : (
          <CompactNotice text="Benchmark tracking is active and building the required portfolio history." />
        )}
      </OverviewSection>

      <button
        type="button"
        onClick={() => onChangeTab("all")}
        className="inline-flex items-center gap-1.5 text-xs font-semibold text-zinc-400 transition hover:text-zinc-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-300/30"
      >
        View all badges <ArrowRight className="h-3.5 w-3.5" />
      </button>
    </div>
  );
}

function SummaryCard({
  icon,
  label,
  value,
  detail,
  tone,
  onClick,
}: {
  icon: React.ReactNode;
  label: string;
  value: string;
  detail: string;
  tone: "sky" | "emerald" | "violet";
  onClick?: () => void;
}) {
  const content = (
    <>
      <div className="flex items-center gap-2">
        <span
          className={cn(
            "grid h-7 w-7 place-items-center rounded-lg border",
            tone === "sky" && "border-sky-300/15 bg-sky-300/[0.05] text-sky-300",
            tone === "emerald" &&
              "border-emerald-300/15 bg-emerald-300/[0.05] text-emerald-300",
            tone === "violet" &&
              "border-violet-300/15 bg-violet-300/[0.05] text-violet-300",
          )}
        >
          {icon}
        </span>
        <span className="text-[10px] font-semibold uppercase tracking-[0.16em] text-zinc-600">
          {label}
        </span>
      </div>
      <p className="mt-3 text-sm font-semibold text-zinc-100">{value}</p>
      <p className="mt-1 text-xs leading-5 text-zinc-500">{detail}</p>
    </>
  );

  const className = cn(
    "rounded-2xl border p-4 text-left transition",
    tone === "sky" &&
      "border-sky-300/10 bg-[radial-gradient(circle_at_top_right,rgba(56,189,248,0.07),transparent_50%),rgba(24,24,27,0.42)]",
    tone === "emerald" &&
      "border-emerald-300/10 bg-[radial-gradient(circle_at_top_right,rgba(52,211,153,0.07),transparent_50%),rgba(24,24,27,0.42)]",
    tone === "violet" &&
      "border-violet-300/10 bg-[radial-gradient(circle_at_top_right,rgba(167,139,250,0.08),transparent_50%),rgba(24,24,27,0.42)]",
  );

  return onClick ? (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        className,
        "hover:-translate-y-0.5 hover:border-zinc-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-300/30",
      )}
    >
      {content}
    </button>
  ) : (
    <div className={className}>{content}</div>
  );
}

function OverviewSection({
  title,
  description,
  children,
}: {
  title: string;
  description: string;
  children: React.ReactNode;
}) {
  return (
    <section>
      <div className="mb-3">
        <div>
          <h2 className="achievements-card-title text-lg font-semibold text-zinc-100">{title}</h2>
          <p className="mt-1 text-xs text-zinc-500">{description}</p>
        </div>
      </div>
      {children}
    </section>
  );
}

function OverviewBadgeCard({
  badge,
  context = "preview",
  onSelect,
}: {
  badge: AchievementProgress;
  context?: "preview" | "recent" | "closest";
  onSelect: (badge: AchievementProgress) => void;
}) {
  return (
    <button
      type="button"
      onClick={() => onSelect(badge)}
      className="group rounded-2xl border border-zinc-800 bg-zinc-900/35 p-4 text-left transition hover:border-zinc-700 hover:bg-zinc-900/55 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-300/30"
    >
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <h3 className="achievements-card-title truncate text-base font-semibold text-zinc-100">
            {badge.name}
          </h3>
          <div className="mt-2 flex flex-wrap items-center gap-1.5">
            <BadgeDifficultyPill difficulty={badge.difficulty} />
            <BadgeStatusPill status={badge.status} />
          </div>
        </div>
        <BadgeMark badgeId={badge.id} className="h-9 w-9" />
      </div>

      <p className="mt-3 line-clamp-2 text-xs leading-5 text-zinc-500">
        {badge.explanation}
      </p>

      {context === "recent" && (
        <p className="mt-3 text-[11px] text-emerald-300/75">
          {badge.awardedAt
            ? `Awarded ${new Date(badge.awardedAt).toLocaleDateString()}`
            : "Unlocked"}
        </p>
      )}

      {context === "closest" && progressAvailable(badge) && (
        <div className="mt-3 space-y-2">
          <BadgeProgressBar pct={badge.progressPct ?? 0} status={badge.status} />
          <div className="flex items-center justify-between gap-3 text-[11px]">
            <span className="font-mono text-sky-300">
              {Math.round(badge.progressPct ?? 0)}%
            </span>
            {typeof badge.currentEdgePoints === "number" && (
              <span className="text-zinc-500">
                {formatEdge(badge.currentEdgePoints)} / +
                {badge.requiredEdgePoints.toFixed(1)} pts
              </span>
            )}
          </div>
        </div>
      )}
    </button>
  );
}

function CompactNotice({ text }: { text: string }) {
  return (
    <div className="rounded-2xl border border-dashed border-zinc-800 bg-zinc-900/25 px-5 py-8 text-center text-sm text-zinc-500">
      {text}
    </div>
  );
}

function CatalogueSection({
  eyebrow,
  title,
  description,
  tone,
  badges,
  onSelect,
}: {
  eyebrow: string;
  title: string;
  description: string;
  tone: "amber" | "emerald";
  badges: AchievementProgress[];
  onSelect: (badge: AchievementProgress) => void;
}) {
  return (
    <section>
      <p
        className={cn(
          "text-[10px] font-semibold uppercase tracking-[0.18em]",
          tone === "amber" ? "text-amber-300/70" : "text-emerald-300/70",
        )}
      >
        {eyebrow}
      </p>
      <h2 className="achievements-display mt-1.5 text-2xl font-semibold tracking-tight text-zinc-100">
        {title}
      </h2>
      <p className="mt-1 max-w-2xl text-sm leading-6 text-zinc-500">
        {description}
      </p>
      <div className="mt-5">
        <BadgeGrid badges={badges} onSelect={onSelect} />
      </div>
    </section>
  );
}

function AllBadgesList({
  badges,
  onSelect,
}: {
  badges: AchievementProgress[];
  onSelect: (badge: AchievementProgress) => void;
}) {
  return (
    <section>
      <div className="mb-4">
        <p className="text-[10px] font-semibold uppercase tracking-[0.18em] text-zinc-600">
          Full catalogue
        </p>
        <h2 className="achievements-display mt-1.5 text-2xl font-semibold tracking-tight text-zinc-100">
          All badges
        </h2>
        <p className="mt-1 text-sm text-zinc-500">
          Select a row to inspect its benchmark recipe and exact unlock rule.
        </p>
      </div>

      <div className="overflow-hidden rounded-2xl border border-zinc-800 bg-zinc-950/30">
        <div className="hidden grid-cols-[1fr_7rem_6rem_8rem] border-b border-zinc-800 bg-gradient-to-r from-violet-300/[0.035] via-transparent to-sky-300/[0.035] px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.14em] text-zinc-600 sm:grid">
          <span>Badge</span>
          <span>Difficulty</span>
          <span>Period</span>
          <span className="text-right">Status</span>
        </div>
        {badges.map((badge) => (
          <button
            key={badge.id}
            type="button"
            onClick={() => onSelect(badge)}
            className="grid w-full gap-2 border-b border-zinc-900 px-4 py-3.5 text-left transition last:border-b-0 hover:bg-violet-300/[0.025] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-violet-300/30 sm:grid-cols-[1fr_7rem_6rem_8rem] sm:items-center"
          >
            <div className="flex min-w-0 items-center gap-3">
              <BadgeMark badgeId={badge.id} className="h-8 w-8 rounded-lg" iconClassName="h-3.5 w-3.5" />
              <div className="min-w-0">
                <span className="achievements-card-title text-base font-semibold text-zinc-100">
                  {badge.name}
                </span>
                <p className="mt-0.5 truncate text-xs text-zinc-600">
                  {badge.inspiredBy}
                </p>
              </div>
            </div>
            <div className="flex items-center gap-2 sm:block">
              <span className="text-[10px] uppercase text-zinc-600 sm:hidden">
                Difficulty
              </span>
              <BadgeDifficultyPill difficulty={badge.difficulty} />
            </div>
            <div className="flex items-center gap-2 text-xs text-zinc-400 sm:block">
              <span className="text-[10px] uppercase text-zinc-600 sm:hidden">
                Period
              </span>
              {badge.period}
            </div>
            <div className="flex items-center gap-2 sm:justify-end">
              <span className="text-[10px] uppercase text-zinc-600 sm:hidden">
                Status
              </span>
              {progressAvailable(badge) && badge.status !== "unlocked" ? (
                <span className="font-mono text-xs text-sky-300">
                  {Math.round(badge.progressPct ?? 0)}%
                </span>
              ) : (
                <BadgeStatusPill status={badge.status} />
              )}
            </div>
          </button>
        ))}
      </div>
    </section>
  );
}
