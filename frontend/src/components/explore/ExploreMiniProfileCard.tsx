import { ArrowUpRight, Award } from "lucide-react";
import { Link } from "react-router-dom";

import type { ExploreProfile } from "@/types/explore";
import { cn } from "@/utils/cn";
import { formatPercent } from "@/utils/formatPercent";
import { gainLossColor } from "@/utils/gainLoss";

const pretty = (value?: string) =>
  value
    ? value.replaceAll("_", " ").replace(/\b\w/g, (letter) => letter.toUpperCase())
    : "Balanced";

const accents = {
  featured: {
    frame: "border-amber-300/15 hover:border-amber-300/30",
    glow: "from-amber-300/[0.08]",
    avatar: "border-amber-300/20 bg-amber-300/[0.07] text-amber-100",
    tag: "border-amber-300/15 bg-amber-300/[0.05] text-amber-100/80",
    detail: "text-amber-200/70",
  },
  similar: {
    frame: "border-violet-300/15 hover:border-violet-300/30",
    glow: "from-violet-400/[0.09]",
    avatar: "border-violet-300/20 bg-violet-400/[0.07] text-violet-100",
    tag: "border-violet-300/15 bg-violet-400/[0.05] text-violet-100/80",
    detail: "text-violet-200/70",
  },
  search: {
    frame: "border-sky-300/15 hover:border-sky-300/30",
    glow: "from-sky-400/[0.08]",
    avatar: "border-sky-300/20 bg-sky-400/[0.07] text-sky-100",
    tag: "border-sky-300/15 bg-sky-400/[0.05] text-sky-100/80",
    detail: "text-sky-200/70",
  },
} as const;

export function ExploreMiniProfileCard({
  profile,
  accent,
}: {
  profile: ExploreProfile;
  accent: keyof typeof accents;
}) {
  const style = accents[accent];
  const initials = profile.display_name
    .split(/\s+/)
    .slice(0, 2)
    .map((part) => part[0])
    .join("")
    .toUpperCase();
  const badge = profile.badges[0];

  return (
    <article
      className={cn(
        "group relative flex min-h-40 min-w-0 flex-col overflow-hidden rounded-xl border bg-zinc-900/55 p-3.5 transition duration-200 hover:-translate-y-0.5 hover:bg-zinc-900/80",
        style.frame,
      )}
    >
      <div className={cn("pointer-events-none absolute inset-x-0 top-0 h-14 bg-gradient-to-b to-transparent", style.glow)} />
      <div className="relative flex items-start gap-2.5">
        <div className={cn("grid h-8 w-8 shrink-0 place-items-center rounded-lg border font-mono text-[10px] font-medium", style.avatar)}>
          {initials || "—"}
        </div>
        <div className="min-w-0 flex-1">
          <Link
            to={`/profiles/${encodeURIComponent(profile.handle)}`}
            className="block truncate text-[13px] font-semibold tracking-[-0.01em] text-zinc-100 hover:text-white"
          >
            {profile.display_name}
          </Link>
          <div className="truncate font-mono text-[9px] text-zinc-500">@{profile.handle}</div>
        </div>
        <span className={cn("max-w-24 truncate rounded-full border px-2 py-1 text-[8px] font-medium", style.tag)}>
          {pretty(profile.strategy_tag)}
        </span>
      </div>

      <div className="relative mt-3 flex items-baseline gap-2.5">
        <span className={cn("font-mono text-sm font-semibold tabular-nums", gainLossColor(profile.ranked_return_percentage))}>
          {formatPercent(profile.ranked_return_percentage)}
        </span>
        <span className="font-mono text-[9px] tabular-nums text-zinc-500">
          {profile.global_rank ? `#${profile.global_rank}` : `IDX ${profile.ranked_index?.toFixed(1) ?? "—"}`}
        </span>
      </div>

      <div className="relative mt-2.5 flex min-h-6 items-center gap-1.5 overflow-hidden">
        {profile.public_weights.slice(0, 2).map((weight) => (
          <span key={weight.symbol} className="truncate rounded-md border border-zinc-800/90 bg-zinc-950/75 px-1.5 py-1 font-mono text-[8px] tabular-nums text-zinc-400">
            {weight.symbol} {weight.weight_percentage.toFixed(0)}%
          </span>
        ))}
      </div>

      <div className="relative mt-auto flex items-center justify-between gap-2 border-t border-white/[0.05] pt-2.5">
        <div className={cn("flex min-w-0 items-center gap-1 text-[9px]", style.detail)}>
          {badge ? <Award className="h-3 w-3 shrink-0" /> : null}
          <span className="truncate">{badge?.name ?? pretty(profile.strategy_tag)}</span>
        </div>
        <Link
          to={`/profiles/${encodeURIComponent(profile.handle)}`}
          aria-label={`View ${profile.display_name}'s profile`}
          className="flex shrink-0 items-center gap-1 text-[10px] font-medium text-zinc-400 transition group-hover:text-zinc-100"
        >
          View <ArrowUpRight className="h-3 w-3" />
        </Link>
      </div>
    </article>
  );
}
