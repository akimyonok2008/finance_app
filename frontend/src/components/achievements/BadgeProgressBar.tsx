import type { BadgeStatus } from "@/types/achievements";
import { cn } from "@/utils/cn";

export function BadgeProgressBar({
  pct,
  status,
  className,
}: {
  pct: number;
  status: BadgeStatus;
  className?: string;
}) {
  const clamped = Math.min(100, Math.max(0, pct));
  const tone =
    status === "unlocked"
      ? "bg-emerald-400"
      : status === "in_progress"
        ? "bg-sky-400"
        : "bg-zinc-600";

  return (
    <div
      className={cn("h-1.5 overflow-hidden rounded-full bg-zinc-800", className)}
      role="progressbar"
      aria-valuenow={Math.round(clamped)}
      aria-valuemin={0}
      aria-valuemax={100}
    >
      <div
        className={cn("h-full rounded-full transition-all", tone)}
        style={{ width: `${clamped}%` }}
      />
    </div>
  );
}
