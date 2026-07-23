import { Search, SlidersHorizontal } from "lucide-react";

import { ExploreMiniProfileCard } from "@/components/explore/ExploreMiniProfileCard";
import type { ExploreProfile } from "@/types/explore";

export function ExploreSearchResults({
  profiles,
  query,
  symbol,
}: {
  profiles: ExploreProfile[];
  query: string;
  symbol: string;
}) {
  const context = [
    query ? `“${query}”` : "",
    symbol ? `${symbol} holders` : "",
  ].filter(Boolean);

  return (
    <section className="rounded-2xl border border-sky-300/10 bg-[radial-gradient(circle_at_top_left,rgba(56,189,248,0.06),transparent_34%),rgba(24,24,27,0.34)] p-4 sm:p-5">
      <div className="flex flex-wrap items-end justify-between gap-3 border-b border-sky-300/10 pb-3">
        <div>
          <div className="flex items-center gap-2">
            <Search className="h-3.5 w-3.5 text-sky-300/75" />
            <h2 className="explore-display text-base font-semibold text-zinc-100">
              Search results
            </h2>
          </div>
          <p className="mt-1 text-[11px] text-zinc-500">
            {context.length > 0
              ? `Profiles matching ${context.join(" and ")}.`
              : "Profiles matching your selected filters."}
          </p>
        </div>
        <span className="flex items-center gap-1.5 font-mono text-[9px] uppercase tracking-[0.16em] text-sky-300/55">
          <SlidersHorizontal className="h-3 w-3" />
          {profiles.length} {profiles.length === 1 ? "match" : "matches"}
        </span>
      </div>

      {profiles.length > 0 ? (
        <div className="mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
          {profiles.map((profile) => (
            <ExploreMiniProfileCard
              key={profile.handle}
              profile={profile}
              accent="search"
            />
          ))}
        </div>
      ) : (
        <div className="mt-3 rounded-xl border border-dashed border-sky-300/10 px-4 py-8 text-center">
          <p className="text-sm font-medium text-zinc-300">No matching profiles</p>
          <p className="mt-1 text-xs text-zinc-500">
            Try another name, handle, or public holding symbol.
          </p>
        </div>
      )}
    </section>
  );
}
