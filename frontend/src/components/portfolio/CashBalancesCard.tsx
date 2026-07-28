import { Card } from "@/components/ui/card";
import { useCashBalances } from "@/hooks/usePositions";
import { formatMoney } from "@/utils/formatMoney";
import { formatQuantity } from "@/utils/decimal";

export function CashBalancesCard() {
  const cash = useCashBalances();

  if (cash.isLoading) {
    return <Card className="p-6 text-sm text-zinc-500">Loading cash balances…</Card>;
  }
  if (cash.isError) {
    return <Card className="p-6 text-sm text-rose-300">Cash balances are unavailable.</Card>;
  }

  const balances = cash.data?.cash_balances ?? [];
  return (
    <Card className="border-emerald-300/10 bg-emerald-300/[0.025] p-5">
      <div className="flex items-start justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold text-zinc-100">Cash</h2>
          <p className="mt-1 text-xs text-zinc-500">
            Current settled balances. Deposit and withdrawal records live in Activity.
          </p>
        </div>
        <span className="font-mono text-sm font-semibold tabular-nums text-emerald-200">
          {formatMoney(cash.data?.total_cash_value_base, cash.data?.base_currency)}
        </span>
      </div>
      <div className="mt-4 overflow-hidden rounded-xl border border-zinc-800">
        {balances.map((balance) => (
          <div
            key={balance.currency}
            className="grid grid-cols-3 gap-3 border-b border-zinc-800 px-4 py-3 text-sm last:border-b-0"
          >
            <span className="font-semibold text-zinc-200">{balance.currency}</span>
            <span className="text-right font-mono tabular-nums text-zinc-400">
              {formatQuantity(balance.amount)}
            </span>
            <span className="text-right font-mono tabular-nums text-zinc-400">
              {formatMoney(balance.value_base, cash.data?.base_currency)} · {balance.weight_percentage.toFixed(1)}%
            </span>
          </div>
        ))}
        {balances.length === 0 && (
          <p className="px-4 py-6 text-center text-sm text-zinc-500">
            No cash recorded yet.
          </p>
        )}
      </div>
    </Card>
  );
}
