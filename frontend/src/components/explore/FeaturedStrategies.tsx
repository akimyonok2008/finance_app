import { ExploreProfileCard } from "@/components/explore/ExploreProfileCard";
import type { ExploreProfile } from "@/types/explore";

export function FeaturedStrategies({ profiles }: { profiles: ExploreProfile[] }) {
  if (profiles.length === 0) return null;
  return (
    <section>
      <h2 className="text-base font-semibold text-zinc-100">Featured Strategies</h2>
      <p className="mt-1 text-xs text-zinc-500">Strong ranked performance with visible public weights.</p>
      <div className="mt-4 grid gap-4 md:grid-cols-2 2xl:grid-cols-3">
        {profiles.slice(0, 5).map((profile) => (
          <ExploreProfileCard key={profile.handle} profile={profile} />
        ))}
      </div>
    </section>
  );
}
