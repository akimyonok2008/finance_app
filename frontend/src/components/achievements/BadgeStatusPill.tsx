import { BadgeCheck, Loader, Lock } from "lucide-react";

import type { BadgeStatus } from "@/types/achievements";
import { cn } from "@/utils/cn";

const CONFIG: Record<
  BadgeStatus,
  { label: string; className: string; Icon: typeof Lock }
> = {
  unlocked: {
    label: "Unlocked",
    className: "border-emerald-500/25 bg-emerald-500/[0.07] text-emerald-300",
    Icon: BadgeCheck,
  },
  in_progress: {
    label: "In progress",
    className: "border-sky-500/25 bg-sky-500/[0.07] text-sky-300",
    Icon: Loader,
  },
  locked: {
    label: "Locked",
    className: "border-zinc-700 bg-zinc-800/40 text-zinc-400",
    Icon: Lock,
  },
};

export function BadgeStatusPill({
  status,
  className,
}: {
  status: BadgeStatus;
  className?: string;
}) {
  const { label, className: tone, Icon } = CONFIG[status];
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide",
        tone,
        className,
      )}
    >
      <Icon className="h-3 w-3" />
      {label}
    </span>
  );
}
