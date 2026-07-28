import { MessageSquare, UserCheck, UserPlus, UserX } from "lucide-react";
import { useNavigate } from "react-router-dom";

import { Button } from "@/components/ui/button";
import { BlockButton } from "@/components/social/BlockButton";
import { useCreateConversation } from "@/hooks/useDM";
import {
  useFollowMutation,
  useFollowState,
  useUnfollowMutation,
} from "@/hooks/useSocial";

type Props = {
  handle: string;
  isSelf?: boolean;
};

export function FollowButton({ handle, isSelf }: Props) {
  const navigate = useNavigate();
  const state = useFollowState(handle);
  const follow = useFollowMutation(handle);
  const unfollow = useUnfollowMutation(handle);
  const createConversation = useCreateConversation();
  const busy = follow.isPending || unfollow.isPending || createConversation.isPending;
  const relation = state.data;

  if (isSelf) {
    return (
      <Button variant="outline" size="sm" disabled>
        Your profile
      </Button>
    );
  }

  const startMessage = async () => {
    const resp = await createConversation.mutateAsync(handle);
    navigate(
      `/explore?tab=messages&conversationId=${encodeURIComponent(resp.conversation.id)}`,
    );
  };

  return (
    <div className="flex flex-wrap items-center justify-end gap-2">
      {relation?.is_friend ? (
        <Button size="sm" onClick={startMessage} disabled={busy}>
          <MessageSquare /> Message
        </Button>
      ) : (
        <p className="max-w-[220px] text-right text-xs text-zinc-500">
          You can message each other after you both follow.
        </p>
      )}

      {relation?.is_following ? (
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => unfollow.mutate()}
          disabled={busy || state.isLoading}
          className="border-rose-400/20 text-rose-200 hover:bg-rose-400/10 hover:text-rose-100"
        >
          {relation.is_friend ? <UserCheck /> : <UserX />}
          {relation.is_friend ? "Friends" : "Following"}
        </Button>
      ) : (
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => follow.mutate()}
          disabled={busy || state.isLoading}
        >
          <UserPlus /> Follow
        </Button>
      )}
      <BlockButton handle={handle} />
    </div>
  );
}
