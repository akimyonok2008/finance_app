import { cn } from "@/utils/cn";

export type AchievementFilter =
  | "all"
  | "unlocked"
  | "in_progress"
  | "easy"
  | "medium"
  | "hard"
  | "elite"
  | "investor"
  | "strategy";

const FILTERS: { value: AchievementFilter; label: string }[] = [
  { value: "all", label: "All" },
  { value: "unlocked", label: "Unlocked" },
  { value: "in_progress", label: "In progress" },
  { value: "easy", label: "Easy" },
  { value: "medium", label: "Medium" },
  { value: "hard", label: "Hard" },
  { value: "elite", label: "Elite" },
  { value: "investor", label: "Legendary investors" },
  { value: "strategy", label: "Strategies" },
];

export function AchievementFilterBar({
  value,
  onChange,
  counts,
}: {
  value: AchievementFilter;
  onChange: (value: AchievementFilter) => void;
  counts: Record<AchievementFilter, number>;
}) {
  return (
    <div
      role="tablist"
      aria-label="Filter badges"
      className="flex flex-wrap gap-1.5"
    >
      {FILTERS.map(({ value: v, label }) => {
        const active = v === value;
        return (
          <button
            key={v}
            type="button"
            role="tab"
            aria-selected={active}
            onClick={() => onChange(v)}
            className={cn(
              "flex items-center gap-1.5 rounded-lg border px-3 py-1.5 text-xs font-medium transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-500",
              active
                ? "border-zinc-300 bg-zinc-50 text-zinc-950"
                : "border-zinc-800 bg-zinc-900/40 text-zinc-400 hover:border-zinc-700 hover:text-zinc-100",
            )}
          >
            {label}
            <span
              className={cn(
                "font-mono text-[10px] tabular-nums",
                active ? "text-zinc-500" : "text-zinc-600",
              )}
            >
              {counts[v]}
            </span>
          </button>
        );
      })}
    </div>
  );
}
