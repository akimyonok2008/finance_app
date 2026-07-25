import { Card } from "@/components/ui/card";
import { useIncomeEvents } from "@/hooks/usePositions";
import type { IncomeEventView } from "@/api/portfolioApi";

// AutomaticIncome is a restrained, read-only view of investment income the system
// detected and credited automatically — dividends, ETF/fund distributions, bond
// coupons, return of capital, stock dividends. Users NEVER enter ordinary income;
// the background pipeline detects it from provider data, calculates eligibility
// from historical holdings, and credits cash (or reinvested shares). The section
// stays hidden when there is nothing to show.
export function AutomaticIncome() {
  const { data, isLoading } = useIncomeEvents();
  const items = data ?? [];
  if (isLoading || items.length === 0) {
    return null;
  }
  return (
    <Card className="mt-4 border-zinc-800 bg-zinc-950/40 p-5">
      <div className="flex items-baseline justify-between gap-3">
        <h2 className="portfolio-display text-lg font-semibold text-zinc-100">
          Income
        </h2>
        <span className="text-[11px] text-zinc-500">
          Detected &amp; credited automatically · no action required
        </span>
      </div>
      <ul className="mt-3 divide-y divide-zinc-800/70">
        {items.map((item) => (
          <li key={item.id} className="flex items-start justify-between gap-4 py-2.5">
            <div className="min-w-0">
              <p className="text-sm font-medium text-zinc-200">
                {item.symbol || eventLabel(item.event_type)}
                {item.estimated && (
                  <span className="ml-2 rounded-full border border-amber-700/50 px-1.5 py-0.5 text-[9px] uppercase tracking-wide text-amber-400/90">
                    Estimated
                  </span>
                )}
              </p>
              <p className="mt-0.5 text-xs text-zinc-500">{item.explanation}</p>
              <p className="mt-1 text-[11px] text-zinc-500">{amountLine(item)}</p>
            </div>
            <span className="shrink-0 rounded-full border border-zinc-700 px-2 py-0.5 text-[10px] uppercase tracking-wide text-zinc-400">
              {item.status}
            </span>
          </li>
        ))}
      </ul>
    </Card>
  );
}

// amountLine renders gross / withholding / net distinctly so nothing is an
// ambiguous single figure. Return of capital and stock dividends read
// differently from ordinary cash income.
function amountLine(item: IncomeEventView): string {
  const money = (v: number) => `${item.currency} ${v.toFixed(2)}`;
  if (item.event_type === "stock_dividend") {
    return "Stock dividend — quantity and cost basis adjusted; no cash";
  }
  if (item.reinvestment_quantity && item.reinvestment_quantity > 0) {
    return `Net ${money(item.net_amount)} reinvested · ${item.reinvestment_quantity.toFixed(4)} shares`;
  }
  if (item.withholding_amount > 0 || item.fee_amount > 0) {
    return `Gross ${money(item.gross_amount)} · withholding ${money(
      item.withholding_amount,
    )} · net ${money(item.net_amount)}`;
  }
  return `Net ${money(item.net_amount)} credited`;
}

function eventLabel(type: string): string {
  return type.replace(/_/g, " ");
}
