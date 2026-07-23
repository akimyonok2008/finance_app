import { motion } from "framer-motion";
import { Activity, Coins, TrendingUp, Wallet } from "lucide-react";
import type { LucideIcon } from "lucide-react";

import { Card } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { usePortfolioSummary } from "@/hooks/usePortfolioSummary";
import { cn } from "@/utils/cn";
import { formatMoney } from "@/utils/formatMoney";
import { formatPercent } from "@/utils/formatPercent";
import { gainLossColor } from "@/utils/gainLoss";

type CardSpec = {
  key: string;
  label: string;
  value: string;
  icon: LucideIcon;
  valueClassName?: string;
  hint?: string;
  tone: "cyan" | "emerald" | "rose" | "violet" | "amber";
};

function SummaryCard({ spec, index }: { spec: CardSpec; index: number }) {
  const Icon = spec.icon;
  return (
    <motion.div
      initial={{ opacity: 0, y: 12 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.35, delay: index * 0.05 }}
    >
      <Card
        className={cn(
          "relative overflow-hidden p-5 transition duration-200 hover:-translate-y-0.5",
          spec.tone === "cyan" && "border-cyan-300/15 bg-[radial-gradient(circle_at_top_right,rgba(34,211,238,0.11),transparent_48%),rgba(24,24,27,0.52)] hover:border-cyan-300/25",
          spec.tone === "emerald" && "border-emerald-300/15 bg-[radial-gradient(circle_at_top_right,rgba(52,211,153,0.1),transparent_48%),rgba(24,24,27,0.52)] hover:border-emerald-300/25",
          spec.tone === "rose" && "border-rose-300/15 bg-[radial-gradient(circle_at_top_right,rgba(251,113,133,0.1),transparent_48%),rgba(24,24,27,0.52)] hover:border-rose-300/25",
          spec.tone === "violet" && "border-violet-300/15 bg-[radial-gradient(circle_at_top_right,rgba(167,139,250,0.11),transparent_48%),rgba(24,24,27,0.52)] hover:border-violet-300/25",
          spec.tone === "amber" && "border-amber-300/15 bg-[radial-gradient(circle_at_top_right,rgba(251,191,36,0.1),transparent_48%),rgba(24,24,27,0.52)] hover:border-amber-300/25",
        )}
      >
        <div className="flex items-center justify-between">
          <span className="text-[10px] font-semibold uppercase tracking-[0.16em] text-zinc-500">
            {spec.label}
          </span>
          <span
            className={cn(
              "grid h-9 w-9 place-items-center rounded-xl border bg-zinc-950/35",
              spec.tone === "cyan" && "border-cyan-300/15 text-cyan-300/80",
              spec.tone === "emerald" && "border-emerald-300/15 text-emerald-300/80",
              spec.tone === "rose" && "border-rose-300/15 text-rose-300/80",
              spec.tone === "violet" && "border-violet-300/15 text-violet-300/80",
              spec.tone === "amber" && "border-amber-300/15 text-amber-300/80",
            )}
          >
            <Icon className="h-4 w-4" />
          </span>
        </div>
        <div
          className={cn(
            "mt-4 font-mono text-2xl font-medium tabular-nums tracking-tight",
            spec.valueClassName,
          )}
        >
          {spec.value}
        </div>
        {spec.hint && (
          <p className="mt-1 text-xs text-muted-foreground">{spec.hint}</p>
        )}
      </Card>
    </motion.div>
  );
}

function SummarySkeletons() {
  return (
    <>
      {Array.from({ length: 4 }).map((_, i) => (
        <Card key={i} className="p-5">
          <div className="flex items-center justify-between">
            <Skeleton className="h-3 w-24" />
            <Skeleton className="h-8 w-8 rounded-lg" />
          </div>
          <Skeleton className="mt-5 h-7 w-28" />
          <Skeleton className="mt-2 h-3 w-16" />
        </Card>
      ))}
    </>
  );
}

export function PortfolioSummaryCards() {
  const { data, isLoading, isError } = usePortfolioSummary();

  const grid = "grid grid-cols-2 gap-4 lg:grid-cols-4";

  if (isLoading) {
    return (
      <div className={grid}>
        <SummarySkeletons />
      </div>
    );
  }

  // On error (or no summary yet) render neutral placeholders rather than crash.
  const summary = data;
  const currency = summary?.base_currency ?? "USD";
  const gainPct = summary?.gain_loss_percentage;

  const specs: CardSpec[] = [
    {
      key: "index",
      label: "Portfolio Index",
      value:
        summary?.portfolio_index !== undefined
          ? summary.portfolio_index.toFixed(2)
          : "—",
      icon: Activity,
      valueClassName: "text-cyan-100",
      hint: "Baseline 100.00",
      tone: "cyan",
    },
    {
      key: "gain",
      label: "Gain / Loss",
      value: formatPercent(gainPct),
      icon: TrendingUp,
      valueClassName: gainLossColor(gainPct),
      hint:
        summary?.gain_loss !== undefined
          ? formatMoney(summary.gain_loss, currency)
          : undefined,
      tone: (gainPct ?? 0) < 0 ? "rose" : "emerald",
    },
    {
      key: "value",
      label: "Current Value",
      value: formatMoney(summary?.current_value, currency),
      icon: Wallet,
      valueClassName: "text-violet-100",
      tone: "violet",
    },
    {
      key: "cost",
      label: "Cost Basis",
      value: formatMoney(summary?.total_cost_basis, currency),
      icon: Coins,
      valueClassName: "text-amber-100",
      tone: "amber",
    },
  ];

  return (
    <div className={grid}>
      {isError ? (
        <SummarySkeletons />
      ) : (
        specs.map((spec, i) => (
          <SummaryCard key={spec.key} spec={spec} index={i} />
        ))
      )}
    </div>
  );
}
