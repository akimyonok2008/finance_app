import { cn } from "@/utils/cn";
import type { CoachModeMeta } from "@/types/coach";

type Props = {
  meta: CoachModeMeta;
  active: boolean;
  disabled: boolean;
  onSelect: () => void;
};

/** A compact mode action button. Title + one-line description, no icons noise. */
export function CoachModeButton({ meta, active, disabled, onSelect }: Props) {
  return (
    <button
      type="button"
      onClick={onSelect}
      disabled={disabled}
      aria-pressed={active}
      className={cn(
        "flex flex-col items-start gap-0.5 rounded-lg border px-3 py-2.5 text-left transition",
        "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-500/50",
        "disabled:cursor-not-allowed disabled:opacity-50",
        active
          ? "border-violet-500/40 bg-violet-500/10"
          : "border-zinc-800 bg-zinc-900/40 hover:border-zinc-700 hover:bg-zinc-800/60",
      )}
    >
      <span
        className={cn(
          "text-sm font-medium",
          active ? "text-violet-200" : "text-zinc-100",
        )}
      >
        {meta.label}
      </span>
      <span className="text-xs leading-snug text-zinc-500">
        {meta.description}
      </span>
    </button>
  );
}
