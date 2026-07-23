import type { LucideIcon } from "lucide-react";
import {
  Banknote,
  Binary,
  BookOpenCheck,
  Brain,
  Building2,
  ChartNoAxesCombined,
  Eye,
  Gem,
  Globe2,
  GraduationCap,
  HandCoins,
  Landmark,
  Orbit,
  Pickaxe,
  PieChart,
  Rocket,
  Scale,
  ShieldCheck,
  Telescope,
  Umbrella,
} from "lucide-react";

import { cn } from "@/utils/cn";

type MarkStyle = {
  Icon: LucideIcon;
  tone: string;
};

const MARKS: Record<string, MarkStyle> = {
  cash_plus_30d: {
    Icon: Banknote,
    tone: "border-emerald-300/20 bg-emerald-300/[0.07] text-emerald-300",
  },
  first_market_edge_30d: {
    Icon: ChartNoAxesCombined,
    tone: "border-sky-300/20 bg-sky-300/[0.07] text-sky-300",
  },
  gold_check_30d: {
    Icon: Gem,
    tone: "border-amber-300/20 bg-amber-300/[0.07] text-amber-300",
  },
  balanced_start_30d: {
    Icon: Scale,
    tone: "border-teal-300/20 bg-teal-300/[0.07] text-teal-300",
  },
  bogle_badge_90d: {
    Icon: Landmark,
    tone: "border-blue-300/20 bg-blue-300/[0.07] text-blue-300",
  },
  global_allocator_90d: {
    Icon: Globe2,
    tone: "border-cyan-300/20 bg-cyan-300/[0.07] text-cyan-300",
  },
  dividend_challenger_90d: {
    Icon: HandCoins,
    tone: "border-lime-300/20 bg-lime-300/[0.07] text-lime-300",
  },
  balanced_beater_90d: {
    Icon: PieChart,
    tone: "border-indigo-300/20 bg-indigo-300/[0.07] text-indigo-300",
  },
  inflation_shield_90d: {
    Icon: ShieldCheck,
    tone: "border-orange-300/20 bg-orange-300/[0.07] text-orange-300",
  },
  commodity_edge_90d: {
    Icon: Pickaxe,
    tone: "border-yellow-300/20 bg-yellow-300/[0.07] text-yellow-300",
  },
  oracle_badge_6m: {
    Icon: Eye,
    tone: "border-violet-300/20 bg-violet-300/[0.08] text-violet-300",
  },
  buffett_portfolio_6m: {
    Icon: Building2,
    tone: "border-amber-300/20 bg-amber-300/[0.08] text-amber-300",
  },
  all_weather_6m: {
    Icon: Umbrella,
    tone: "border-sky-300/20 bg-sky-300/[0.08] text-sky-300",
  },
  munger_6m: {
    Icon: Brain,
    tone: "border-fuchsia-300/20 bg-fuchsia-300/[0.08] text-fuchsia-300",
  },
  graham_6m: {
    Icon: BookOpenCheck,
    tone: "border-rose-300/20 bg-rose-300/[0.08] text-rose-300",
  },
  lynch_6m: {
    Icon: Rocket,
    tone: "border-red-300/20 bg-red-300/[0.08] text-red-300",
  },
  swensen_6m: {
    Icon: GraduationCap,
    tone: "border-teal-300/20 bg-teal-300/[0.08] text-teal-300",
  },
  soros_1y: {
    Icon: Orbit,
    tone: "border-purple-300/20 bg-purple-300/[0.08] text-purple-300",
  },
  quant_1y: {
    Icon: Binary,
    tone: "border-cyan-300/20 bg-cyan-300/[0.08] text-cyan-300",
  },
  druckenmiller_1y: {
    Icon: Telescope,
    tone: "border-pink-300/20 bg-pink-300/[0.08] text-pink-300",
  },
};

const FALLBACK: MarkStyle = {
  Icon: ChartNoAxesCombined,
  tone: "border-zinc-700 bg-zinc-900 text-zinc-400",
};

export function BadgeMark({
  badgeId,
  className,
  iconClassName,
}: {
  badgeId: string;
  className?: string;
  iconClassName?: string;
}) {
  const { Icon, tone } = MARKS[badgeId] ?? FALLBACK;

  return (
    <span
      className={cn(
        "grid h-10 w-10 shrink-0 place-items-center rounded-xl border shadow-sm shadow-black/10",
        tone,
        className,
      )}
      aria-hidden="true"
    >
      <Icon className={cn("h-[18px] w-[18px]", iconClassName)} />
    </span>
  );
}
