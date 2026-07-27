import { useState } from "react";
import { ArrowRight, ChevronDown } from "lucide-react";
import { Link } from "react-router-dom";

import { groupActivities } from "@/components/portfolio/activityGrouping";
import { Card } from "@/components/ui/card";
import type { PortfolioActivity } from "@/types/portfolio";
import { cn } from "@/utils/cn";
import { formatMoney } from "@/utils/formatMoney";
import { gainLossColor } from "@/utils/gainLoss";

export function ActivityTimeline({ activities }: { activities: PortfolioActivity[] }) {
  return (
    <div className="space-y-2" data-testid="activity-timeline">
      {groupActivities(activities).map((group) => (
        <GroupedActivity key={group.key} activities={group.activities} />
      ))}
    </div>
  );
}

function GroupedActivity({ activities }: { activities: PortfolioActivity[] }) {
  const [expanded, setExpanded] = useState(false);
  const primary = pickPrimary(activities);
  const summary = summarize(activities, primary);
  const episodeID =
    primary.position_episode_id ??
    activities.find((item) => item.position_episode_id)?.position_episode_id;
  const automatic = activities.some((item) => item.origin !== "user_recorded");

  return (
    <Card className="grid grid-cols-1 gap-3 p-4 sm:grid-cols-[1fr_auto] sm:items-start">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-semibold text-zinc-100">{summary.title}</span>
          <span className="rounded-full border border-zinc-800 px-2 py-0.5 text-[10px] text-zinc-500">
            {automatic ? "Automatic" : "User recorded"}
          </span>
        </div>
        <p className="mt-1 text-xs text-zinc-500">
          {new Date(primary.occurred_at).toLocaleString()}
          {automatic ? " · No action required" : ""}
        </p>
        <div className="mt-3 space-y-1.5">
          {summary.details.map((detail) => (
            <DisplayAmount key={detail.label} {...detail} />
          ))}
        </div>
        <div className="mt-3 flex flex-wrap items-center gap-3">
          {activities.length > 1 && (
            <button
              type="button"
              aria-expanded={expanded}
              onClick={() => setExpanded((value) => !value)}
              className="inline-flex items-center gap-1 text-xs text-cyan-200/75 hover:text-cyan-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-300/40"
            >
              {expanded ? "Hide activity legs" : `Show ${activities.length} activity legs`}
              <ChevronDown className={cn("h-3 w-3 transition", expanded && "rotate-180")} />
            </button>
          )}
          {episodeID && (
            <Link
              to={`/portfolio?tab=state&episode=${encodeURIComponent(episodeID)}`}
              aria-label={`View the ${primary.symbol ?? ""} position episode this activity belongs to`}
              className="inline-flex items-center gap-1 text-xs text-zinc-400 hover:text-zinc-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-300/40"
            >
              View position episode <ArrowRight className="h-3 w-3" />
            </Link>
          )}
        </div>
        {expanded && (
          <ul
            aria-label="Immutable activity legs"
            className="mt-3 divide-y divide-zinc-800 rounded-lg border border-zinc-800 bg-zinc-950/30"
          >
            {activities.map((activity) => (
              <li key={activity.id} className="flex flex-col gap-1 px-3 py-2 text-xs sm:flex-row sm:items-center sm:justify-between">
                <span className="capitalize text-zinc-400">
                  {activity.activity_type.replaceAll("_", " ")}
                </span>
                <span className="font-mono text-zinc-500">
                  {formatMoney(activity.net_amount ?? activity.gross_amount, activity.currency)}
                </span>
              </li>
            ))}
          </ul>
        )}
      </div>
      {summary.headline && (
        <DisplayAmount {...summary.headline} className="sm:text-right" />
      )}
    </Card>
  );
}

type Tone = "positive" | "negative" | "neutral";
type AmountLine = {
  label: string;
  amount?: number;
  currency?: string;
  text?: string;
  tone: Tone;
  className?: string;
};

function DisplayAmount({ label, amount, currency, text, tone, className }: AmountLine) {
  const signed = tone === "negative" && amount !== undefined ? -Math.abs(amount) : amount;
  return (
    <p
      data-amount-tone={tone}
      className={cn(
        "text-xs",
        tone === "positive" && gainLossColor(Math.abs(amount ?? 1)),
        tone === "negative" && gainLossColor(-Math.abs(amount ?? 1)),
        tone === "neutral" && "text-zinc-400",
        className,
      )}
    >
      {label}
      {text ? ` ${text}` : ""}
      {signed !== undefined && currency ? ` ${formatMoney(signed, currency)}` : ""}
    </p>
  );
}

function pickPrimary(items: PortfolioActivity[]) {
  const priority = [
    "reinvested_dividend",
    "sell",
    "buy",
    "return_of_capital",
    "cash_dividend",
    "withdrawal",
    "deposit",
  ];
  const rank = (kind: PortfolioActivity["activity_type"]) => {
    const index = priority.indexOf(kind);
    return index === -1 ? priority.length : index;
  };
  return [...items].sort(
    (a, b) => rank(a.activity_type) - rank(b.activity_type),
  )[0];
}

function summarize(items: PortfolioActivity[], primary: PortfolioActivity) {
  const fee = sum(items.filter(isFee), (item) => item.net_amount ?? item.gross_amount);
  const funding = sum(
    items.filter((item) => item.activity_type === "deposit" && item.origin !== "user_recorded"),
    (item) => item.net_amount ?? item.gross_amount,
  );
  const buy = items.find((item) => item.activity_type === "buy");
  const sell = items.find((item) => item.activity_type === "sell");
  const reinvested = items.find((item) => item.activity_type === "reinvested_dividend");
  const currency = primary.currency;

  if (reinvested) {
    return {
      title: `${reinvested.symbol ?? "Investment"} dividend reinvested`,
      details: [
        { label: "Income", amount: reinvested.net_amount ?? reinvested.gross_amount, currency, tone: "positive" as Tone },
        { label: "Shares added", text: `${buy?.quantity ?? reinvested.quantity ?? 0}`, tone: "neutral" as Tone },
      ],
    };
  }
  if (sell) {
    const proceeds = sell.net_amount ?? Math.max(0, sell.gross_amount - fee);
    return {
      title: `Sold ${sell.quantity ?? 0} ${sell.symbol ?? ""} at ${formatMoney(sell.unit_price ?? 0, currency)}`,
      headline: { label: "Net proceeds", amount: proceeds, currency, tone: "neutral" as Tone },
      details: [
        ...(sell.realized_gain_loss_base !== undefined
          ? [{ label: "Realized P&L", amount: sell.realized_gain_loss_base, currency: "USD", tone: sell.realized_gain_loss_base >= 0 ? "positive" as Tone : "negative" as Tone }]
          : []),
        ...(fee > 0 ? [{ label: "Transaction fee", amount: fee, currency, tone: "negative" as Tone }] : []),
      ],
    };
  }
  if (buy) {
    const total = buy.gross_amount + fee;
    const existingCash = Math.max(0, total - funding);
    return {
      title: `Bought ${buy.quantity ?? 0} ${buy.symbol ?? ""} at ${formatMoney(buy.unit_price ?? 0, currency)}`,
      headline: { label: "Purchase cost", amount: total, currency, tone: "neutral" as Tone },
      details: [
        ...(funding > 0 ? [{ label: "Used existing cash", amount: existingCash, currency, tone: "neutral" as Tone }] : []),
        ...(funding > 0 ? [{ label: "Automatically funded", amount: funding, currency, tone: "neutral" as Tone }] : []),
        ...(fee > 0 ? [{ label: "Transaction fee", amount: fee, currency, tone: "negative" as Tone }] : []),
      ],
    };
  }

  const amount = primary.net_amount ?? primary.gross_amount;
  const semantic = semanticFor(primary, amount);
  return {
    title: primary.activity_type.replaceAll("_", " ").replace(/\b\w/g, (c) => c.toUpperCase()),
    headline: { label: semantic.label, amount: Math.abs(amount), currency, tone: semantic.tone },
    details:
      primary.activity_type === "return_of_capital"
        ? [{ label: "Cash received; remaining cost basis was reduced.", tone: "neutral" as Tone }]
        : [],
  };
}

function semanticFor(activity: PortfolioActivity, amount: number): { label: string; tone: Tone } {
  if (activity.activity_type === "withdrawal") return { label: "Cash flow", tone: "negative" };
  if (activity.activity_type === "deposit") {
    return {
      label: "Cash flow",
      tone: activity.origin === "user_recorded" ? "positive" : "neutral",
    };
  }
  if (isFee(activity)) return { label: "Fee", tone: "negative" };
  if (["cash_dividend", "etf_distribution", "interest_income", "return_of_capital"].includes(activity.activity_type)) {
    return { label: "Income", tone: "positive" };
  }
  if (activity.realized_gain_loss_base !== undefined) {
    return { label: "Realized P&L", tone: activity.realized_gain_loss_base >= 0 ? "positive" : "negative" };
  }
  return { label: "Amount", tone: amount < 0 ? "negative" : "neutral" };
}

function isFee(activity: PortfolioActivity) {
  return activity.activity_type.includes("fee");
}

function sum(items: PortfolioActivity[], value: (item: PortfolioActivity) => number) {
  return items.reduce((total, item) => total + Math.abs(value(item)), 0);
}
