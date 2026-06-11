import { motion } from "framer-motion";
import { BarChart2 } from "lucide-react";
import { Link } from "react-router-dom";

export function DashboardEmptyState() {
  return (
    <motion.div
      initial={{ opacity: 0, y: 12 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.18 }}
      className="mb-5 flex flex-col items-center justify-between gap-5 rounded-2xl border border-zinc-800 bg-zinc-900/40 px-6 py-5 text-center sm:flex-row sm:text-left"
    >
      <div className="grid h-12 w-12 place-items-center rounded-xl border border-zinc-800 bg-zinc-950/50 text-zinc-400">
        <BarChart2 className="h-5 w-5" />
      </div>
      <div className="flex-1 space-y-1">
        <h2 className="text-base font-medium tracking-tight">
          No positions yet
        </h2>
        <p className="mx-auto max-w-sm text-sm text-slate-400">
          Add your first position to begin.
        </p>
      </div>
      <Link
        to="/portfolio"
        className="inline-flex h-10 shrink-0 items-center gap-2 rounded-lg bg-zinc-50 px-5 text-sm font-medium text-zinc-950 transition hover:bg-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-500"
      >
        Go to Portfolio
      </Link>
    </motion.div>
  );
}
