import { motion } from "framer-motion";

import { GlobalRankingWidget } from "@/pages/Dashboard/components/GlobalRankingWidget";
import { PerformanceChartCard } from "@/pages/Dashboard/components/PerformanceChartCard";
import { SprintWidget } from "@/pages/Dashboard/components/SprintWidget";
import { TrophyCaseWidget } from "@/pages/Dashboard/components/TrophyCaseWidget";
import type { useDashboard } from "@/hooks/useDashboard";

const containerVariants = {
  hidden: {},
  show: { transition: { staggerChildren: 0.08 } },
};

const cardVariants = {
  hidden: { opacity: 0, y: 18, scale: 0.98 },
  show: {
    opacity: 1,
    y: 0,
    scale: 1,
    transition: { type: "spring" as const, stiffness: 260, damping: 24 },
  },
};

type DashboardData = ReturnType<typeof useDashboard>;

export function DashboardBentoGrid(props: DashboardData) {
  const {
    portfolioSummary,
    leaderboardMe,
    currentSprint,
    sprintStatus,
    achievements,
    isLoading,
    errors,
    refetch,
  } = props;

  return (
    <motion.div
      variants={containerVariants}
      initial="hidden"
      animate="show"
      className="grid grid-cols-1 gap-5 lg:grid-cols-12"
    >
      {/* Performance chart — dominant left column */}
      <motion.div variants={cardVariants} className="lg:col-span-8">
        <PerformanceChartCard
          summary={portfolioSummary}
          isLoading={isLoading}
          isError={!!errors.summary}
          onRetry={refetch.summary}
        />
      </motion.div>

      {/* Sprint widget — right column */}
      <motion.div variants={cardVariants} className="lg:col-span-4">
        <SprintWidget
          sprint={currentSprint}
          sprintStatus={sprintStatus}
          isLoading={isLoading}
        />
      </motion.div>

      {/* Global ranking — secondary left */}
      <motion.div variants={cardVariants} className="lg:col-span-4">
        <GlobalRankingWidget
          leaderboardMe={leaderboardMe}
          isLoading={isLoading}
          isError={!!errors.leaderboard}
          onRetry={refetch.leaderboard}
        />
      </motion.div>

      {/* Trophy case — secondary right */}
      <motion.div variants={cardVariants} className="lg:col-span-8">
        <TrophyCaseWidget
          achievements={achievements}
          isLoading={isLoading}
          isError={!!errors.achievements}
        />
      </motion.div>
    </motion.div>
  );
}
