import { LockKeyhole } from "lucide-react";

export function ExplorePrivacyNote() {
  return (
    <div className="flex items-start gap-2 rounded-xl border border-zinc-800 bg-zinc-900/30 px-4 py-3 text-[11px] leading-5 text-zinc-500">
      <LockKeyhole className="mt-0.5 h-3.5 w-3.5 shrink-0" />
      Explore shows public symbols and percentage weights only. Quantities, values, cost basis, and buy prices stay private.
    </div>
  );
}
