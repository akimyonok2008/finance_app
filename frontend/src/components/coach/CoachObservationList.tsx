import { statusBadgeClass } from "@/components/coach/coachStyles";
import { cn } from "@/utils/cn";
import type { CoachObservation } from "@/types/coach";

/** Renders labelled observations; the status word is always shown as text so
 * meaning never depends on color alone. */
export function CoachObservationList({
  observations,
}: {
  observations?: CoachObservation[];
}) {
  if (!observations || observations.length === 0) return null;

  return (
    <ul className="space-y-2">
      {observations.map((o, i) => (
        <li
          key={`${o.label}-${i}`}
          className="rounded-lg border border-zinc-800 bg-zinc-900/40 p-3"
        >
          <div className="mb-1 flex items-center gap-2">
            <span className="text-sm font-medium text-zinc-200">{o.label}</span>
            <span
              className={cn(
                "rounded border px-1.5 py-0.5 text-[11px] font-medium uppercase tracking-wide",
                statusBadgeClass(o.status),
              )}
            >
              {o.status}
            </span>
          </div>
          <p className="text-sm leading-relaxed text-zinc-400">{o.text}</p>
        </li>
      ))}
    </ul>
  );
}
