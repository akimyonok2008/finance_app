import { cn } from "@/utils/cn";
import type { CoachTop10Comparison } from "@/types/coach";

function fmtNum(v: number | undefined, suffix = ""): string {
  if (v === undefined || v === null || !Number.isFinite(v)) return "—";
  return `${v.toFixed(2)}${suffix}`;
}

function fmtSigned(v: number | undefined, suffix = ""): string {
  if (v === undefined || v === null || !Number.isFinite(v)) return "—";
  const sign = v > 0 ? "+" : "";
  return `${sign}${v.toFixed(2)}${suffix}`;
}

function Stat({
  label,
  value,
  tone,
}: {
  label: string;
  value: string;
  tone?: "positive" | "negative" | "neutral";
}) {
  return (
    <div className="rounded-lg border border-zinc-800 bg-zinc-900/40 px-3 py-2">
      <div className="text-[11px] uppercase tracking-wide text-zinc-500">
        {label}
      </div>
      <div
        className={cn(
          "font-mono text-base tabular-nums",
          tone === "positive" && "text-emerald-300",
          tone === "negative" && "text-rose-300",
          (!tone || tone === "neutral") && "text-zinc-100",
        )}
      >
        {value}
      </div>
    </div>
  );
}

/** Renders the deterministic top-10 comparison. Handles the limited/unavailable
 * benchmark as a calm informational state, not an error. */
export function CoachComparisonPanel({
  comparison,
}: {
  comparison?: CoachTop10Comparison;
}) {
  if (!comparison) return null;

  if (!comparison.available) {
    return (
      <div className="rounded-lg border border-zinc-800 bg-zinc-900/40 p-3 text-sm text-zinc-400">
        Top-10 comparison isn’t available yet — more leaderboard participants
        with positions are needed before a benchmark can be shown.
        {comparison.notes?.length ? (
          <ul className="mt-1.5 list-disc space-y-0.5 pl-4 text-zinc-500">
            {comparison.notes.map((n, i) => (
              <li key={i}>{n}</li>
            ))}
          </ul>
        ) : null}
      </div>
    );
  }

  const gap = comparison.return_gap_percentage_points;
  const gapTone =
    gap === undefined ? "neutral" : gap > 0 ? "positive" : gap < 0 ? "negative" : "neutral";

  return (
    <div className="space-y-2">
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
        <Stat label="Sample" value={fmtNum(comparison.sample_size)} />
        <Stat label="Return gap" value={fmtSigned(gap, " pp")} tone={gapTone} />
        <Stat label="Shared symbols" value={fmtNum(comparison.shared_symbols_count)} />
        <Stat
          label="Your largest wt."
          value={fmtNum(comparison.user_largest_weight_percentage, "%")}
        />
      </div>
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
        <Stat
          label="Top-10 median largest wt."
          value={fmtNum(comparison.top10_median_largest_weight_percentage, "%")}
        />
      </div>
      {comparison.limited && (
        <p className="text-xs text-amber-300/80">
          Limited benchmark: fewer than 10 other portfolios — treat comparisons
          as directional.
        </p>
      )}
      {comparison.notes?.length ? (
        <ul className="list-disc space-y-0.5 pl-4 text-xs text-zinc-500">
          {comparison.notes.map((n, i) => (
            <li key={i}>{n}</li>
          ))}
        </ul>
      ) : null}
    </div>
  );
}
