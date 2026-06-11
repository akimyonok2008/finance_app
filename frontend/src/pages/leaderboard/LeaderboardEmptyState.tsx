import { Trophy } from "lucide-react";
import { Link } from "react-router-dom";

export function LeaderboardEmptyState() {
  return (
    <div className="flex flex-col items-center rounded-2xl border border-dashed border-zinc-800 bg-zinc-900/30 px-6 py-16 text-center">
      <div className="grid h-12 w-12 place-items-center rounded-xl border border-zinc-800 bg-zinc-900/50 text-zinc-400">
        <Trophy className="h-6 w-6" />
      </div>
      <h2 className="mt-5 text-xl font-semibold tracking-tight">
        No rankings yet
      </h2>
      <p className="mt-2 max-w-md text-sm text-zinc-400">
        Once investors add portfolios, performance rankings will appear here.
      </p>
      <Link
        to="/portfolio"
        className="mt-6 rounded-xl bg-zinc-50 px-5 py-2.5 text-sm font-medium text-zinc-950 transition hover:bg-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-emerald-400/50"
      >
        Go to Portfolio
      </Link>
    </div>
  );
}
