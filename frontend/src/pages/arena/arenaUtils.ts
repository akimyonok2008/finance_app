import type { Achievement } from "@/types/arena";

export function formatSignedPercent(value: number | null | undefined): string {
  if (value === null || value === undefined || !Number.isFinite(value)) {
    return "—";
  }
  if (value > 0) return `+${value.toFixed(2)}%`;
  return `${value.toFixed(2)}%`;
}

export function getPercentClassName(
  value: number | null | undefined,
): string {
  if (value === null || value === undefined || !Number.isFinite(value)) {
    return "text-zinc-400";
  }
  if (value > 0) return "text-emerald-500";
  if (value < 0) return "text-rose-500";
  return "text-zinc-400";
}

export function calculateProgressPercent(
  currentProgress: number,
  targetProgress: number,
): number {
  if (targetProgress <= 0) return 0;
  return Math.min(100, Math.round((currentProgress / targetProgress) * 100));
}

export function formatUnlockedDate(value?: string): string {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  return `Unlocked ${new Intl.DateTimeFormat("en-GB", {
    day: "2-digit",
    month: "short",
    year: "numeric",
  }).format(date)}`;
}

export function sortAchievements(
  achievements: Achievement[],
): Achievement[] {
  return [...achievements].sort((a, b) => {
    if (a.isUnlocked !== b.isUnlocked) return a.isUnlocked ? -1 : 1;
    if (a.isUnlocked && b.isUnlocked) {
      return (
        new Date(b.unlockedAt ?? 0).getTime() -
        new Date(a.unlockedAt ?? 0).getTime()
      );
    }
    return (
      calculateProgressPercent(b.currentProgress, b.targetProgress) -
      calculateProgressPercent(a.currentProgress, a.targetProgress)
    );
  });
}
