import { motion } from "framer-motion";
import { RefreshCw } from "lucide-react";
import { useState } from "react";

import { useAuth } from "@/auth/useAuth";
import { AppNav } from "@/components/layout/AppNav";
import { DashboardBentoGrid } from "@/pages/Dashboard/components/DashboardBentoGrid";
import { DashboardEmptyState } from "@/pages/Dashboard/components/DashboardEmptyState";
import { DashboardSkeleton } from "@/pages/Dashboard/components/DashboardSkeleton";
import { useDashboard } from "@/hooks/useDashboard";

export function DashboardPage() {
  const { user } = useAuth();
  const dashboard = useDashboard();
  const { isLoading, portfolioSummary } = dashboard;
  const [isRefreshing, setIsRefreshing] = useState(false);

  const handleRefresh = async () => {
    setIsRefreshing(true);
    await dashboard.refetch.all();
    setIsRefreshing(false);
  };

  const isEmpty =
    !isLoading &&
    (!portfolioSummary ||
      (portfolioSummary.current_value === 0 &&
        portfolioSummary.total_cost_basis === 0));

  return (
    <div className="min-h-screen bg-zinc-950 text-zinc-50">
      <main className="mx-auto w-full max-w-7xl px-4 py-4 sm:px-6 lg:px-8">
        <AppNav
          actions={
            <button
              type="button"
              onClick={handleRefresh}
              disabled={isRefreshing}
              aria-label="Refresh dashboard"
              className="rounded-lg p-2 text-zinc-400 transition hover:bg-zinc-800/70 hover:text-zinc-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-500 disabled:opacity-50"
            >
              <RefreshCw
                className={`h-3.5 w-3.5 ${isRefreshing ? "animate-spin" : ""}`}
              />
            </button>
          }
        />

        {/* Page header */}
        <motion.div
          initial={{ opacity: 0, y: -10 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.4 }}
          className="mb-8 flex items-start justify-between gap-4"
        >
          <div>
            <div className="flex items-center gap-3">
              <h1 className="text-2xl font-medium tracking-tight sm:text-3xl">
                Dashboard
              </h1>
              <span className="rounded-md border border-zinc-800 bg-zinc-900/60 px-2 py-1 text-[11px] text-zinc-500">
                Private beta
              </span>
            </div>
            <p className="mt-1 text-sm text-zinc-400">
              {user?.display_name ? `Welcome back, ${user.display_name}. ` : ""}
              Portfolio performance and rankings.
            </p>
          </div>
        </motion.div>

        {/* Content */}
        {isLoading ? (
          <DashboardSkeleton />
        ) : (
          <>
            {isEmpty && <DashboardEmptyState />}
            <DashboardBentoGrid {...dashboard} />
          </>
        )}
      </main>
    </div>
  );
}
