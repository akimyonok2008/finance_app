import { Card } from "@/components/ui/card";
import { useCorporateActions } from "@/hooks/usePositions";

// AutomaticAdjustments is a restrained, read-only view of corporate actions the
// system applied (or is processing) automatically. Users never enter these; the
// section stays hidden when there is nothing to show.
export function AutomaticAdjustments() {
  const { data, isLoading } = useCorporateActions();
  const items = data ?? [];
  if (isLoading || items.length === 0) {
    return null;
  }
  return (
    <Card className="mt-4 border-zinc-800 bg-zinc-950/40 p-5">
      <div className="flex items-baseline justify-between gap-3">
        <h2 className="portfolio-display text-lg font-semibold text-zinc-100">
          Automatic adjustments
        </h2>
        <span className="text-[11px] text-zinc-500">System-maintained · no action required</span>
      </div>
      <ul className="mt-3 divide-y divide-zinc-800/70">
        {items.map((item) => (
          <li key={item.id} className="flex items-start justify-between gap-4 py-2.5">
            <div className="min-w-0">
              <p className="text-sm font-medium text-zinc-200">{item.display_symbol}</p>
              <p className="mt-0.5 text-xs text-zinc-500">{item.explanation}</p>
            </div>
            <span className="shrink-0 rounded-full border border-zinc-700 px-2 py-0.5 text-[10px] uppercase tracking-wide text-zinc-400">
              {item.status}
            </span>
          </li>
        ))}
      </ul>
    </Card>
  );
}
