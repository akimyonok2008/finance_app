import { ChartNoAxesColumnIncreasing } from "lucide-react";

import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import type { PerformanceHistoryPoint } from "@/types/profile";
import { formatPercent } from "@/utils/formatPercent";
import { gainLossColor } from "@/utils/gainLoss";

function formatDate(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "Recent";
  return new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
  }).format(date);
}

function sparklinePoints(points: PerformanceHistoryPoint[]): string {
  if (points.length === 0) return "";
  if (points.length === 1) return "0,28 100,28";

  const values = points.map((point) => point.portfolio_index);
  const min = Math.min(...values);
  const max = Math.max(...values);
  const span = Math.max(max - min, 1);

  return points
    .map((point, index) => {
      const x = (index / (points.length - 1)) * 100;
      const y = 52 - ((point.portfolio_index - min) / span) * 48;
      return `${x.toFixed(2)},${y.toFixed(2)}`;
    })
    .join(" ");
}

export function ProfilePerformanceHistoryCard({
  history,
}: {
  history: PerformanceHistoryPoint[];
}) {
  const visibleHistory = history.slice(-8);
  const latest = visibleHistory.at(-1);
  const first = visibleHistory.at(0);
  const movement =
    latest && first ? latest.portfolio_index - first.portfolio_index : undefined;
  const linePoints = sparklinePoints(visibleHistory);

  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between gap-4 space-y-0">
        <div>
          <CardTitle>Performance history</CardTitle>
          <p className="mt-1 text-sm text-zinc-500">
            Public index and return history. No values or quantities.
          </p>
        </div>
        <ChartNoAxesColumnIncreasing className="h-5 w-5 text-zinc-500" />
      </CardHeader>
      <CardContent>
        {visibleHistory.length === 0 ? (
          <div className="rounded-xl border border-dashed border-zinc-800 px-4 py-8 text-center text-sm text-zinc-500">
            No public history yet.
          </div>
        ) : (
          <div className="grid gap-5 lg:grid-cols-[1.1fr_.9fr]">
            <div className="rounded-xl border border-zinc-800 bg-zinc-950/40 p-4">
              <div className="flex items-start justify-between gap-3">
                <div>
                  <p className="text-[10px] font-medium uppercase tracking-widest text-zinc-500">
                    Latest index
                  </p>
                  <p className="mt-2 font-mono text-2xl font-semibold tabular-nums text-zinc-100">
                    {latest?.portfolio_index.toFixed(2) ?? "-"}
                  </p>
                </div>
                <div className="text-right">
                  <p className="text-[10px] font-medium uppercase tracking-widest text-zinc-500">
                    Window move
                  </p>
                  <p
                    className={`mt-2 font-mono text-sm font-semibold tabular-nums ${gainLossColor(
                      movement,
                    )}`}
                  >
                    {movement === undefined
                      ? "-"
                      : `${movement > 0 ? "+" : ""}${movement.toFixed(2)}`}
                  </p>
                </div>
              </div>
              <svg
                viewBox="0 0 100 56"
                className="mt-5 h-24 w-full overflow-visible"
                role="img"
                aria-label="Profile performance history"
                preserveAspectRatio="none"
              >
                <polyline
                  points={linePoints}
                  fill="none"
                  stroke="currentColor"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth="2.2"
                  className="text-zinc-100"
                  vectorEffect="non-scaling-stroke"
                />
              </svg>
            </div>
            <div className="space-y-2">
              {visibleHistory
                .slice()
                .reverse()
                .map((point) => (
                  <div
                    key={`${point.captured_at}-${point.portfolio_index}`}
                    className="flex items-center justify-between gap-4 rounded-lg border border-zinc-800 bg-zinc-950/30 px-3 py-2"
                  >
                    <div>
                      <p className="text-sm font-medium text-zinc-200">
                        {formatDate(point.captured_at)}
                      </p>
                      <p className="text-xs text-zinc-500">
                        Index {point.portfolio_index.toFixed(2)}
                      </p>
                    </div>
                    <p
                      className={`font-mono text-sm font-semibold tabular-nums ${gainLossColor(
                        point.return_percentage,
                      )}`}
                    >
                      {formatPercent(point.return_percentage)}
                    </p>
                  </div>
                ))}
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
