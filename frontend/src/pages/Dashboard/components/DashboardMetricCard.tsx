import { cn } from "@/utils/cn";

type Tone = "default" | "positive" | "negative" | "violet";

const toneValue: Record<Tone, string> = {
  default: "text-slate-50",
  positive: "text-emerald-400",
  negative: "text-rose-400",
  violet: "text-violet-300",
};

export type DashboardMetricCardProps = {
  label: string;
  value: string;
  helper?: string;
  icon?: React.ReactNode;
  tone?: Tone;
  className?: string;
};

export function DashboardMetricCard({
  label,
  value,
  helper,
  icon,
  tone = "default",
  className,
}: DashboardMetricCardProps) {
  return (
    <div
      className={cn(
        "flex items-center gap-3 rounded-2xl border border-white/[0.07] bg-white/[0.03] px-4 py-3",
        className,
      )}
    >
      {icon && (
        <div className="grid h-8 w-8 shrink-0 place-items-center rounded-xl border border-white/10 bg-slate-900/60 text-slate-400">
          {icon}
        </div>
      )}
      <div className="min-w-0">
        <div className="text-[11px] uppercase tracking-widest text-slate-500">
          {label}
        </div>
        <div
          className={cn(
            "truncate text-sm font-semibold tabular-nums",
            toneValue[tone],
          )}
        >
          {value}
        </div>
        {helper && (
          <div className="text-[11px] text-slate-500">{helper}</div>
        )}
      </div>
    </div>
  );
}
