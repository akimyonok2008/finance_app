import { LeaderboardRow } from "@/pages/leaderboard/LeaderboardRow";
import type { LeaderboardEntry } from "@/types/leaderboard";

export function LeaderboardTable({ entries }: { entries: LeaderboardEntry[] }) {
  return (
    <div className="hidden overflow-hidden rounded-2xl border border-zinc-800 bg-zinc-900/40 shadow-sm shadow-black/20 lg:block">
      <table className="w-full">
        <thead className="border-b border-zinc-800 bg-white/[0.02] text-left text-xs uppercase tracking-[0.18em] text-zinc-500">
          <tr>
            <th className="px-5 py-4 font-medium">Rank</th>
            <th className="px-5 py-4 font-medium">Investor</th>
            <th className="px-5 py-4 font-medium">Strategy</th>
            <th className="px-5 py-4 font-medium">24h Change</th>
            <th className="px-5 py-4 text-right font-medium">Total ROI</th>
          </tr>
        </thead>
        <tbody>
          {entries.map((entry, index) => (
            <LeaderboardRow
              key={`${entry.rank}-${entry.display_name}`}
              entry={entry}
              index={index}
            />
          ))}
        </tbody>
      </table>
    </div>
  );
}
