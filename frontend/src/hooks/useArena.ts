import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";

import {
  getAchievements,
  getActiveSprint,
  getCohortLeaderboard,
  joinSprint,
} from "@/api/arenaApi";
import { useAuth } from "@/auth/useAuth";

export function useArena() {
  const { user } = useAuth();
  const queryClient = useQueryClient();

  const sprintQuery = useQuery({
    queryKey: ["sprintStatus"],
    queryFn: getActiveSprint,
    retry: 1,
  });
  const leaderboardQuery = useQuery({
    queryKey: ["leaderboard"],
    queryFn: getCohortLeaderboard,
    retry: 1,
  });
  const achievementsQuery = useQuery({
    queryKey: ["achievements"],
    queryFn: getAchievements,
    retry: 1,
  });
  const joinMutation = useMutation({
    mutationFn: joinSprint,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["sprintStatus"] });
      queryClient.invalidateQueries({ queryKey: ["leaderboard"] });
      queryClient.invalidateQueries({ queryKey: ["achievements"] });
      toast.success("You joined the sprint");
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const leaderboard = leaderboardQuery.data?.map((entry) => ({
    ...entry,
    isCurrentUser:
      entry.isCurrentUser ||
      Boolean(
        user?.display_name &&
          entry.username.toLowerCase() === user.display_name.toLowerCase(),
      ),
  }));

  return {
    sprint: sprintQuery.data,
    leaderboard,
    achievements: achievementsQuery.data,
    isLoading:
      sprintQuery.isLoading ||
      leaderboardQuery.isLoading ||
      achievementsQuery.isLoading,
    isError:
      sprintQuery.isError &&
      leaderboardQuery.isError &&
      achievementsQuery.isError,
    errors: {
      sprint: sprintQuery.error,
      leaderboard: leaderboardQuery.error,
      achievements: achievementsQuery.error,
    },
    joinSprint: joinMutation.mutate,
    isJoiningSprint: joinMutation.isPending,
    refetch: async () => {
      await Promise.all([
        sprintQuery.refetch(),
        leaderboardQuery.refetch(),
        achievementsQuery.refetch(),
      ]);
    },
  };
}
