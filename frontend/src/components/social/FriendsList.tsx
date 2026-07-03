import { MessageSquare, UserRound } from "lucide-react";
import { Link, useNavigate } from "react-router-dom";

import { Button } from "@/components/ui/button";
import { useCreateConversation } from "@/hooks/useDM";
import type { FriendItem } from "@/types/social";

type Props = {
  friends: FriendItem[];
  empty: string;
  canMessage?: boolean;
};

export function FriendsList({ friends, empty, canMessage = false }: Props) {
  const navigate = useNavigate();
  const createConversation = useCreateConversation();

  if (friends.length === 0) {
    return (
      <div className="rounded-xl border border-zinc-800 bg-zinc-900/35 px-5 py-10 text-center text-sm text-zinc-500">
        {empty}
      </div>
    );
  }

  return (
    <div className="grid gap-3 md:grid-cols-2">
      {friends.map((friend) => (
        <div
          key={friend.handle}
          className="rounded-xl border border-zinc-800 bg-zinc-900/35 p-4"
        >
          <div className="flex items-start justify-between gap-3">
            <div className="flex items-center gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-full border border-zinc-800 bg-zinc-950 text-xs font-semibold uppercase text-zinc-300">
                {friend.avatar_key || friend.handle.slice(0, 2)}
              </div>
              <div>
                <div className="font-medium text-zinc-100">{friend.display_name}</div>
                <div className="text-xs text-zinc-500">@{friend.handle}</div>
              </div>
            </div>
            {friend.strategy_tag && (
              <span className="rounded-full border border-zinc-800 px-2 py-1 text-[10px] uppercase text-zinc-500">
                {friend.strategy_tag.replaceAll("_", " ")}
              </span>
            )}
          </div>

          <div className="mt-4 flex flex-wrap gap-2">
            <Button asChild variant="outline" size="sm">
              <Link to={`/profiles/${friend.handle}`}>
                <UserRound /> Profile
              </Link>
            </Button>
            {canMessage && (
              <Button
                size="sm"
                onClick={async () => {
                  const resp = await createConversation.mutateAsync(friend.handle);
                  navigate(
                    `/explore?tab=messages&conversationId=${encodeURIComponent(resp.conversation.id)}`,
                  );
                }}
                disabled={createConversation.isPending}
              >
                <MessageSquare /> Message
              </Button>
            )}
          </div>
        </div>
      ))}
    </div>
  );
}
