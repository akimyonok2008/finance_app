import { LockKeyhole } from "lucide-react";

import type { PublicWeight } from "@/types/profile";

export function PublicWeightsCard({ weights }: { weights: PublicWeight[] }) {
  return (
    <section className="rounded-2xl border border-zinc-800 bg-zinc-900/50 p-5">
      <div>
        <h2 className="text-sm font-semibold text-zinc-100">Public strategy weights</h2>
        <p className="mt-1 text-xs text-zinc-500">Symbols and allocation percentages.</p>
      </div>
      {weights.length === 0 ? (
        <div className="mt-5 rounded-xl border border-dashed border-zinc-800 px-4 py-8 text-center text-sm text-zinc-500">
          This user has not shared public weights.
        </div>
      ) : (
        <div className="mt-5 divide-y divide-zinc-800">
          {weights.map((item) => (
            <div key={`${item.symbol}-${item.asset_type ?? ""}`} className="flex items-center gap-3 py-3">
              <div className="min-w-0 flex-1">
                <div className="font-mono text-sm font-semibold text-zinc-100">{item.symbol}</div>
                <div className="mt-0.5 text-[11px] capitalize text-zinc-500">{item.asset_type ?? "asset"}</div>
              </div>
              <div className="font-mono text-sm tabular-nums text-zinc-200">{item.weight_percentage.toFixed(2)}%</div>
            </div>
          ))}
        </div>
      )}
      <div className="mt-4 flex items-start gap-2 border-t border-zinc-800 pt-4 text-[11px] leading-5 text-zinc-500">
        <LockKeyhole className="mt-0.5 h-3.5 w-3.5 shrink-0" />
        Weights are public. Quantities, values, cost basis, and buy prices are private.
      </div>
    </section>
  );
}
