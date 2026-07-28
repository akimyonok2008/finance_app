import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import {
  useCashBalances,
  useDepositCash,
  usePortfolioActivities,
  useWithdrawCash,
} from "@/hooks/usePositions";
import { CURRENCIES, type CurrencyCode } from "@/types/portfolio";
import { formatMoney } from "@/utils/formatMoney";
import { compareDecimal, formatQuantity, isValidDecimalString } from "@/utils/decimal";

export function CashActivityPanel() {
  const cash = useCashBalances();
  const activities = usePortfolioActivities();
  const deposit = useDepositCash();
  const withdrawal = useWithdrawCash();
  const [currency, setCurrency] = useState<CurrencyCode>("USD");
  const [amount, setAmount] = useState("");

  const trimmedAmount = amount.trim();
  const validAmount =
    trimmedAmount !== "" &&
    isValidDecimalString(trimmedAmount) &&
    compareDecimal(trimmedAmount, "0") > 0;
  const pending = deposit.isPending || withdrawal.isPending;
  const submit = (kind: "deposit" | "withdrawal") => {
    if (!validAmount) return;
    const mutation = kind === "deposit" ? deposit : withdrawal;
    mutation.mutate(
      { currency, amount: trimmedAmount },
      { onSuccess: () => setAmount("") },
    );
  };

  return (
    <div className="mt-6 grid gap-4 lg:grid-cols-[1fr_1.35fr]">
      <Card className="border-emerald-300/10 bg-emerald-300/[0.025] p-5">
        <div className="flex items-start justify-between gap-3">
          <div>
            <h2 className="portfolio-display text-lg font-semibold text-zinc-100">Cash</h2>
            <p className="mt-1 text-xs text-zinc-500">Multi-currency, immediately settled tracking balances.</p>
          </div>
          <span className="text-sm font-semibold text-emerald-200">
            {formatMoney(cash.data?.total_cash_value_base ?? 0, cash.data?.base_currency ?? "USD")}
          </span>
        </div>

        <div className="mt-4 grid gap-2">
          {(cash.data?.cash_balances ?? []).map((balance) => (
            <div key={balance.currency} className="flex items-center justify-between rounded-lg bg-zinc-950/45 px-3 py-2 text-sm">
              <span className="font-semibold text-zinc-200">{balance.currency}</span>
              <span className="text-zinc-400">
                {formatQuantity(balance.amount)} · {balance.weight_percentage.toFixed(1)}%
              </span>
            </div>
          ))}
          {!cash.isLoading && (cash.data?.cash_balances.length ?? 0) === 0 && (
            <p className="text-sm text-zinc-500">No cash recorded yet.</p>
          )}
        </div>

        <div className="mt-4 grid grid-cols-[90px_1fr] gap-2">
          <select
            value={currency}
            onChange={(event) => setCurrency(event.target.value as CurrencyCode)}
            className="rounded-lg border border-zinc-700 bg-zinc-900 px-2 py-2 text-sm"
          >
            {CURRENCIES.map((item) => <option key={item}>{item}</option>)}
          </select>
          <input
            aria-label="Cash amount"
            type="text"
            inputMode="decimal"
            value={amount}
            onChange={(event) => setAmount(event.target.value)}
            placeholder="Amount"
            className="rounded-lg border border-zinc-700 bg-zinc-900 px-3 py-2 text-sm"
          />
        </div>
        <div className="mt-2 grid grid-cols-2 gap-2">
          <Button disabled={pending || !validAmount} onClick={() => submit("deposit")}>
            Record deposit
          </Button>
          <Button disabled={pending || !validAmount} variant="outline" onClick={() => submit("withdrawal")}>
            Record withdrawal
          </Button>
        </div>
      </Card>

      <Card className="border-indigo-300/10 bg-indigo-300/[0.02] p-5">
        <h2 className="portfolio-display text-lg font-semibold text-zinc-100">Recent activity</h2>
        <p className="mt-1 text-xs text-zinc-500">Owner-private tracking records; no orders or bank transfers are placed.</p>
        <div className="mt-4 max-h-64 space-y-2 overflow-y-auto">
          {(activities.data ?? []).map((activity) => (
            <div key={activity.id} className="flex items-center justify-between gap-4 rounded-lg bg-zinc-950/45 px-3 py-2 text-sm">
              <div>
                <span className="font-semibold capitalize text-zinc-200">
                  {activity.activity_type.replace("_", " ")}
                </span>
                {activity.symbol && <span className="ml-2 text-zinc-500">{activity.symbol}</span>}
                <p className="text-[11px] text-zinc-600">{new Date(activity.occurred_at).toLocaleString()}</p>
              </div>
              <span className="text-right text-zinc-400">
                {activity.quantity ? `${activity.quantity} · ` : ""}
                {activity.gross_amount.toLocaleString()} {activity.currency}
              </span>
            </div>
          ))}
          {!activities.isLoading && (activities.data?.length ?? 0) === 0 && (
            <p className="text-sm text-zinc-500">No activity recorded yet.</p>
          )}
        </div>
      </Card>
    </div>
  );
}
