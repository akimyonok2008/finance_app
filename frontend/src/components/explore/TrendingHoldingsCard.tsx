import { TrendingUp } from "lucide-react";

import type { TrendingHolding } from "@/types/explore";
import { cn } from "@/utils/cn";

export function TrendingHoldingsCard({
  holdings,
  selectedSymbol,
  onSelectSymbol,
}: {
  holdings: TrendingHolding[];
  selectedSymbol?: string;
  onSelectSymbol: (symbol: string) => void;
}) {
  return (
    <section className="rounded-2xl border border-zinc-800 bg-zinc-900/50 p-5">
      <div className="flex items-center gap-2">
        <TrendingUp className="h-4 w-4 text-zinc-500" />
        <h2 className="text-sm font-semibold text-zinc-100">Trending Holdings</h2>
      </div>
      <p className="mt-1 text-xs text-zinc-500">Most common symbols in public strategies.</p>
      {holdings.length === 0 ? (
        <p className="mt-5 text-sm leading-6 text-zinc-600">Trending holdings will appear when more public strategies are available.</p>
      ) : (
        <div className="mt-4 divide-y divide-zinc-800">
          {holdings.slice(0, 12).map((holding) => (
            <button
              key={holding.symbol}
              type="button"
              onClick={() => onSelectSymbol(holding.symbol)}
              className={cn(
                "flex w-full items-center gap-3 py-3 text-left transition hover:text-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-500",
                selectedSymbol === holding.symbol ? "text-zinc-100" : "text-zinc-300",
              )}
            >
              <div className="min-w-0 flex-1">
                <div className="font-mono text-sm font-semibold">{holding.symbol}</div>
                <div className="mt-1 text-[10px] text-zinc-600">
                  {holding.profile_count} profiles
                  {holding.average_weight_percentage !== undefined ? ` · avg ${holding.average_weight_percentage.toFixed(1)}%` : ""}
                  {holding.top10_count !== undefined ? ` · ${holding.top10_count} in top 10` : ""}
                </div>
              </div>
              <span className="rounded-md border border-zinc-800 px-1.5 py-1 text-[9px] capitalize text-zinc-500">{holding.asset_type ?? "asset"}</span>
            </button>
          ))}
        </div>
      )}
    </section>
  );
}
