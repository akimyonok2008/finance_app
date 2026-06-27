import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";

import {
  getMyProfile,
  getPublicProfile,
  updateMyProfile,
} from "@/api/profile";
import { ApiError } from "@/api/client";
import { queryKeys } from "@/hooks/queryKeys";
import type { UpdateProfileRequest } from "@/types/profile";

export function useMyProfile() {
  return useQuery({
    queryKey: queryKeys.myProfile,
    queryFn: ({ signal }) => getMyProfile(signal),
  });
}

export function usePublicProfile(handle: string) {
  return useQuery({
    queryKey: queryKeys.publicProfile(handle),
    queryFn: ({ signal }) => getPublicProfile(handle, signal),
    enabled: handle.length > 0,
    retry: (failureCount, error) =>
      !(error instanceof ApiError && error.status === 404) && failureCount < 1,
  });
}

export function useUpdateProfile() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: UpdateProfileRequest) => updateMyProfile(input),
    onSuccess: (profile) => {
      queryClient.setQueryData(queryKeys.myProfile, profile);
      queryClient.invalidateQueries({
        queryKey: queryKeys.publicProfile(profile.handle),
      });
      toast.success("Profile updated");
    },
    onError: (error: Error) => toast.error(error.message),
  });
}
