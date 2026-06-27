import { ArrowUpRight, CircleUserRound, Trophy } from "lucide-react";
import { Link } from "react-router-dom";

import type { ExploreProfile } from "@/types/explore";
import { formatPercent } from "@/utils/formatPercent";
import { gainLossColor } from "@/utils/gainLoss";

const pretty = (value?: string) =>
  value
    ? value.replaceAll("_", " ").replace(/\b\w/g, (letter) => letter.toUpperCase())
    : "Balanced";

export function ExploreProfileCard({ profile }: { profile: ExploreProfile }) {
  const weights = profile.public_weights.slice(0, 3);
  const badge = profile.badges[0];

  return (
    <article className="flex h-full min-w-0 flex-col rounded-2xl border border-zinc-800 bg-zinc-900/50 p-5 transition hover:border-zinc-700">
      <div className="flex items-start gap-3">
        <div className="grid h-10 w-10 shrink-0 place-items-center rounded-xl border border-zinc-700 bg-zinc-950">
          <CircleUserRound className="h-5 w-5 text-zinc-400" />
        </div>
        <div className="min-w-0 flex-1">
          <Link
            to={`/profiles/${encodeURIComponent(profile.handle)}`}
            className="block truncate text-sm font-semibold text-zinc-100 hover:text-white"
          >
            {profile.display_name}
          </Link>
          <div className="truncate font-mono text-[11px] text-zinc-500">@{profile.handle}</div>
        </div>
        <span className="max-w-32 truncate rounded-full border border-violet-400/15 bg-violet-400/[0.04] px-2 py-1 text-[10px] text-violet-200">
          {pretty(profile.strategy_tag)}
        </span>
      </div>

      {profile.bio ? (
        <p className="mt-4 line-clamp-2 text-xs leading-5 text-zinc-500">{profile.bio}</p>
      ) : null}

      <div className="mt-5 grid grid-cols-3 gap-2 border-y border-zinc-800 py-4">
        <Metric label="Index" value={profile.ranked_index?.toFixed(1) ?? "—"} />
        <Metric
          label="Return"
          value={formatPercent(profile.ranked_return_percentage)}
          className={gainLossColor(profile.ranked_return_percentage)}
        />
        <Metric label="Rank" value={profile.global_rank ? `#${profile.global_rank}` : "—"} />
      </div>

      <div className="mt-4 flex min-h-7 flex-wrap gap-1.5">
        {weights.length > 0 ? (
          weights.map((weight) => (
            <span key={weight.symbol} className="rounded-lg border border-zinc-800 bg-zinc-950/70 px-2 py-1 font-mono text-[10px] tabular-nums text-zinc-300">
              {weight.symbol} {weight.weight_percentage.toFixed(1)}%
            </span>
          ))
        ) : (
          <span className="text-[11px] text-zinc-600">Weights hidden</span>
        )}
      </div>

      <div className="mt-auto flex items-end justify-between gap-3 pt-5">
        <div className="min-w-0 text-[10px] leading-5 text-zinc-500">
          {profile.concentration?.position_count !== undefined ? `${profile.concentration.position_count} holdings` : "Holdings private"}
          {profile.concentration?.top3_weight_percentage !== undefined
            ? ` · Top 3 ${profile.concentration.top3_weight_percentage.toFixed(1)}%`
            : ""}
          {badge ? (
            <div className="mt-1 flex items-center gap-1 truncate text-amber-300/80">
              <Trophy className="h-3 w-3 shrink-0" /> {badge.name}
            </div>
          ) : null}
        </div>
        <Link
          to={`/profiles/${encodeURIComponent(profile.handle)}`}
          className="flex shrink-0 items-center gap-1 text-xs font-medium text-zinc-300 hover:text-white"
        >
          View profile <ArrowUpRight className="h-3.5 w-3.5" />
        </Link>
      </div>
    </article>
  );
}

function Metric({ label, value, className = "text-zinc-100" }: { label: string; value: string; className?: string }) {
  return (
    <div>
      <div className="text-[9px] uppercase tracking-widest text-zinc-600">{label}</div>
      <div className={`mt-1 font-mono text-sm font-semibold tabular-nums ${className}`}>{value}</div>
    </div>
  );
}
