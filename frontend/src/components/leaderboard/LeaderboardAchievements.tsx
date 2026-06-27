import { Award, Check, LockKeyhole } from "lucide-react";

import { Skeleton } from "@/components/ui/skeleton";
import { useAchievements } from "@/hooks/useAchievements";

export function LeaderboardAchievements() {
  const query = useAchievements();
  const achievements = query.data ?? [];
  const unlocked = achievements.filter((item) => item.unlocked).length;

  return (
    <section className="rounded-2xl border border-zinc-800 bg-zinc-900/35 p-5">
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <Award className="h-4 w-4 text-violet-300" />
          <h2 className="text-sm font-semibold text-zinc-100">Achievements</h2>
        </div>
        <span className="font-mono text-xs text-zinc-500">{unlocked}/{achievements.length}</span>
      </div>
      {query.isLoading ? (
        <div className="mt-4 space-y-2"><Skeleton className="h-12" /><Skeleton className="h-12" /></div>
      ) : query.isError ? (
        <p className="mt-4 text-sm text-rose-300">Achievements are unavailable.</p>
      ) : (
        <div className="mt-4 space-y-2">
          {achievements.slice(0, 6).map((achievement) => (
            <div key={achievement.key} className="flex gap-3 rounded-xl border border-zinc-800/80 bg-zinc-950/35 px-3 py-3">
              <div className={`mt-0.5 grid h-6 w-6 shrink-0 place-items-center rounded-full ${achievement.unlocked ? "bg-emerald-400/10 text-emerald-300" : "bg-zinc-800 text-zinc-600"}`}>
                {achievement.unlocked ? <Check className="h-3.5 w-3.5" /> : <LockKeyhole className="h-3 w-3" />}
              </div>
              <div>
                <p className="text-xs font-medium text-zinc-200">{achievement.name}</p>
                <p className="mt-0.5 text-[11px] leading-4 text-zinc-500">{achievement.description}</p>
              </div>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}
