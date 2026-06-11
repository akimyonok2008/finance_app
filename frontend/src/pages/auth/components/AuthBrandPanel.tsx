import { motion } from "framer-motion";
import { Activity, ShieldCheck, TrendingUp } from "lucide-react";

function PreviewStat({
  label,
  value,
  delta,
  icon: Icon,
}: {
  label: string;
  value: string;
  delta?: string;
  icon: React.ElementType;
}) {
  return (
    <div className="flex items-center gap-3 rounded-xl border border-zinc-800 bg-zinc-900/40 px-4 py-3">
      <div className="grid h-8 w-8 shrink-0 place-items-center rounded-lg border border-zinc-800 bg-zinc-950/60">
        <Icon className="h-4 w-4 text-zinc-400" />
      </div>
      <div className="min-w-0 flex-1">
        <div className="text-[11px] uppercase tracking-widest text-zinc-500">
          {label}
        </div>
        <div className="tabular-nums text-sm font-semibold text-zinc-100">
          {value}
        </div>
      </div>
      {delta && (
        <span className="shrink-0 text-xs font-medium tabular-nums text-emerald-400">
          {delta}
        </span>
      )}
    </div>
  );
}

export function AuthBrandPanel() {
  return (
    <section
      className="relative hidden overflow-hidden border-r border-zinc-800 bg-[#0a0a0a] lg:flex lg:flex-col lg:items-center lg:justify-center lg:px-14"
    >
      <div
        className="pointer-events-none absolute inset-0 opacity-25"
        style={{
          backgroundImage:
            "linear-gradient(rgba(255,255,255,0.035) 1px, transparent 1px), linear-gradient(90deg, rgba(255,255,255,0.035) 1px, transparent 1px)",
          backgroundSize: "48px 48px",
        }}
      />

      {/* Content */}
      <div className="relative z-10 flex w-full max-w-sm flex-col gap-10">
        {/* Logo mark */}
        <motion.div
          initial={{ opacity: 0, y: -10 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.1, duration: 0.6 }}
          className="flex items-center gap-2.5"
        >
          <div className="grid h-9 w-9 place-items-center rounded-lg border border-zinc-800 bg-zinc-900/60">
            <ShieldCheck className="h-4 w-4 text-zinc-300" />
          </div>
          <span className="text-sm font-semibold tracking-wide text-zinc-200">
            Portfolio Arena
          </span>
        </motion.div>

        <div>
          <h2 className="text-3xl font-medium tracking-tight text-zinc-50">
            Private portfolio tracking.
          </h2>
          <p className="mt-3 text-sm leading-6 text-zinc-400">
            Compete on performance, never wealth.
          </p>
        </div>

        {/* Preview stat hints */}
        <motion.div
          initial={{ opacity: 0, y: 12 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.2, duration: 0.3 }}
          className="flex w-full flex-col gap-3"
        >
          <PreviewStat
            label="Portfolio Index"
            value="114.28"
            delta="+14.28%"
            icon={Activity}
          />
          <PreviewStat
            label="Sprint Rank"
            value="#3 of 241"
            icon={TrendingUp}
          />
          <PreviewStat
            label="Leaderboard"
            value="Anonymous"
            icon={ShieldCheck}
          />
        </motion.div>
      </div>
    </section>
  );
}
