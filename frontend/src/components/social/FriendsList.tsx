import { ArrowUpRight, MessageSquare } from "lucide-react";
import { Link, useNavigate } from "react-router-dom";

import { Button } from "@/components/ui/button";
import { useCreateConversation } from "@/hooks/useDM";
import type { FriendItem } from "@/types/social";
import { avatarInitials } from "@/utils/avatarInitials";

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
      <div className="rounded-2xl border border-dashed border-violet-300/10 bg-violet-300/[0.025] px-5 py-14 text-center">
        <p className="explore-display text-lg font-semibold text-zinc-300">Your network is quiet</p>
        <p className="mt-2 text-sm text-zinc-500">{empty}</p>
      </div>
    );
  }

  return (
    <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
      {friends.map((friend) => (
        <article
          key={friend.handle}
          className="group relative overflow-hidden rounded-xl border border-violet-300/15 bg-[radial-gradient(circle_at_top_left,rgba(167,139,250,0.07),transparent_44%),rgba(24,24,27,0.48)] p-4 transition duration-200 hover:-translate-y-0.5 hover:border-sky-300/25 hover:bg-zinc-900/75"
        >
          <div className="pointer-events-none absolute right-3 top-3 h-12 w-12 rounded-full bg-sky-400/[0.035] blur-xl" />
          <div className="flex items-start justify-between gap-3">
            <div className="relative flex min-w-0 items-center gap-3">
              <div className="flex h-9 w-9 shrink-0 items-center justify-center overflow-hidden rounded-lg border border-violet-300/20 bg-gradient-to-br from-violet-400/[0.13] to-sky-400/[0.07] font-mono text-[10px] font-medium uppercase text-violet-50">
                {avatarInitials(friend.avatar_key, friend.handle)}
              </div>
              <div className="min-w-0">
                <div className="truncate text-sm font-semibold text-zinc-100">{friend.display_name}</div>
                <div className="truncate font-mono text-[10px] text-zinc-500">@{friend.handle}</div>
              </div>
            </div>
            {friend.strategy_tag && (
              <span className="max-w-28 truncate rounded-full border border-violet-300/15 bg-violet-400/[0.04] px-2 py-1 text-[8px] font-medium uppercase tracking-wide text-violet-100/65">
                {friend.strategy_tag.replaceAll("_", " ")}
              </span>
            )}
          </div>

          <div className="relative mt-5 flex items-center justify-between gap-2 border-t border-violet-300/[0.08] pt-3">
            <Button asChild variant="ghost" size="sm" className="px-2 text-zinc-400 hover:text-zinc-100">
              <Link to={`/profiles/${friend.handle}`}>
                Profile <ArrowUpRight />
              </Link>
            </Button>
            {canMessage && (
              <div className="flex items-center gap-2">
                <span className="flex items-center gap-1 text-[8px] font-medium uppercase tracking-[0.14em] text-emerald-300/65">
                  <span className="h-1.5 w-1.5 rounded-full bg-emerald-300/80 shadow-[0_0_8px_rgba(110,231,183,0.45)]" />
                  Mutual
                </span>
                <Button
                  size="sm"
                  variant="outline"
                  className="border-sky-300/20 bg-sky-400/[0.06] text-sky-100 hover:bg-sky-400/[0.12]"
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
              </div>
            )}
          </div>
        </article>
      ))}
    </div>
  );
}
