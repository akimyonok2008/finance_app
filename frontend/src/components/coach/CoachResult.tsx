import { motion } from "framer-motion";

import { CoachComparisonPanel } from "@/components/coach/CoachComparisonPanel";
import { CoachDisclaimer } from "@/components/coach/CoachDisclaimer";
import { CoachObservationList } from "@/components/coach/CoachObservationList";
import { riskBadgeClass } from "@/components/coach/coachStyles";
import { cn } from "@/utils/cn";
import type { CoachResponse } from "@/types/coach";

function NoteSection({ title, items }: { title: string; items?: string[] }) {
  if (!items || items.length === 0) return null;
  return (
    <section>
      <h4 className="mb-1.5 text-xs font-medium uppercase tracking-wide text-zinc-500">
        {title}
      </h4>
      <ul className="space-y-1">
        {items.map((t, i) => (
          <li
            key={i}
            className="flex gap-2 text-sm leading-relaxed text-zinc-400"
          >
            <span className="mt-1.5 h-1 w-1 shrink-0 rounded-full bg-zinc-600" />
            <span className="min-w-0 break-words">{t}</span>
          </li>
        ))}
      </ul>
    </section>
  );
}

function formatTime(iso?: string): string | null {
  if (!iso) return null;
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return null;
  return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

export function CoachResult({ data }: { data: CoachResponse }) {
  const time = formatTime(data.generated_at);
  const showComparison =
    data.top10_comparison !== undefined &&
    (data.mode === "compare_top10" ||
      data.top10_comparison.available);

  return (
    <motion.div
      initial={{ opacity: 0, y: 6 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.25 }}
      className="space-y-4"
    >
      {/* Header */}
      <div className="flex flex-wrap items-center gap-2">
        <h3 className="text-base font-medium text-zinc-100">{data.title}</h3>
        {data.risk_level && (
          <span
            className={cn(
              "rounded border px-1.5 py-0.5 text-[11px] font-medium uppercase tracking-wide",
              riskBadgeClass(data.risk_level),
            )}
          >
            {data.risk_level} risk
          </span>
        )}
        {time && (
          <span className="ml-auto font-mono text-xs tabular-nums text-zinc-600">
            {time}
          </span>
        )}
      </div>

      {/* Summary */}
      {data.summary && (
        <p className="text-sm leading-relaxed text-zinc-300">{data.summary}</p>
      )}

      {/* Top-10 comparison */}
      {showComparison && (
        <div className="space-y-1.5">
          <h4 className="text-xs font-medium uppercase tracking-wide text-zinc-500">
            Top-10 comparison
          </h4>
          <CoachComparisonPanel comparison={data.top10_comparison} />
        </div>
      )}

      {/* Observations */}
      {data.observations && data.observations.length > 0 && (
        <div className="space-y-1.5">
          <h4 className="text-xs font-medium uppercase tracking-wide text-zinc-500">
            Observations
          </h4>
          <CoachObservationList observations={data.observations} />
        </div>
      )}

      <NoteSection title="Technical notes" items={data.technical_notes} />
      <NoteSection title="Fundamental notes" items={data.fundamental_notes} />
      <NoteSection title="Learning points" items={data.learning_points} />
      <NoteSection
        title="Questions to consider"
        items={data.questions_to_consider}
      />

      <CoachDisclaimer text={data.disclaimer} />
    </motion.div>
  );
}
