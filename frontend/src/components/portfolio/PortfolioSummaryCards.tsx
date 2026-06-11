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
};

function SummaryCard({ spec, index }: { spec: CardSpec; index: number }) {
  const Icon = spec.icon;
  return (
    <motion.div
      initial={{ opacity: 0, y: 12 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.35, delay: index * 0.05 }}
    >
      <Card className="p-5">
        <div className="flex items-center justify-between">
          <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
            {spec.label}
          </span>
          <span className="grid h-8 w-8 place-items-center rounded-lg border border-white/10 bg-white/[0.03] text-slate-400">
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
      valueClassName: "text-zinc-100",
      hint: "Baseline 100.00",
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
    },
    {
      key: "value",
      label: "Current Value",
      value: formatMoney(summary?.current_value, currency),
      icon: Wallet,
    },
    {
      key: "cost",
      label: "Cost Basis",
      value: formatMoney(summary?.total_cost_basis, currency),
      icon: Coins,
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
