import { CircleUserRound, Trophy } from "lucide-react";
import { Link } from "react-router-dom";

import type { ExploreProfile } from "@/types/explore";
import { formatPercent } from "@/utils/formatPercent";
import { gainLossColor } from "@/utils/gainLoss";

export function TopPerformersList({ profiles }: { profiles: ExploreProfile[] }) {
  return (
    <section>
      <h2 className="text-base font-semibold text-zinc-100">Top Performers</h2>
      <p className="mt-1 text-xs text-zinc-500">Ranked strategy performance from public baselines.</p>
      <div className="mt-4 overflow-hidden rounded-2xl border border-zinc-800 bg-zinc-900/40">
        {profiles.map((profile, index) => (
          <Link
            key={profile.handle}
            to={`/profiles/${encodeURIComponent(profile.handle)}`}
            className="grid gap-4 border-b border-zinc-800 p-4 transition last:border-b-0 hover:bg-white/[0.025] sm:grid-cols-[36px_minmax(160px,1fr)_110px_90px_minmax(140px,1fr)] sm:items-center"
          >
            <div className="font-mono text-sm tabular-nums text-zinc-500">
              #{profile.global_rank ?? index + 1}
            </div>
            <div className="flex min-w-0 items-center gap-3">
              <div className="grid h-9 w-9 shrink-0 place-items-center rounded-xl border border-zinc-800 bg-zinc-950">
                <CircleUserRound className="h-4 w-4 text-zinc-500" />
              </div>
              <div className="min-w-0">
                <div className="truncate text-sm font-medium text-zinc-100">{profile.display_name}</div>
                <div className="truncate font-mono text-[10px] text-zinc-600">@{profile.handle}</div>
              </div>
            </div>
            <div>
              <div className="text-[9px] uppercase tracking-widest text-zinc-600">Ranked return</div>
              <div className={`mt-1 font-mono text-sm font-semibold tabular-nums ${gainLossColor(profile.ranked_return_percentage)}`}>
                {formatPercent(profile.ranked_return_percentage)}
              </div>
            </div>
            <div>
              <div className="text-[9px] uppercase tracking-widest text-zinc-600">Index</div>
              <div className="mt-1 font-mono text-sm tabular-nums text-zinc-200">{profile.ranked_index?.toFixed(1) ?? "—"}</div>
            </div>
            <div className="flex min-w-0 flex-wrap gap-1.5">
              {profile.public_weights.slice(0, 3).map((weight) => (
                <span key={weight.symbol} className="rounded-md border border-zinc-800 bg-zinc-950/70 px-1.5 py-1 font-mono text-[9px] text-zinc-400">
                  {weight.symbol} {weight.weight_percentage.toFixed(1)}%
                </span>
              ))}
              {profile.badges[0] ? <Trophy className="h-3.5 w-3.5 text-amber-300/70" /> : null}
            </div>
          </Link>
        ))}
      </div>
    </section>
  );
}
