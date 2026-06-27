import { CalendarDays, CircleUserRound, Medal, Trophy } from "lucide-react";

import type { PublicProfile } from "@/types/profile";

const pretty = (value?: string) =>
  value ? value.replaceAll("_", " ").replace(/\b\w/g, (c) => c.toUpperCase()) : "";

export function ProfileHeader({ profile }: { profile: PublicProfile }) {
  return (
    <header className="rounded-2xl border border-zinc-800 bg-zinc-900/50 p-5 sm:p-6">
      <div className="flex flex-col gap-5 sm:flex-row sm:items-start">
        <div className="grid h-16 w-16 shrink-0 place-items-center rounded-2xl border border-zinc-700 bg-zinc-950 text-zinc-300">
          <CircleUserRound className="h-7 w-7" />
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="break-words text-2xl font-medium tracking-tight text-zinc-50">
              {profile.display_name}
            </h1>
            {profile.strategy_tag && (
              <span className="rounded-full border border-violet-400/20 bg-violet-400/[0.06] px-2.5 py-1 text-xs text-violet-200">
                {pretty(profile.strategy_tag)}
              </span>
            )}
          </div>
          <p className="mt-1 break-all font-mono text-xs text-zinc-500">
            @{profile.handle}
          </p>
          {profile.bio && (
            <p className="mt-3 max-w-2xl break-words text-sm leading-6 text-zinc-300">
              {profile.bio}
            </p>
          )}
          <div className="mt-4 flex flex-wrap gap-x-4 gap-y-2 text-xs text-zinc-500">
            {profile.global_rank ? (
              <span className="flex items-center gap-1.5">
                <Medal className="h-3.5 w-3.5" /> Global #{profile.global_rank}
              </span>
            ) : null}
            {profile.sprint_rank ? (
              <span className="flex items-center gap-1.5">
                <Trophy className="h-3.5 w-3.5" /> Sprint #{profile.sprint_rank}
              </span>
            ) : null}
            {profile.joined_at ? (
              <span className="flex items-center gap-1.5">
                <CalendarDays className="h-3.5 w-3.5" />
                Joined {new Date(profile.joined_at).toLocaleDateString()}
              </span>
            ) : null}
          </div>
        </div>
      </div>
    </header>
  );
}
