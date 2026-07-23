import type { LucideIcon } from "lucide-react";
import {
  Activity,
  ArrowRight,
  BadgeCheck,
  BarChart3,
  Copy,
  GitCompare,
  LockKeyhole,
  Medal,
  Percent,
  Radar,
  Search,
  ShieldCheck,
  Sparkles,
  Timer,
  Trophy,
  Users,
} from "lucide-react";
import { Link } from "react-router-dom";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { cn } from "@/utils/cn";

const leaderboardRows = [
  {
    rank: "#1",
    name: "Atlas Momentum",
    tag: "Momentum",
    index: "128.40",
    returnValue: "+28.40%",
    badge: "Sprint Leader",
    weights: "NVDA 24% / QQQ 19%",
  },
  {
    rank: "#2",
    name: "Northstar Balanced",
    tag: "Balanced",
    index: "119.75",
    returnValue: "+19.75%",
    badge: "Steady Climber",
    weights: "SPY 42% / MSFT 16%",
  },
  {
    rank: "#3",
    name: "River Tech Growth",
    tag: "Tech Growth",
    index: "114.20",
    returnValue: "+14.20%",
    badge: "Top Performer",
    weights: "AAPL 21% / NVDA 18%",
  },
  {
    rank: "#18",
    name: "You",
    tag: "Core Index",
    index: "104.80",
    returnValue: "+4.80%",
    badge: "Baseline Builder",
    weights: "SPY 65% / QQQ 20%",
  },
];

const statCards = [
  { label: "Portfolio Index", value: "104.80", icon: Activity },
  { label: "Rank", value: "#18", icon: Trophy },
  { label: "Achievement", value: "Baseline Builder", icon: BadgeCheck },
  { label: "Strategy", value: "Core Index", icon: Radar },
];

const competeCards = [
  {
    title: "Ranked leaderboard",
    copy: "Compete by percentage performance, not account size.",
    icon: Trophy,
    tone: "text-amber-300",
  },
  {
    title: "Strategy tags",
    copy: "Frame your baseline with a simple public strategy label.",
    icon: Radar,
    tone: "text-violet-300",
  },
  {
    title: "Timeframes",
    copy: "Track how strategy rankings develop across snapshots.",
    icon: Timer,
    tone: "text-zinc-300",
  },
  {
    title: "Achievements",
    copy: "Earn compact badges for portfolio milestones.",
    icon: Medal,
    tone: "text-amber-300",
  },
];

const exploreCards = [
  {
    title: "Top performers",
    copy: "Find public strategies climbing by ranked return.",
    icon: Trophy,
  },
  {
    title: "Trending holdings",
    copy: "See symbols appearing in public percentage weights.",
    icon: BarChart3,
  },
  {
    title: "Public percentage weights",
    copy: "Review symbols and allocations only, never quantities.",
    icon: Percent,
  },
  {
    title: "Similar strategies",
    copy: "Discover profiles that rhyme with your baseline.",
    icon: Users,
  },
];

const copyCards = [
  {
    title: "Compare",
    copy: "Put your baseline beside a public strategy profile.",
    icon: GitCompare,
  },
  {
    title: "Preview",
    copy: "Review percentage weights before creating anything.",
    icon: Search,
  },
  {
    title: "Copy safely",
    copy: "Create your own fresh baseline. Nothing is traded.",
    icon: Copy,
  },
];

const privateCards = [
  {
    title: "Private portfolio",
    copy: "Your full positions stay inside authenticated owner views.",
    icon: LockKeyhole,
  },
  {
    title: "Portfolio index",
    copy: "Track performance from a simple strategy baseline.",
    icon: Activity,
  },
  {
    title: "Market data status",
    copy: "See quote freshness without exposing provider keys.",
    icon: ShieldCheck,
  },
];

function FeatureCard({
  title,
  copy,
  icon: Icon,
  tone = "text-zinc-300",
}: {
  title: string;
  copy: string;
  icon: LucideIcon;
  tone?: string;
}) {
  return (
    <article className="rounded-lg border border-zinc-800 bg-zinc-900/35 p-5 shadow-sm shadow-black/20">
      <div className="mb-4 grid h-9 w-9 place-items-center rounded-lg border border-zinc-800 bg-zinc-950/70">
        <Icon className={cn("h-4 w-4", tone)} />
      </div>
      <h3 className="text-sm font-semibold text-zinc-100">{title}</h3>
      <p className="mt-2 text-sm leading-6 text-zinc-400">{copy}</p>
    </article>
  );
}

function SectionHeader({
  eyebrow,
  title,
  copy,
}: {
  eyebrow: string;
  title: string;
  copy?: string;
}) {
  return (
    <div className="max-w-2xl">
      <p className="text-xs font-medium uppercase tracking-[0.24em] text-zinc-500">
        {eyebrow}
      </p>
      <h2 className="mt-3 text-2xl font-semibold tracking-tight text-zinc-50 sm:text-3xl">
        {title}
      </h2>
      {copy && <p className="mt-3 text-sm leading-6 text-zinc-400">{copy}</p>}
    </div>
  );
}

function LeaderboardPreview() {
  return (
    <div className="rounded-lg border border-zinc-800 bg-zinc-950/80 shadow-2xl shadow-black/30">
      <div className="flex items-center justify-between border-b border-zinc-800 px-4 py-3 sm:px-5">
        <div>
          <p className="text-xs uppercase tracking-[0.22em] text-zinc-500">
            Ranked Strategies
          </p>
          <h2 className="mt-1 text-sm font-semibold text-zinc-100">
            Live strategy board
          </h2>
        </div>
        <Badge className="border-amber-400/20 bg-amber-400/10 text-amber-200">
          Prototype 3
        </Badge>
      </div>

      <div className="hidden sm:block">
        <table className="w-full table-fixed text-left text-sm">
          <thead className="border-b border-zinc-800 text-xs uppercase tracking-wider text-zinc-500">
            <tr>
              <th className="w-16 px-5 py-3 font-medium">Rank</th>
              <th className="px-3 py-3 font-medium">Strategy</th>
              <th className="w-24 px-3 py-3 text-right font-medium">Index</th>
              <th className="w-24 px-5 py-3 text-right font-medium">Return</th>
            </tr>
          </thead>
          <tbody>
            {leaderboardRows.map((row) => (
              <tr
                key={row.rank}
                className={cn(
                  "border-b border-zinc-900 last:border-0",
                  row.name === "You" && "bg-zinc-900/55",
                )}
              >
                <td className="px-5 py-4 font-mono text-sm text-amber-200">
                  {row.rank}
                </td>
                <td className="min-w-0 px-3 py-4">
                  <div className="truncate font-medium text-zinc-100">
                    {row.name}
                  </div>
                  <div className="mt-1 flex min-w-0 flex-wrap items-center gap-2 text-xs text-zinc-500">
                    <span>{row.tag}</span>
                    <span className="h-1 w-1 rounded-full bg-zinc-700" />
                    <span className="truncate text-violet-300/80">
                      {row.badge}
                    </span>
                  </div>
                </td>
                <td className="px-3 py-4 text-right font-mono tabular-nums text-zinc-100">
                  {row.index}
                </td>
                <td className="px-5 py-4 text-right font-mono tabular-nums text-emerald-300">
                  {row.returnValue}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <div className="space-y-3 p-3 sm:hidden">
        {leaderboardRows.map((row) => (
          <div
            key={row.rank}
            className={cn(
              "rounded-lg border border-zinc-800 bg-zinc-900/35 p-4",
              row.name === "You" && "border-amber-300/25 bg-amber-300/5",
            )}
          >
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <div className="font-mono text-sm text-amber-200">{row.rank}</div>
                <div className="mt-1 truncate font-medium text-zinc-100">
                  {row.name}
                </div>
                <div className="mt-1 text-xs text-zinc-500">{row.tag}</div>
              </div>
              <div className="text-right">
                <div className="font-mono text-sm tabular-nums text-zinc-100">
                  {row.index}
                </div>
                <div className="mt-1 font-mono text-xs tabular-nums text-emerald-300">
                  {row.returnValue}
                </div>
              </div>
            </div>
            <div className="mt-3 flex flex-wrap gap-2 text-xs text-zinc-500">
              <span className="rounded-md border border-violet-400/15 bg-violet-400/10 px-2 py-1 text-violet-200">
                {row.badge}
              </span>
              <span className="rounded-md border border-zinc-800 bg-zinc-950/60 px-2 py-1">
                {row.weights}
              </span>
            </div>
          </div>
        ))}
      </div>

      <div className="grid grid-cols-2 gap-3 border-t border-zinc-800 p-3 sm:grid-cols-4">
        {statCards.map(({ label, value, icon: Icon }) => (
          <div
            key={label}
            className="rounded-lg border border-zinc-800 bg-zinc-900/35 p-3"
          >
            <Icon className="h-4 w-4 text-zinc-500" />
            <div className="mt-3 text-[11px] uppercase tracking-wider text-zinc-500">
              {label}
            </div>
            <div className="mt-1 truncate font-mono text-sm font-semibold tabular-nums text-zinc-100">
              {value}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

export function LandingPage() {
  return (
    <div className="min-h-screen overflow-x-hidden bg-zinc-950 text-zinc-50">
      <div
        className="pointer-events-none fixed inset-0 opacity-30"
        style={{
          backgroundImage:
            "linear-gradient(rgba(255,255,255,0.035) 1px, transparent 1px), linear-gradient(90deg, rgba(255,255,255,0.035) 1px, transparent 1px)",
          backgroundSize: "48px 48px",
        }}
      />

      <header className="sticky top-0 z-20 border-b border-zinc-900 bg-zinc-950/85 backdrop-blur">
        <div className="mx-auto flex max-w-7xl items-center justify-between gap-4 px-4 py-3 sm:px-6 lg:px-8">
          <a href="#arena" className="flex items-center gap-2.5">
            <div className="grid h-8 w-8 place-items-center rounded-lg border border-zinc-800 bg-zinc-900/70">
              <Trophy className="h-4 w-4 text-amber-300" />
            </div>
            <span className="brand-wordmark text-lg font-semibold text-zinc-100">
              Alarvest
            </span>
          </a>

          <nav className="hidden items-center gap-5 text-sm text-zinc-400 md:flex">
            <a className="hover:text-zinc-100" href="#leaderboard">
              Leaderboard
            </a>
            <a className="hover:text-zinc-100" href="#explore">
              Explore
            </a>
            <a className="hover:text-zinc-100" href="#how-it-works">
              How it works
            </a>
          </nav>

          <div className="flex items-center gap-2">
            <Button asChild variant="ghost" size="sm">
              <Link to="/login">Sign in</Link>
            </Button>
            <Button asChild size="sm">
              <Link to="/register">Create account</Link>
            </Button>
          </div>
        </div>
      </header>

      <main className="relative z-10">
        <section
          id="arena"
          className="mx-auto grid max-w-7xl gap-10 px-4 pb-16 pt-14 sm:px-6 sm:pt-20 lg:grid-cols-[minmax(0,0.9fr)_minmax(420px,1.1fr)] lg:items-center lg:px-8 lg:pb-24"
        >
          <div>
            <Badge className="border-zinc-800 bg-zinc-900/70 text-zinc-300">
              Real portfolio strategy ranking
            </Badge>
            <p className="brand-wordmark mt-6 border-b border-zinc-800 pb-4 text-6xl font-semibold text-zinc-50 sm:text-8xl">
              Alarvest
            </p>
            <h1 className="mt-7 max-w-3xl text-4xl font-semibold tracking-tight text-zinc-50 sm:text-6xl">
              Track your portfolio. Rank your strategy.
            </h1>
            <p className="mt-5 max-w-2xl text-base leading-7 text-zinc-400 sm:text-lg">
              Build a real portfolio baseline, compete by percentage
              performance, discover public strategies, and climb the leaderboard
              without turning investing into a trading game.
            </p>

            <div className="mt-8 flex flex-col gap-3 sm:flex-row">
              <Button asChild size="lg" className="h-12">
                <Link to="/register">
                  Create account
                  <ArrowRight className="h-4 w-4" />
                </Link>
              </Button>
              <Button asChild variant="outline" size="lg" className="h-12">
                <Link to="/login">Sign in</Link>
              </Button>
            </div>

            <p className="mt-5 text-sm text-zinc-500">
              No brokerage connection. No trades. No wealth rankings.
            </p>
          </div>

          <LeaderboardPreview />
        </section>

        <section
          id="leaderboard"
          className="border-y border-zinc-900 bg-zinc-950/80 py-16"
        >
          <div className="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
            <SectionHeader
              eyebrow="Leaderboard"
              title="Compete on strategy, not wealth"
            />
            <div className="mt-8 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
              {competeCards.map((card) => (
                <FeatureCard key={card.title} {...card} />
              ))}
            </div>
          </div>
        </section>

        <section id="explore" className="py-16">
          <div className="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
            <SectionHeader
              eyebrow="Explore"
              title="Explore public strategies"
            />
            <div className="mt-8 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
              {exploreCards.map((card) => (
                <FeatureCard key={card.title} {...card} />
              ))}
            </div>
          </div>
        </section>

        <section
          id="how-it-works"
          className="border-y border-zinc-900 bg-zinc-950/80 py-16"
        >
          <div className="mx-auto grid max-w-7xl gap-8 px-4 sm:px-6 lg:grid-cols-[0.8fr_1.2fr] lg:px-8">
            <SectionHeader
              eyebrow="Copy flow"
              title="Copy ideas, not trades"
              copy="Preview public strategy weights and create your own fresh baseline. Nothing is traded. Nothing is copied by value."
            />
            <div className="grid gap-4 sm:grid-cols-3">
              {copyCards.map((card) => (
                <FeatureCard key={card.title} {...card} />
              ))}
            </div>
          </div>
        </section>

        <section className="py-16">
          <div className="mx-auto grid max-w-7xl gap-8 px-4 sm:px-6 lg:grid-cols-[0.8fr_1.2fr] lg:px-8">
            <SectionHeader
              eyebrow="Private workspace"
              title="Your strategy command center"
              copy="Track your full portfolio privately, then choose what your public strategy profile reveals."
            />
            <div className="grid gap-4 sm:grid-cols-3">
              {privateCards.map((card) => (
                <FeatureCard key={card.title} {...card} />
              ))}
            </div>
          </div>
        </section>

        <section className="px-4 py-16 sm:px-6 lg:px-8">
          <div className="mx-auto max-w-3xl text-center">
            <Sparkles className="mx-auto h-5 w-5 text-violet-300" />
            <h2 className="mt-4 text-3xl font-semibold tracking-tight text-zinc-50 sm:text-4xl">
              Create your baseline. Enter the leaderboard.
            </h2>
            <div className="mt-8 flex flex-col justify-center gap-3 sm:flex-row">
              <Button asChild size="lg" className="h-12">
                <Link to="/register">Create account</Link>
              </Button>
              <Button asChild variant="outline" size="lg" className="h-12">
                <Link to="/login">Sign in</Link>
              </Button>
            </div>
          </div>
        </section>
      </main>
    </div>
  );
}
