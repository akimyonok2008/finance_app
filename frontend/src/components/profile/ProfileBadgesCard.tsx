import { Trophy } from "lucide-react";

import type { ProfileBadge } from "@/types/profile";

export function ProfileBadgesCard({ badges }: { badges: ProfileBadge[] }) {
  return (
    <section className="rounded-2xl border border-zinc-800 bg-zinc-900/50 p-5">
      <h2 className="text-sm font-semibold text-zinc-100">Public badges</h2>
      {badges.length === 0 ? (
        <p className="mt-4 text-sm text-zinc-500">No public badges yet.</p>
      ) : (
        <div className="mt-4 flex flex-wrap gap-2">
          {badges.map((badge) => (
            <div key={badge.key} className="flex items-center gap-2 rounded-xl border border-amber-400/15 bg-amber-400/[0.04] px-3 py-2 text-xs text-zinc-300">
              <Trophy className="h-3.5 w-3.5 text-amber-300" />
              {badge.name}
            </div>
          ))}
        </div>
      )}
    </section>
  );
}
