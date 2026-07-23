import { Orbit } from "lucide-react";

import { ExploreMiniProfileCard } from "@/components/explore/ExploreMiniProfileCard";
import type { ExploreProfile } from "@/types/explore";

export function SimilarStrategies({ profiles }: { profiles: ExploreProfile[] }) {
  return (
    <section>
      <div className="flex items-end justify-between gap-4 border-b border-violet-300/10 pb-2.5">
        <div>
          <div className="flex items-center gap-2">
            <Orbit className="h-3.5 w-3.5 text-violet-300/75" />
            <h2 className="explore-display text-base font-semibold text-zinc-100">Similar to You</h2>
          </div>
          <p className="mt-1 text-[11px] text-zinc-500">Profiles with overlapping holdings, exposures, or strategy patterns.</p>
        </div>
        <span className="font-mono text-[9px] uppercase tracking-[0.18em] text-violet-300/45">Matched</span>
      </div>
      {profiles.length > 0 ? (
        <div className="mt-3 grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
          {profiles.slice(0, 5).map((profile) => (
            <ExploreMiniProfileCard key={profile.handle} profile={profile} accent="similar" />
          ))}
        </div>
      ) : (
        <p className="mt-3 rounded-xl border border-dashed border-violet-300/10 bg-violet-300/[0.025] px-4 py-5 text-xs text-zinc-500">
          Similar profiles will appear after your portfolio has enough comparable data.
        </p>
      )}
    </section>
  );
}
