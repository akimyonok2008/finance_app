import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";

import {
  followUser,
  getFollowers,
  getFollowing,
  getFollowState,
  getFriends,
  unfollowUser,
} from "@/api/social";
import { queryKeys } from "@/hooks/queryKeys";

export function useFollowState(handle: string) {
  return useQuery({
    queryKey: queryKeys.followState(handle),
    queryFn: ({ signal }) => getFollowState(handle, signal),
    enabled: handle.length > 0,
  });
}

export function useFriends() {
  return useQuery({
    queryKey: queryKeys.friends,
    queryFn: ({ signal }) => getFriends(signal),
  });
}

export function useFollowing() {
  return useQuery({
    queryKey: queryKeys.following,
    queryFn: ({ signal }) => getFollowing(signal),
  });
}

export function useFollowers() {
  return useQuery({
    queryKey: queryKeys.followers,
    queryFn: ({ signal }) => getFollowers(signal),
  });
}

export function useFollowMutation(handle: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => followUser(handle),
    onSuccess: (state) => {
      invalidateSocial(queryClient, state.handle);
      toast.success(state.is_friend ? "You are now friends" : "Following profile");
    },
    onError: (error: Error) => toast.error(error.message),
  });
}

export function useUnfollowMutation(handle: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => unfollowUser(handle),
    onSuccess: (state) => {
      invalidateSocial(queryClient, state.handle);
      toast.success("Unfollowed profile");
    },
    onError: (error: Error) => toast.error(error.message),
  });
}

function invalidateSocial(
  queryClient: ReturnType<typeof useQueryClient>,
  handle: string,
) {
  queryClient.invalidateQueries({ queryKey: queryKeys.followState(handle) });
  queryClient.invalidateQueries({ queryKey: queryKeys.friends });
  queryClient.invalidateQueries({ queryKey: queryKeys.following });
  queryClient.invalidateQueries({ queryKey: queryKeys.followers });
  queryClient.invalidateQueries({ queryKey: queryKeys.publicProfile(handle) });
  queryClient.invalidateQueries({ queryKey: queryKeys.dmConversations });
}
