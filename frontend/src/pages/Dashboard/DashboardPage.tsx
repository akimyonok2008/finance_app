import type { LucideIcon } from "lucide-react";
import {
  ArrowRight,
  BarChart3,
  Compass,
  Eye,
  Medal,
  RefreshCw,
} from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";

import { useAuth } from "@/auth/useAuth";
import { AppNav } from "@/components/layout/AppNav";
import { QuoteFreshnessBadge } from "@/components/portfolio/QuoteFreshnessBadge";
import { useDashboard } from "@/hooks/useDashboard";
import { useExplore } from "@/hooks/useExplore";
import { useMyProfile } from "@/hooks/useProfile";
import { usePositions } from "@/hooks/usePositions";
import { PerformanceChartCard } from "@/pages/Dashboard/components/PerformanceChartCard";
import { formatRank } from "@/pages/Dashboard/utils/dashboardFormatters";
import type { DashboardPortfolioSummary, LeaderboardMe } from "@/types/dashboard";
import type { ExploreResponse } from "@/types/explore";
import type { MyProfile } from "@/types/profile";
import type { Position, QuoteStatus } from "@/types/portfolio";
import { cn } from "@/utils/cn";
import { formatMoney } from "@/utils/formatMoney";
import { formatPercent } from "@/utils/formatPercent";

type Tone = "default" | "emerald" | "rose" | "amber" | "violet";

export function DashboardPage() {
  const { user } = useAuth();
  const dashboard = useDashboard();
  const positionsQuery = usePositions();
  const profileQuery = useMyProfile();
  const exploreQuery = useExplore({
    sort: "top",
    timeframe: "ALL",
    limit: 3,
    offset: 0,
  });
  const [isRefreshing, setIsRefreshing] = useState(false);

  const portfolioSummary = dashboard.portfolioSummary;
  const positions = positionsQuery.data ?? [];
  const leaderboardMe = dashboard.leaderboardMe;
  const profile = profileQuery.data ?? null;
  const explore = exploreQuery.data ?? null;

  const handleRefresh = async () => {
    setIsRefreshing(true);
    await Promise.all([
      dashboard.refetch.all(),
      positionsQuery.refetch(),
      profileQuery.refetch(),
      exploreQuery.refetch(),
    ]);
    setIsRefreshing(false);
  };

  return (
    <div className="min-h-screen bg-zinc-950 text-zinc-50">
      <main className="mx-auto w-full max-w-7xl px-4 pb-16 pt-4 sm:px-6 lg:px-8">
        <AppNav
          actions={
            <button
              type="button"
              onClick={handleRefresh}
              disabled={isRefreshing}
              aria-label="Refresh dashboard"
              className="rounded-lg p-2 text-zinc-400 transition hover:bg-zinc-800/70 hover:text-zinc-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-500 disabled:opacity-50"
            >
              <RefreshCw
                className={`h-3.5 w-3.5 ${isRefreshing ? "animate-spin" : ""}`}
              />
            </button>
          }
        />

        <DashboardHero
          displayName={user?.display_name || user?.email}
          summary={portfolioSummary}
          positions={positions}
          leaderboardMe={leaderboardMe}
          profile={profile}
          explore={explore}
        />

        <div className="mt-5 grid gap-4 lg:grid-cols-[minmax(0,1fr)_20rem]">
          <PerformanceChartCard
            summary={portfolioSummary}
            isLoading={dashboard.isLoading}
            isError={!!dashboard.errors.summary}
            onRetry={dashboard.refetch.summary}
          />

          <aside className="space-y-5">
            <NextActionCard
              positions={positions}
              profile={profile}
              leaderboardMe={leaderboardMe}
            />
            <MarketDataStatus status={portfolioSummary?.quote_status} />
            <PrivateSnapshot summary={portfolioSummary} />
          </aside>
        </div>
      </main>
    </div>
  );
}

function DashboardHero({
  displayName,
  summary,
  positions,
  leaderboardMe,
  profile,
  explore,
}: {
  displayName?: string;
  summary: DashboardPortfolioSummary | null;
  positions: Position[];
  leaderboardMe: LeaderboardMe | null;
  profile: MyProfile | null;
  explore: ExploreResponse | null;
}) {
  const portfolioReturn = summary
    ? formatPercent(summary.gain_loss_percentage)
    : "Not started";
  const portfolioDetail = summary
    ? `${formatMoney(summary.current_value, summary.base_currency ?? "USD")} | index ${summary.portfolio_index.toFixed(2)}`
    : "Add holdings to create a baseline";
  const leaderboardValue = leaderboardMe?.rank
    ? formatRank(leaderboardMe.rank)
    : "Not ranked";
  const leaderboardDetail = leaderboardMe?.total_participants
    ? `${leaderboardMe.total_participants} ranked strategies`
    : "Rank appears after your baseline";
  const profileValue = profile
    ? profile.is_public
      ? "Public profile"
      : "Private profile"
    : "Profile pending";
  const profileDetail = profile
    ? profile.show_public_weights
      ? "Weights visible by percentage"
      : "Weights stay private"
    : "Set visibility and strategy tag";
  const trend = explore?.trending_holdings?.[0];
  const topPerformer = explore?.top_performers?.[0];
  const exploreValue = trend?.symbol
    ? trend.symbol
    : topPerformer?.display_name
      ? topPerformer.display_name
      : "Public strategies";
  const exploreDetail = trend
    ? `${trend.profile_count} public profiles hold it`
    : topPerformer?.ranked_return_percentage !== undefined
      ? `${formatPercent(topPerformer.ranked_return_percentage)} ranked return`
      : "Compare public weights and ideas";

  return (
    <section className="rounded-2xl border border-zinc-800 bg-zinc-900/45 p-4 shadow-sm shadow-black/20 sm:p-5">
      <div className="grid gap-5 xl:grid-cols-[minmax(0,0.9fr)_minmax(0,1.1fr)] xl:items-end">
        <div>
          <p className="text-xs font-medium uppercase tracking-[0.16em] text-zinc-500">
            {displayName ? `Welcome back, ${displayName}` : "Welcome back"}
          </p>
          <h1 className="mt-2 text-2xl font-medium tracking-tight">
            Your investing command center
          </h1>
          <p className="mt-2 max-w-3xl text-sm leading-6 text-zinc-400">
            Track your private portfolio, compete by ranked performance, and discover
            public strategies without exposing wealth.
          </p>
        </div>
        <div className="grid gap-3 sm:grid-cols-2">
          <HeroSnapshotCard
            to="/portfolio"
            icon={BarChart3}
            page="Portfolio"
            label="Return"
            value={portfolioReturn}
            detail={`${positions.length} positions | ${portfolioDetail}`}
            tone={
              (summary?.gain_loss_percentage ?? 0) > 0
                ? "emerald"
                : (summary?.gain_loss_percentage ?? 0) < 0
                  ? "rose"
                  : "default"
            }
          />
          <HeroSnapshotCard
            to="/leaderboard"
            icon={Medal}
            page="Leaderboard"
            label="Standing"
            value={leaderboardValue}
            detail={leaderboardDetail}
            tone="amber"
          />
          <HeroSnapshotCard
            to="/profile"
            icon={Eye}
            page="Profile"
            label="Visibility"
            value={profileValue}
            detail={profileDetail}
          />
          <HeroSnapshotCard
            to="/explore"
            icon={Compass}
            page="Explore"
            label="Market signal"
            value={exploreValue}
            detail={exploreDetail}
            tone="violet"
          />
        </div>
      </div>
    </section>
  );
}

function HeroSnapshotCard({
  to,
  icon: Icon,
  page,
  label,
  value,
  detail,
  tone = "default",
}: {
  to: string;
  icon: LucideIcon;
  page: string;
  label: string;
  value: string;
  detail: string;
  tone?: Tone;
}) {
  return (
    <Link
      to={to}
      className="rounded-xl border border-zinc-800 bg-zinc-950/35 p-3.5 transition hover:border-zinc-700 hover:bg-zinc-950/55 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-500"
    >
      <div className="flex items-center justify-between gap-3">
        <span className="text-[11px] font-medium uppercase tracking-[0.14em] text-zinc-600">
          {page}
        </span>
        <Icon className={cn("h-4 w-4", toneClass(tone))} />
      </div>
      <p className="mt-3 text-xs font-medium text-zinc-500">{label}</p>
      <p className={cn("mt-1 truncate font-mono text-base font-semibold tabular-nums", toneClass(tone))}>
        {value}
      </p>
      <p className="mt-1 line-clamp-2 text-xs leading-5 text-zinc-500">{detail}</p>
    </Link>
  );
}

function NextActionCard({
  positions,
  profile,
  leaderboardMe,
}: {
  positions: Position[];
  profile: MyProfile | null;
  leaderboardMe: LeaderboardMe | null;
}) {
  const next =
    positions.length === 0
      ? {
          to: "/portfolio",
          title: "Create your strategy baseline",
          body: "Add your current holdings to start ranked performance.",
          cta: "Go to Portfolio",
        }
      : profile && !profile.is_public
        ? {
            to: "/profile",
            title: "Control your public presence",
            body: "Make your profile public only when you are ready.",
            cta: "Edit Profile",
          }
        : leaderboardMe?.rank
          ? {
              to: "/leaderboard",
              title: "Check your rank",
              body: "See your current leaderboard standing and achievements.",
              cta: "Open Leaderboard",
            }
          : {
              to: "/explore",
              title: "Compare with public strategies",
              body: "Discover visible weights and top performers.",
              cta: "Open Explore",
            };

  return (
    <Link
      to={next.to}
      className="block rounded-2xl border border-zinc-800 bg-zinc-900/45 p-4 transition hover:border-zinc-700 hover:bg-zinc-900/65 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-500"
    >
      <p className="text-xs font-medium uppercase tracking-[0.14em] text-zinc-600">
        Next best step
      </p>
      <h2 className="mt-2.5 text-base font-semibold text-zinc-100">{next.title}</h2>
      <p className="mt-2 text-sm leading-6 text-zinc-500">{next.body}</p>
      <span className="mt-4 inline-flex items-center gap-1 text-xs font-medium text-zinc-300">
        {next.cta} <ArrowRight className="h-3.5 w-3.5" />
      </span>
    </Link>
  );
}

function PrivateSnapshot({
  summary,
}: {
  summary: DashboardPortfolioSummary | null;
}) {
  if (!summary) return null;
  return (
    <div className="rounded-2xl border border-zinc-800 bg-zinc-900/35 p-4">
      <p className="text-xs font-medium uppercase tracking-[0.14em] text-zinc-600">
        Private snapshot
      </p>
      <div className="mt-3.5 space-y-3">
        <SnapshotRow label="Value" value={formatMoney(summary.current_value, summary.base_currency ?? "USD")} />
        <SnapshotRow label="Return" value={formatPercent(summary.gain_loss_percentage)} />
        <SnapshotRow label="Index" value={summary.portfolio_index.toFixed(2)} />
      </div>
      <p className="mt-3.5 text-xs text-zinc-600">Owner-only. Not shown on public profiles.</p>
    </div>
  );
}

function MarketDataStatus({ status }: { status?: QuoteStatus }) {
  return (
    <div className="rounded-2xl border border-zinc-800 bg-zinc-900/35 p-4">
      <div className="flex items-center justify-between gap-3">
        <p className="text-xs font-medium uppercase tracking-[0.14em] text-zinc-600">
          Market data
        </p>
        <QuoteFreshnessBadge
          provider={status?.provider}
          status={status?.provider_status}
          isStale={(status?.stale_count ?? 0) > 0}
          fetchedAt={status?.last_fetched_at}
        />
      </div>
      <div className="mt-3 grid grid-cols-2 gap-3">
        <SnapshotRow label="Provider" value={status?.provider ?? "mock"} />
        <SnapshotRow
          label="Stale"
          value={`${status?.stale_count ?? 0} / ${status?.total_quotes ?? 0}`}
        />
      </div>
    </div>
  );
}

function SnapshotRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-3">
      <span className="text-sm text-zinc-500">{label}</span>
      <span className="font-mono text-sm tabular-nums text-zinc-200">{value}</span>
    </div>
  );
}

function toneClass(tone: Tone) {
  switch (tone) {
    case "emerald":
      return "text-emerald-300";
    case "rose":
      return "text-rose-300";
    case "amber":
      return "text-amber-300";
    case "violet":
      return "text-violet-300";
    default:
      return "text-zinc-300";
  }
}
