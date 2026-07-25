import { useState } from "react";
import {
  ArrowRight,
  CircleDollarSign,
  History,
  MinusCircle,
  PlusCircle,
} from "lucide-react";
import { Link, useSearchParams } from "react-router-dom";

import { AddPositionForm } from "@/components/portfolio/AddPositionForm";
import { ClosePositionDialog } from "@/components/portfolio/ClosePositionDialog";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { useActivityList } from "@/hooks/useActivity";
import {
  useDepositCash,
  usePositions,
  useWithdrawCash,
} from "@/hooks/usePositions";
import type {
  ActivityCategory,
} from "@/types/activity";
import type { Position } from "@/types/portfolio";
import {
  CURRENCIES,
  type CurrencyCode,
  type PortfolioActivity,
} from "@/types/portfolio";
import { cn } from "@/utils/cn";
import { formatMoney } from "@/utils/formatMoney";
import { gainLossColor } from "@/utils/gainLoss";

const FILTERS: Array<{ value: ActivityCategory; label: string }> = [
  { value: "all", label: "All" },
  { value: "cash_flows", label: "Cash flows" },
  { value: "trades", label: "Trades" },
  { value: "income", label: "Income" },
  { value: "fees", label: "Fees" },
  { value: "automatic_adjustments", label: "Automatic adjustments" },
  { value: "corrections", label: "Corrections" },
];

export function ActivityTabContent() {
  const [searchParams, setSearchParams] = useSearchParams();
  const rawCategory = searchParams.get("category") as ActivityCategory | null;
  const category = FILTERS.some((item) => item.value === rawCategory)
    ? rawCategory!
    : "all";
  const symbolFilter = searchParams.get("symbol") ?? "";

  // The archive is opened on demand — a deep link that already carries a
  // filter (e.g. from a "View contributing activities" link) opens it
  // automatically instead of requiring an extra click.
  const [archiveOpen, setArchiveOpen] = useState(
    () => searchParams.has("category") || searchParams.has("symbol"),
  );

  const activity = useActivityList(
    { category, symbol: symbolFilter || undefined, limit: 100 },
    archiveOpen,
  );

  const setCategory = (value: ActivityCategory) => {
    const next = new URLSearchParams(searchParams);
    if (value === "all") next.delete("category");
    else next.set("category", value);
    setSearchParams(next);
  };
  const setSymbolFilter = (value: string) => {
    const next = new URLSearchParams(searchParams);
    if (!value) next.delete("symbol");
    else next.set("symbol", value.toUpperCase());
    setSearchParams(next);
  };

  const positions = usePositions();
  const sellPositionId = searchParams.get("sell");
  const sellTarget =
    positions.data?.find((position) => position.id === sellPositionId) ?? null;

  const selectSellTarget = (id: string) => {
    const next = new URLSearchParams(searchParams);
    next.set("sell", id);
    setSearchParams(next);
  };
  const clearSellTarget = () => {
    if (!searchParams.has("sell")) return;
    const next = new URLSearchParams(searchParams);
    next.delete("sell");
    setSearchParams(next, { replace: true });
  };

  return (
    <div>
      <div className="flex justify-end">
        <button
          type="button"
          onClick={() => setArchiveOpen((open) => !open)}
          aria-pressed={archiveOpen}
          className={cn(
            "flex items-center gap-1.5 rounded-lg border px-2.5 py-1.5 text-xs font-medium transition",
            archiveOpen
              ? "border-cyan-300/30 bg-cyan-300/10 text-cyan-200"
              : "border-zinc-800 bg-zinc-900/40 text-zinc-400 hover:border-zinc-700 hover:text-zinc-100",
          )}
        >
          <History className="h-3.5 w-3.5" />
          Archive
        </button>
      </div>

      <CashFlowRecorder />

      <div className="mt-4 grid gap-4 md:grid-cols-2">
        <RecordBuyCard />
        <RecordSaleCard
          positions={positions.data ?? []}
          isLoading={positions.isLoading}
          onSelect={selectSellTarget}
        />
      </div>

      <ClosePositionDialog
        position={sellTarget}
        open={sellTarget !== null}
        onOpenChange={(open) => {
          if (!open) clearSellTarget();
        }}
      />

      {archiveOpen && (
        <Card className="mt-6 p-5">
          <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
            <div className="flex flex-1 gap-1 overflow-x-auto rounded-xl border border-zinc-800 bg-zinc-900/40 p-1">
              {FILTERS.map((filter) => (
                <button
                  key={filter.value}
                  type="button"
                  onClick={() => setCategory(filter.value)}
                  className={cn(
                    "shrink-0 rounded-lg px-3 py-2 text-xs font-medium transition",
                    category === filter.value
                      ? "bg-zinc-100 text-zinc-950"
                      : "text-zinc-400 hover:bg-zinc-800 hover:text-zinc-100",
                  )}
                >
                  {filter.label}
                </button>
              ))}
            </div>
            <input
              aria-label="Filter by symbol"
              value={symbolFilter}
              onChange={(event) => setSymbolFilter(event.target.value)}
              placeholder="Symbol (optional)"
              className="w-full rounded-lg border border-zinc-700 bg-zinc-900 px-3 py-2 text-sm uppercase tracking-wide sm:w-40"
            />
          </div>

          <section className="mt-4 space-y-2" aria-live="polite">
            {activity.isLoading && <Card className="p-6 text-sm text-zinc-500">Searching…</Card>}
            {activity.isError && <Card className="p-6 text-sm text-rose-300">Activity is unavailable.</Card>}
            {(activity.data?.items ?? []).map((item) => (
              <ActivityRow key={item.id} activity={item} />
            ))}
            {!activity.isLoading && !activity.isError && (activity.data?.items.length ?? 0) === 0 && (
              <Card className="p-10 text-center">
                <h2 className="font-semibold text-zinc-100">No matching activity</h2>
                <p className="mt-2 text-sm text-zinc-500">
                  Nothing in your history matches this search yet.
                </p>
              </Card>
            )}
          </section>
        </Card>
      )}
    </div>
  );
}

function CashFlowRecorder() {
  const deposit = useDepositCash();
  const withdrawal = useWithdrawCash();
  const [currency, setCurrency] = useState<CurrencyCode>("USD");
  const [amount, setAmount] = useState("");
  const value = Number(amount);
  const pending = deposit.isPending || withdrawal.isPending;
  const record = (kind: "deposit" | "withdrawal") => {
    if (!Number.isFinite(value) || value <= 0) return;
    (kind === "deposit" ? deposit : withdrawal).mutate(
      { currency, amount: value },
      { onSuccess: () => setAmount("") },
    );
  };

  return (
    <Card className="mt-6 p-5">
      <div className="flex items-center gap-2">
        <CircleDollarSign className="h-4 w-4 text-zinc-400" />
        <h2 className="font-semibold text-zinc-100">Record a cash flow</h2>
      </div>
      <p className="mt-1 text-xs text-zinc-500">
        Records an account action that happened elsewhere. No bank transfer is initiated.
      </p>
      <div className="mt-4 grid gap-2 sm:grid-cols-[110px_1fr_auto_auto]">
        <select
          aria-label="Cash flow currency"
          value={currency}
          onChange={(event) => setCurrency(event.target.value as CurrencyCode)}
          className="rounded-lg border border-zinc-700 bg-zinc-900 px-3 py-2 text-sm"
        >
          {CURRENCIES.map((item) => <option key={item}>{item}</option>)}
        </select>
        <input
          aria-label="Cash flow amount"
          type="number"
          min="0"
          step="any"
          value={amount}
          onChange={(event) => setAmount(event.target.value)}
          placeholder="Amount"
          className="rounded-lg border border-zinc-700 bg-zinc-900 px-3 py-2 text-sm"
        />
        <Button disabled={pending || value <= 0} onClick={() => record("deposit")}>Record deposit</Button>
        <Button disabled={pending || value <= 0} variant="outline" onClick={() => record("withdrawal")}>Record withdrawal</Button>
      </div>
    </Card>
  );
}

function RecordBuyCard() {
  return (
    <Card className="p-5">
      <div className="flex items-center gap-2">
        <PlusCircle className="h-4 w-4 text-zinc-400" />
        <h2 className="font-semibold text-zinc-100">Record buy</h2>
      </div>
      <p className="mt-1 text-xs text-zinc-500">
        Uses available cash in the asset's quote currency. No brokerage order is placed.
      </p>
      <div className="mt-4">
        <AddPositionForm compact />
      </div>
    </Card>
  );
}

function RecordSaleCard({
  positions,
  isLoading,
  onSelect,
}: {
  positions: Position[];
  isLoading: boolean;
  onSelect: (positionId: string) => void;
}) {
  return (
    <Card className="p-5">
      <div className="flex items-center gap-2">
        <MinusCircle className="h-4 w-4 text-zinc-400" />
        <h2 className="font-semibold text-zinc-100">Record sale</h2>
      </div>
      <p className="mt-1 text-xs text-zinc-500">
        Record a sale that already happened for one of your open positions.
      </p>
      <div className="mt-4 space-y-2">
        {isLoading && <p className="text-sm text-zinc-500">Loading positions…</p>}
        {!isLoading && positions.length === 0 && (
          <p className="text-sm text-zinc-500">
            No open positions to sell yet — record a buy first.
          </p>
        )}
        {positions.map((position) => (
          <button
            key={position.id}
            type="button"
            onClick={() => onSelect(position.id)}
            className="flex w-full items-center justify-between rounded-lg border border-zinc-800 bg-zinc-900/40 px-3 py-2.5 text-left text-sm transition hover:border-zinc-600 hover:bg-zinc-900"
          >
            <span className="flex items-center gap-2">
              <span className="font-mono font-medium text-zinc-100">{position.symbol}</span>
              <span className="text-xs text-zinc-500">owned: {position.quantity}</span>
            </span>
            <span className="text-xs font-medium text-amber-300">Sell</span>
          </button>
        ))}
      </div>
    </Card>
  );
}

function ActivityRow({ activity }: { activity: PortfolioActivity }) {
  const automatic = activity.origin !== "user_recorded";
  const amount = activity.net_amount ?? activity.gross_amount;
  const signed = activity.activity_type === "withdrawal" ||
    activity.activity_type.includes("fee") ? -amount : amount;

  return (
    <Card className="grid gap-3 p-4 sm:grid-cols-[1fr_auto] sm:items-center">
      <div>
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-semibold capitalize text-zinc-100">
            {activity.activity_type.replaceAll("_", " ")}
          </span>
          {activity.symbol && <span className="font-mono text-xs text-zinc-400">{activity.symbol}</span>}
          <span className="rounded-full border border-zinc-800 px-2 py-0.5 text-[10px] text-zinc-500">
            {automatic ? "Automatic" : "User recorded"}
          </span>
          <span className="text-[10px] capitalize text-zinc-600">{activity.status}</span>
        </div>
        <p className="mt-1 text-xs text-zinc-500">
          {new Date(activity.occurred_at).toLocaleString()}
          {automatic ? " · No action required" : ""}
        </p>
        {activity.position_episode_id && (
          <Link
            to={`/portfolio?episode=${activity.position_episode_id}`}
            className="mt-2 inline-flex items-center gap-1 text-xs text-zinc-400 hover:text-zinc-100"
          >
            View position <ArrowRight className="h-3 w-3" />
          </Link>
        )}
      </div>
      <div className="text-left sm:text-right">
        <div className={cn("font-mono text-sm tabular-nums", gainLossColor(signed))}>
          {formatMoney(signed, activity.currency)}
        </div>
        {activity.quantity !== undefined && activity.unit_price !== undefined && (
          <p className="mt-1 text-xs text-zinc-500">
            {activity.quantity} at {formatMoney(activity.unit_price, activity.currency)}
          </p>
        )}
        {activity.realized_gain_loss_base !== undefined && (
          <p className={cn("mt-1 text-xs", gainLossColor(activity.realized_gain_loss_base))}>
            Realized {formatMoney(activity.realized_gain_loss_base, "USD")}
          </p>
        )}
      </div>
    </Card>
  );
}
