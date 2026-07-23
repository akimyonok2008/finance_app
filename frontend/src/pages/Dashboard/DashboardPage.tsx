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
import { useDashboard } from "@/hooks/useDashboard";
import { useExplore } from "@/hooks/useExplore";
import { useMyProfile } from "@/hooks/useProfile";
import { usePositions } from "@/hooks/usePositions";
import { PerformanceChartCard } from "@/pages/Dashboard/components/PerformanceChartCard";
import { formatRank } from "@/pages/Dashboard/utils/dashboardFormatters";
import type { LeaderboardMe } from "@/types/dashboard";
import type { ExploreResponse } from "@/types/explore";
import type { MyProfile } from "@/types/profile";
import type { Position } from "@/types/portfolio";
import { cn } from "@/utils/cn";

type Tone = "cyan" | "amber" | "violet" | "teal";

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
    <div className="dashboard-shell min-h-screen bg-[radial-gradient(circle_at_50%_8%,rgba(99,102,241,0.055),transparent_28%),radial-gradient(circle_at_10%_55%,rgba(34,211,238,0.035),transparent_24%),#09090b] text-zinc-50">
      <main className="mx-auto w-full max-w-6xl px-4 pb-20 pt-4 sm:px-6 lg:px-8">
        <AppNav
          actions={
            <button
              type="button"
              onClick={handleRefresh}
              disabled={isRefreshing}
              aria-label="Refresh dashboard"
              className="rounded-lg p-2 text-zinc-400 transition hover:bg-zinc-800/70 hover:text-zinc-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-indigo-300/40 disabled:opacity-50"
            >
              <RefreshCw
                className={`h-3.5 w-3.5 ${isRefreshing ? "animate-spin" : ""}`}
              />
            </button>
          }
        />

        <header className="mb-6 text-center">
          <p className="text-[11px] font-semibold uppercase tracking-[0.2em] text-indigo-200/60">
            Personal overview
          </p>
          <h1 className="dashboard-display mt-2 text-3xl font-semibold tracking-tight sm:text-4xl">
            {user?.display_name ? `${user.display_name}'s dashboard` : "Your dashboard"}
          </h1>
          <p className="mx-auto mt-2 max-w-xl text-sm leading-6 text-zinc-400">
            One clear view of your strategy, portfolio, and public presence.
          </p>
        </header>

        <PerformanceChartCard
          summary={portfolioSummary}
          isLoading={dashboard.isLoading}
          isError={!!dashboard.errors.summary}
          onRetry={dashboard.refetch.summary}
        />

        <div className="mt-5">
          <StrategyCommandCenter
            positions={positions}
            leaderboardMe={leaderboardMe}
            profile={profile}
            explore={explore}
          />
        </div>
      </main>
    </div>
  );
}

function StrategyCommandCenter({
  positions,
  leaderboardMe,
  profile,
  explore,
}: {
  positions: Position[];
  leaderboardMe: LeaderboardMe | null;
  profile: MyProfile | null;
  explore: ExploreResponse | null;
}) {
  const trend = explore?.trending_holdings?.[0];
  const topPerformer = explore?.top_performers?.[0];

  return (
    <section className="rounded-2xl border border-zinc-800 bg-zinc-900/35 p-4 shadow-lg shadow-black/10 sm:p-5">
      <div className="flex items-start justify-between gap-4">
        <div>
          <p className="text-[10px] font-semibold uppercase tracking-[0.18em] text-zinc-600">
            Navigation
          </p>
          <h2 className="mt-1.5 text-base font-semibold tracking-tight text-zinc-100">
            Strategy command center
          </h2>
          <p className="mt-1 text-xs leading-5 text-zinc-500">
            Continue into the part of your strategy that needs attention.
          </p>
        </div>
        <span className="rounded-full border border-zinc-800 bg-zinc-950/40 px-2.5 py-1 text-[10px] text-zinc-500">
          Quick access
        </span>
      </div>

      <div className="mt-4 grid gap-3 sm:grid-cols-2">
        <CommandLink
          to="/portfolio"
          icon={BarChart3}
          label="Portfolio"
          value={`${positions.length} active ${positions.length === 1 ? "position" : "positions"}`}
          detail="Manage holdings and portfolio history"
          tone="cyan"
        />
        <CommandLink
          to="/leaderboard"
          icon={Medal}
          label="Leaderboard"
          value={leaderboardMe?.rank ? formatRank(leaderboardMe.rank) : "Not ranked"}
          detail={
            leaderboardMe?.total_participants
              ? `Among ${leaderboardMe.total_participants} strategies`
              : "Your standing appears after a baseline"
          }
          tone="amber"
        />
        <CommandLink
          to="/profile"
          icon={Eye}
          label="Profile"
          value={profile?.is_public ? "Public" : "Private"}
          detail={
            profile?.show_public_weights
              ? "Allocation weights are visible"
              : "Portfolio composition stays private"
          }
          tone="violet"
        />
        <CommandLink
          to="/explore"
          icon={Compass}
          label="Explore"
          value={trend?.symbol ?? topPerformer?.display_name ?? "Discover"}
          detail="Browse public strategies and ideas"
          tone="teal"
        />
      </div>
    </section>
  );
}

function CommandLink({
  to,
  icon: Icon,
  label,
  value,
  detail,
  tone,
}: {
  to: string;
  icon: LucideIcon;
  label: string;
  value: string;
  detail: string;
  tone: Tone;
}) {
  return (
    <Link
      to={to}
      className={cn(
        "group relative overflow-hidden rounded-xl border bg-zinc-950/40 p-4 transition hover:-translate-y-0.5 hover:bg-zinc-950/60 focus-visible:outline-none focus-visible:ring-2",
        commandTone(tone),
      )}
    >
      <div className="flex items-center justify-between gap-3">
        <span className="flex items-center gap-2 text-xs font-semibold text-zinc-300">
          <Icon className={cn("h-3.5 w-3.5", iconTone(tone))} /> {label}
        </span>
        <ArrowRight className="h-3.5 w-3.5 text-zinc-700 transition group-hover:translate-x-0.5 group-hover:text-zinc-400" />
      </div>
      <p className="mt-4 truncate text-[15px] font-semibold tracking-tight text-zinc-100">
        {value}
      </p>
      <p className="mt-1 text-xs leading-5 text-zinc-500">{detail}</p>
    </Link>
  );
}

function commandTone(tone: Tone) {
  switch (tone) {
    case "cyan":
      return "border-cyan-300/15 bg-[radial-gradient(circle_at_0%_0%,rgba(34,211,238,0.09),transparent_55%)] text-cyan-200/70 hover:border-cyan-300/25 focus-visible:ring-cyan-300/30";
    case "amber":
      return "border-amber-300/15 bg-[radial-gradient(circle_at_0%_0%,rgba(251,191,36,0.09),transparent_55%)] text-amber-200/70 hover:border-amber-300/25 focus-visible:ring-amber-300/30";
    case "violet":
      return "border-violet-300/15 bg-[radial-gradient(circle_at_0%_0%,rgba(167,139,250,0.09),transparent_55%)] text-violet-200/70 hover:border-violet-300/25 focus-visible:ring-violet-300/30";
    case "teal":
      return "border-teal-300/15 bg-[radial-gradient(circle_at_0%_0%,rgba(45,212,191,0.09),transparent_55%)] text-teal-200/70 hover:border-teal-300/25 focus-visible:ring-teal-300/30";
  }
}

function iconTone(tone: Tone) {
  switch (tone) {
    case "cyan":
      return "text-cyan-300/75";
    case "amber":
      return "text-amber-300/75";
    case "violet":
      return "text-violet-300/75";
    case "teal":
      return "text-teal-300/75";
  }
}
