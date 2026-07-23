import { DIFFICULTY_LABEL, type BadgeDifficulty } from "@/data/badgeCatalogue";
import { cn } from "@/utils/cn";

const STYLES: Record<BadgeDifficulty, string> = {
  easy: "border-emerald-500/25 bg-emerald-500/[0.06] text-emerald-300",
  medium: "border-sky-500/25 bg-sky-500/[0.06] text-sky-300",
  hard: "border-amber-500/25 bg-amber-500/[0.06] text-amber-300",
  elite: "border-violet-500/25 bg-violet-500/[0.06] text-violet-300",
};

export function BadgeDifficultyPill({
  difficulty,
  className,
}: {
  difficulty: BadgeDifficulty;
  className?: string;
}) {
  return (
    <span
      className={cn(
        "inline-flex items-center rounded-full border px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide",
        STYLES[difficulty],
        className,
      )}
    >
      {DIFFICULTY_LABEL[difficulty]}
    </span>
  );
}
