import { Archive } from "lucide-react";

import type { PublicClosedPosition } from "@/types/profile";
import { formatPercent } from "@/utils/formatPercent";
import { gainLossColor } from "@/utils/gainLoss";

function formatDate(value?: string): string {
  if (!value) return "Closed";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "Closed";
  return new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
    year: "numeric",
  }).format(date);
}

export function PublicClosedPositionsCard({
  positions,
}: {
  positions: PublicClosedPosition[];
}) {
  return (
    <section className="rounded-2xl border border-zinc-800 bg-zinc-900/50 p-5">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h2 className="text-sm font-semibold text-zinc-100">Closed positions</h2>
          <p className="mt-1 text-xs text-zinc-500">
            Sold holdings shared as symbols and return percentages.
          </p>
        </div>
        <Archive className="h-4 w-4 text-zinc-500" />
      </div>
      {positions.length === 0 ? (
        <div className="mt-5 rounded-xl border border-dashed border-zinc-800 px-4 py-8 text-center text-sm text-zinc-500">
          This user has not shared closed positions.
        </div>
      ) : (
        <div className="mt-5 divide-y divide-zinc-800">
          {positions.map((item) => (
            <div
              key={`${item.symbol}-${item.closed_at ?? ""}`}
              className="flex items-center gap-3 py-3"
            >
              <div className="min-w-0 flex-1">
                <div className="font-mono text-sm font-semibold text-zinc-100">
                  {item.symbol}
                </div>
                <div className="mt-0.5 text-[11px] capitalize text-zinc-500">
                  {item.asset_type ?? "asset"} · {formatDate(item.closed_at)}
                </div>
              </div>
              <div
                className={`font-mono text-sm font-semibold tabular-nums ${gainLossColor(
                  item.return_percentage,
                )}`}
              >
                {formatPercent(item.return_percentage)}
              </div>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}
