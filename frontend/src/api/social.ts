import { apiRequest } from "@/api/client";
import type { FollowState, FriendsResponse, UserListResponse } from "@/types/social";

export function followUser(handle: string): Promise<FollowState> {
  return apiRequest<FollowState>(`/social/follows/${encodeURIComponent(handle)}`, {
    method: "POST",
  });
}

export function unfollowUser(handle: string): Promise<FollowState> {
  return apiRequest<FollowState>(`/social/follows/${encodeURIComponent(handle)}`, {
    method: "DELETE",
  });
}

export function getFollowState(handle: string, signal?: AbortSignal): Promise<FollowState> {
  return apiRequest<FollowState>(
    `/social/follow-state/${encodeURIComponent(handle)}`,
    { signal },
  );
}

export function getFriends(signal?: AbortSignal): Promise<FriendsResponse> {
  return apiRequest<FriendsResponse>("/social/friends", { signal });
}

export function getFollowing(signal?: AbortSignal): Promise<UserListResponse> {
  return apiRequest<UserListResponse>("/social/following", { signal });
}

export function getFollowers(signal?: AbortSignal): Promise<UserListResponse> {
  return apiRequest<UserListResponse>("/social/followers", { signal });
}
