import { ShieldCheck } from "lucide-react";

import { ConcentrationCard } from "@/components/profile/ConcentrationCard";
import { ExposureBreakdownCard } from "@/components/profile/ExposureBreakdownCard";
import { ProfileBadgesCard } from "@/components/profile/ProfileBadgesCard";
import { ProfileHeader } from "@/components/profile/ProfileHeader";
import { ProfilePerformanceCards } from "@/components/profile/ProfilePerformanceCards";
import { PublicWeightsCard } from "@/components/profile/PublicWeightsCard";
import type { PublicProfile } from "@/types/profile";

export function PublicProfileDisplay({ profile }: { profile: PublicProfile }) {
  return (
    <div className="space-y-5">
      <ProfileHeader profile={profile} />
      <ProfilePerformanceCards profile={profile} />
      <div className="grid gap-5 xl:grid-cols-[1.05fr_.95fr]">
        <PublicWeightsCard weights={profile.public_weights} />
        <div className="space-y-5">
          <ExposureBreakdownCard assetTypes={profile.asset_type_exposure} currencies={profile.currency_exposure} />
          <ConcentrationCard concentration={profile.concentration} />
          <ProfileBadgesCard badges={profile.badges} />
        </div>
      </div>
      <div className="flex items-center gap-2 rounded-xl border border-zinc-800 bg-zinc-900/30 px-4 py-3 text-xs text-zinc-500">
        <ShieldCheck className="h-3.5 w-3.5 shrink-0" />
        Profiles show strategy and weights, not net worth.
      </div>
    </div>
  );
}
