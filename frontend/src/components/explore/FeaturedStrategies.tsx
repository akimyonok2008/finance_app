import { Sparkles } from "lucide-react";

import { ExploreMiniProfileCard } from "@/components/explore/ExploreMiniProfileCard";
import type { ExploreProfile } from "@/types/explore";

export function FeaturedStrategies({ profiles }: { profiles: ExploreProfile[] }) {
  return (
    <section>
      <div className="flex items-end justify-between gap-4 border-b border-amber-300/10 pb-2.5">
        <div>
          <div className="flex items-center gap-2">
            <Sparkles className="h-3.5 w-3.5 text-amber-300/75" />
            <h2 className="explore-display text-base font-semibold text-zinc-100">Featured for You</h2>
          </div>
          <p className="mt-1 text-[11px] text-zinc-500">High-quality public strategies selected for performance, context, and variety.</p>
        </div>
        <span className="font-mono text-[9px] uppercase tracking-[0.18em] text-amber-300/45">Curated</span>
      </div>
      {profiles.length > 0 ? (
        <div className="mt-3 grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
          {profiles.slice(0, 5).map((profile) => (
            <ExploreMiniProfileCard key={profile.handle} profile={profile} accent="featured" />
          ))}
        </div>
      ) : (
        <p className="mt-3 rounded-xl border border-dashed border-amber-300/10 bg-amber-300/[0.025] px-4 py-5 text-xs text-zinc-500">
          Featured profiles will appear as more public strategies meet the quality threshold.
        </p>
      )}
    </section>
  );
}
