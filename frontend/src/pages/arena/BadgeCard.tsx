import { motion } from "framer-motion";
import { BadgeCheck, Lock, Sparkles, Trophy } from "lucide-react";

import {
  calculateProgressPercent,
  formatUnlockedDate,
} from "@/pages/arena/arenaUtils";
import type { Achievement } from "@/types/arena";
import { cn } from "@/utils/cn";

export function BadgeCard({
  achievement,
  index = 0,
}: {
  achievement: Achievement;
  index?: number;
}) {
  const progress = calculateProgressPercent(
    achievement.currentProgress,
    achievement.targetProgress,
  );

  return (
    <motion.article
      initial={{ opacity: 0, y: 10 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.2, delay: Math.min(index * 0.04, 0.25) }}
      whileHover={{ y: -2 }}
      className={cn(
        "rounded-2xl border p-4",
        achievement.isUnlocked
          ? "border-violet-500/20 bg-zinc-900/70"
          : "border-zinc-800 bg-zinc-900/40 opacity-80",
      )}
    >
      <div className="flex items-start gap-3">
        <div
          className={cn(
            "grid h-11 w-11 shrink-0 place-items-center rounded-2xl border",
            achievement.isUnlocked
              ? "border-violet-500/20 bg-violet-500/[0.04] text-violet-300"
              : "border-zinc-700 bg-zinc-900 text-zinc-500",
          )}
        >
          {achievement.isUnlocked ? (
            <Trophy className="h-5 w-5" />
          ) : (
            <Lock className="h-4 w-4" />
          )}
        </div>
        <div className="min-w-0">
          <div className="flex items-center gap-1.5">
            <h3 className="text-sm font-semibold text-zinc-100">
              {achievement.name}
            </h3>
            {achievement.isUnlocked ? (
              <BadgeCheck className="h-4 w-4 text-violet-300" />
            ) : (
              <Sparkles className="h-3.5 w-3.5 text-zinc-600" />
            )}
          </div>
          <p className="mt-1 text-xs leading-relaxed text-zinc-500">
            {achievement.description}
          </p>
        </div>
      </div>

      <div className="mt-4">
        <div className="mb-2 flex items-center justify-between gap-2">
          <span className="text-[10px] uppercase tracking-widest text-zinc-500">
            {achievement.isUnlocked ? "Completed" : "Progress"}
          </span>
          <span className="font-mono text-xs tabular-nums text-zinc-400">
            {achievement.currentProgress} / {achievement.targetProgress}
          </span>
        </div>
        <div className="h-2 overflow-hidden rounded-full bg-zinc-800">
          <div
            className={cn("h-full rounded-full transition-all", achievement.isUnlocked ? "bg-violet-400" : "bg-zinc-400")}
            style={{ width: `${progress}%` }}
          />
        </div>
        {achievement.isUnlocked && achievement.unlockedAt && (
          <div className="mt-2 text-[11px] text-zinc-500">
            {formatUnlockedDate(achievement.unlockedAt)}
          </div>
        )}
        {!achievement.isUnlocked && (
          <div className="mt-2 flex items-center gap-1 text-[11px] text-zinc-600">
            <Lock className="h-3 w-3" />
            Locked
          </div>
        )}
      </div>
    </motion.article>
  );
}
