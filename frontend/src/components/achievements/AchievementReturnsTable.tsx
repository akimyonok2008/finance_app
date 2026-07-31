import { useQuery } from "@tanstack/react-query";
import { ChevronDown, ChevronUp } from "lucide-react";
import { useState } from "react";

import { getAchievementReturns } from "@/api/achievements";
import { TimeframeTabs } from "@/pages/leaderboard/TimeframeTabs";
import { queryKeys } from "@/hooks/queryKeys";
import type { LeaderboardTimeframe } from "@/types/leaderboard";
import { formatPercent } from "@/utils/formatPercent";

export function AchievementReturnsTable({
  timeframe,
  onTimeframeChange,
}: {
  timeframe: LeaderboardTimeframe;
  onTimeframeChange: (value: LeaderboardTimeframe) => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const query = useQuery({
    queryKey: queryKeys.achievementReturns(timeframe),
    queryFn: ({ signal }) => getAchievementReturns(timeframe, signal),
    retry: 1,
  });
  const rows = query.data?.rows ?? [];
  const visibleRows = expanded ? rows : rows.slice(0, 5);

  return (
    <section
      aria-labelledby="achievement-returns-title"
      className="overflow-hidden rounded-2xl border border-zinc-800 bg-zinc-900/35"
    >
      <div className="flex flex-col gap-4 border-b border-zinc-800 px-4 py-4 sm:px-5 lg:flex-row lg:items-end lg:justify-between">
        <div>
          <p className="text-[10px] font-semibold uppercase tracking-[0.18em] text-violet-300/70">
            Performance comparison
          </p>
          <h2
            id="achievement-returns-title"
            className="achievements-card-title mt-1 text-lg font-semibold text-zinc-100"
          >
            Achievement returns
          </h2>
          <p className="mt-1 max-w-2xl text-xs leading-5 text-zinc-500">
            Your ranked portfolio return against every achievement benchmark over the same selected window.
          </p>
        </div>
        <TimeframeTabs value={timeframe} onChange={onTimeframeChange} />
      </div>

      {query.isLoading ? (
        <div className="space-y-2 p-4" aria-label="Loading achievement returns">
          {Array.from({ length: 5 }).map((_, index) => (
            <div key={index} className="h-12 animate-pulse rounded-lg bg-zinc-800/45" />
          ))}
        </div>
      ) : query.isError ? (
        <p className="px-5 py-8 text-center text-sm text-amber-200/80">
          Achievement returns are unavailable right now.
        </p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full min-w-[760px] border-collapse text-left">
            <thead>
              <tr className="border-b border-zinc-800 text-[10px] font-semibold uppercase tracking-[0.14em] text-zinc-600">
                <th className="px-5 py-3">Achievement</th>
                <th className="px-3 py-3">Native goal</th>
                <th className="px-3 py-3 text-right">Your return</th>
                <th className="px-3 py-3 text-right">Benchmark</th>
                <th className="px-5 py-3 text-right">Edge</th>
              </tr>
            </thead>
            <tbody>
              {visibleRows.map((row) => (
                <tr key={row.key} className="border-b border-zinc-800/70 last:border-0">
                  <td className="px-5 py-3">
                    <p className="text-sm font-semibold text-zinc-200">{row.name}</p>
                    <p className="mt-0.5 text-[11px] capitalize text-zinc-600">{row.difficulty}</p>
                  </td>
                  <td className="px-3 py-3 text-xs text-zinc-500">{row.native_period}</td>
                  {row.available ? (
                    <>
                      <ReturnCell value={row.portfolio_return_percentage} />
                      <ReturnCell value={row.benchmark_return_percentage} />
                      <ReturnCell value={row.edge_points} points />
                    </>
                  ) : (
                    <td colSpan={3} className="px-5 py-3 text-right text-xs text-zinc-600">
                      {row.reason ?? "Return unavailable for this window."}
                    </td>
                  )}
                </tr>
              ))}
            </tbody>
          </table>
          {rows.length > 5 && (
            <div className="border-t border-zinc-800/80 px-4 py-3 text-center">
              <button
                type="button"
                aria-expanded={expanded}
                onClick={() => setExpanded((value) => !value)}
                className="inline-flex items-center gap-2 rounded-full px-4 py-2 text-xs font-semibold text-zinc-400 transition hover:bg-white/[0.04] hover:text-zinc-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-300/40"
              >
                {expanded ? <>Show fewer achievements <ChevronUp className="h-4 w-4" /></> : <>Show all {rows.length} achievements <ChevronDown className="h-4 w-4" /></>}
              </button>
            </div>
          )}
        </div>
      )}
    </section>
  );
}

function ReturnCell({ value, points = false }: { value?: number; points?: boolean }) {
  const positive = typeof value === "number" && value > 0;
  const negative = typeof value === "number" && value < 0;
  const formatted = points
    ? typeof value === "number"
      ? `${value > 0 ? "+" : ""}${value.toFixed(2)} pts`
      : "—"
    : formatPercent(value);
  return (
    <td
      className={`px-3 py-3 text-right font-mono text-xs ${
        positive ? "text-emerald-300" : negative ? "text-rose-300" : "text-zinc-400"
      }`}
    >
      {formatted}
    </td>
  );
}
