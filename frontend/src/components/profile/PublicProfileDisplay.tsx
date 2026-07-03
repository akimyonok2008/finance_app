import { ShieldCheck } from "lucide-react";

import { ConcentrationCard } from "@/components/profile/ConcentrationCard";
import { ExposureBreakdownCard } from "@/components/profile/ExposureBreakdownCard";
import { FollowButton } from "@/components/social/FollowButton";
import { ProfileBadgesCard } from "@/components/profile/ProfileBadgesCard";
import { ProfileHeader } from "@/components/profile/ProfileHeader";
import { ProfilePerformanceCards } from "@/components/profile/ProfilePerformanceCards";
import { PublicWeightsCard } from "@/components/profile/PublicWeightsCard";
import { StrategyProfileActions } from "@/components/strategy/StrategyProfileActions";
import { useMyProfile } from "@/hooks/useProfile";
import type { PublicProfile } from "@/types/profile";

export function PublicProfileDisplay({ profile }: { profile: PublicProfile }) {
  const me = useMyProfile();
  const isSelf = me.data?.handle === profile.handle;

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
        <ProfileHeader profile={profile} />
        <div className="flex flex-wrap justify-end gap-2">
          <FollowButton handle={profile.handle} isSelf={isSelf} />
          {profile.public_weights.length > 0 && (
          <StrategyProfileActions
            handle={profile.handle}
            displayName={profile.display_name}
            canCopy
          />
          )}
        </div>
      </div>
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
