import { motion } from "framer-motion";

import { AppNav } from "@/components/layout/AppNav";
import { PortfolioCoachCard } from "@/components/coach/PortfolioCoachCard";
import { useAuth } from "@/auth/useAuth";

export function CoachPage() {
  const { user } = useAuth();

  return (
    <div className="min-h-screen bg-zinc-950 text-zinc-50">
      <main className="mx-auto w-full max-w-7xl px-4 pb-16 pt-4 sm:px-6 lg:px-8">
        <AppNav />

        <div className="mx-auto max-w-3xl">
          {/* Page header */}
          <motion.div
            initial={{ opacity: 0, y: -10 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.4 }}
            className="mb-8"
          >
            <div className="flex items-center gap-3">
              <h1 className="text-2xl font-medium tracking-tight sm:text-3xl">
                Portfolio Coach
              </h1>
              <span className="rounded-md border border-zinc-800 bg-zinc-900/60 px-2 py-1 text-[11px] text-zinc-500">
                Analysis only
              </span>
            </div>
            <p className="mt-1 text-sm text-zinc-400">
              {user?.display_name ? `${user.display_name}'s ` : "Your "}
              private portfolio analysis.
            </p>
          </motion.div>

          <PortfolioCoachCard />
        </div>
      </main>
    </div>
  );
}
