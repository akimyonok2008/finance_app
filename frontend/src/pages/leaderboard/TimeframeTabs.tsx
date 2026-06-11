import type { LeaderboardTimeframe } from "@/types/leaderboard";
import { cn } from "@/utils/cn";

const tabs = [
  { label: "Daily", value: "daily" },
  { label: "Weekly", value: "weekly" },
  { label: "Monthly", value: "monthly" },
  { label: "All-Time", value: "all_time" },
] as const;

export function TimeframeTabs({
  value,
  onChange,
}: {
  value: LeaderboardTimeframe;
  onChange: (value: LeaderboardTimeframe) => void;
}) {
  return (
    <div
      role="tablist"
      aria-label="Leaderboard timeframe"
      className="grid grid-cols-2 rounded-lg border border-zinc-800 bg-zinc-900/40 p-1 sm:flex"
    >
      {tabs.map((tab) => {
        const active = tab.value === value;
        return (
          <button
            key={tab.value}
            type="button"
            role="tab"
            aria-selected={active}
            onClick={() => onChange(tab.value)}
            className={cn(
              "rounded-md px-4 py-2 text-xs font-medium transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-500",
              active
                ? "bg-zinc-50 text-zinc-950 shadow-sm"
                : "text-zinc-400 hover:bg-white/[0.04] hover:text-zinc-100",
            )}
          >
            {tab.label}
          </button>
        );
      })}
    </div>
  );
}
