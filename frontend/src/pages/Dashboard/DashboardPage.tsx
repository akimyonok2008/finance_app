import { motion } from "framer-motion";
import { LayoutDashboard, LogOut, RefreshCw, Swords, WalletCards } from "lucide-react";
import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";

import { useAuth } from "@/auth/useAuth";
import { DashboardBentoGrid } from "@/pages/Dashboard/components/DashboardBentoGrid";
import { DashboardEmptyState } from "@/pages/Dashboard/components/DashboardEmptyState";
import { DashboardSkeleton } from "@/pages/Dashboard/components/DashboardSkeleton";
import { useDashboard } from "@/hooks/useDashboard";

export function DashboardPage() {
  const { user, logout } = useAuth();
  const navigate = useNavigate();
  const dashboard = useDashboard();
  const { isLoading, portfolioSummary } = dashboard;
  const [isRefreshing, setIsRefreshing] = useState(false);

  const handleLogout = () => {
    logout();
    navigate("/login");
  };

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
    <div className="min-h-screen overflow-hidden bg-zinc-950 text-zinc-50">
      <main className="mx-auto w-full max-w-7xl px-4 py-4 sm:px-6 lg:px-8">
        <nav className="mb-8 flex items-center justify-between rounded-xl border border-zinc-800 bg-zinc-900/40 p-1">
          <div className="flex items-center gap-1">
            <Link
              to="/dashboard"
              className="flex items-center gap-2 rounded-lg bg-zinc-50 px-3 py-2 text-xs font-medium text-zinc-950"
            >
              <LayoutDashboard className="h-3.5 w-3.5" />
              Dashboard
            </Link>
            <Link
              to="/portfolio"
              className="flex items-center gap-2 rounded-lg px-3 py-2 text-xs font-medium text-zinc-400 transition hover:bg-zinc-800/70 hover:text-zinc-100"
            >
              <WalletCards className="h-3.5 w-3.5" />
              Portfolio
            </Link>
            <Link
              to="/arena"
              className="flex items-center gap-2 rounded-lg px-3 py-2 text-xs font-medium text-zinc-400 transition hover:bg-zinc-800/70 hover:text-zinc-100"
            >
              <Swords className="h-3.5 w-3.5" />
              Arena
            </Link>
          </div>
          <div className="flex items-center gap-1">
            <button
              type="button"
              onClick={handleRefresh}
              disabled={isRefreshing}
              aria-label="Refresh dashboard"
              className="rounded-lg p-2 text-zinc-400 transition hover:bg-zinc-800/70 hover:text-zinc-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-500 disabled:opacity-50"
            >
              <RefreshCw className={`h-3.5 w-3.5 ${isRefreshing ? "animate-spin" : ""}`} />
            </button>
            <button
              type="button"
              onClick={handleLogout}
              aria-label="Sign out"
              className="rounded-lg p-2 text-zinc-400 transition hover:bg-zinc-800/70 hover:text-zinc-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-500"
            >
              <LogOut className="h-3.5 w-3.5" />
            </button>
          </div>
        </nav>

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
              {user?.display_name
                ? `Welcome back, ${user.display_name}. `
                : ""}
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
