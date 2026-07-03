export type SafeProfile = {
  handle: string;
  display_name: string;
  avatar_key?: string;
  strategy_tag?: string;
};

export type FollowState = {
  handle: string;
  is_following: boolean;
  follows_me: boolean;
  is_friend: boolean;
};

export type FriendItem = SafeProfile & {
  friends_since: string;
};

export type FriendsResponse = {
  friends: FriendItem[];
};

export type UserListResponse = {
  users: FriendItem[];
};
