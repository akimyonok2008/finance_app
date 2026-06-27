import { ShieldCheck } from "lucide-react";

import { COACH_DISCLAIMER_FALLBACK } from "@/types/coach";

/** Always-visible, clear but non-alarming educational disclaimer. */
export function CoachDisclaimer({ text }: { text?: string }) {
  return (
    <div className="flex items-start gap-2 border-t border-zinc-800 pt-3 text-[11px] leading-relaxed text-zinc-500">
      <ShieldCheck className="mt-px h-3.5 w-3.5 shrink-0 text-zinc-600" />
      <span>{text || COACH_DISCLAIMER_FALLBACK}</span>
    </div>
  );
}
