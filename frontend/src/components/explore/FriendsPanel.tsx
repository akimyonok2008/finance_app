import { RefreshCw, ShieldCheck } from "lucide-react";
import { useState } from "react";

import { FriendsList } from "@/components/social/FriendsList";
import { Button } from "@/components/ui/button";
import { useFollowers, useFollowing, useFriends } from "@/hooks/useSocial";
import { cn } from "@/utils/cn";

const RELATIONSHIP_TABS = ["Friends", "Following", "Followers"] as const;
type RelationshipTab = (typeof RELATIONSHIP_TABS)[number];

export function FriendsPanel() {
  const [tab, setTab] = useState<RelationshipTab>("Friends");
  const friends = useFriends();
  const following = useFollowing();
  const followers = useFollowers();
  const activeQuery =
    tab === "Friends" ? friends : tab === "Following" ? following : followers;
  const items =
    tab === "Friends"
      ? friends.data?.friends ?? []
      : tab === "Following"
        ? following.data?.users ?? []
        : followers.data?.users ?? [];

  return (
    <section className="space-y-5">
      <div className="flex flex-col justify-between gap-3 sm:flex-row sm:items-start">
        <div>
          <h2 className="text-lg font-semibold text-zinc-100">Friends</h2>
          <p className="mt-1 max-w-2xl text-sm text-zinc-400">
            Follow public profiles. When they follow you back, you can message each other.
          </p>
        </div>
        <button
          type="button"
          onClick={() => activeQuery.refetch()}
          disabled={activeQuery.isFetching}
          aria-label="Refresh friends"
          className="self-start rounded-lg border border-zinc-800 p-2 text-zinc-400 transition hover:bg-zinc-800/70 hover:text-zinc-100 disabled:opacity-50"
        >
          <RefreshCw
            className={cn("h-3.5 w-3.5", activeQuery.isFetching && "animate-spin")}
          />
        </button>
      </div>

      <div className="flex flex-wrap gap-2">
        {RELATIONSHIP_TABS.map((item) => (
          <button
            key={item}
            type="button"
            onClick={() => setTab(item)}
            className={cn(
              "rounded-lg border px-3 py-2 text-sm transition",
              tab === item
                ? "border-zinc-500 bg-zinc-100 text-zinc-950"
                : "border-zinc-800 bg-zinc-900/35 text-zinc-400 hover:text-zinc-100",
            )}
          >
            {item}
          </button>
        ))}
      </div>

      {activeQuery.isLoading ? (
        <div className="rounded-xl border border-zinc-800 bg-zinc-900/35 p-8 text-sm text-zinc-500">
          Loading relationships...
        </div>
      ) : activeQuery.isError ? (
        <div className="rounded-xl border border-rose-400/15 bg-rose-400/[0.04] p-8 text-center">
          <h3 className="font-semibold text-zinc-100">Could not load friends.</h3>
          <Button variant="outline" className="mt-4" onClick={() => activeQuery.refetch()}>
            <RefreshCw /> Retry
          </Button>
        </div>
      ) : (
        <FriendsList
          friends={items}
          canMessage={tab === "Friends"}
          empty={
            tab === "Friends"
              ? "You do not have mutual friends yet."
              : `No ${tab.toLowerCase()} yet.`
          }
        />
      )}

      <div className="flex items-center gap-2 rounded-xl border border-zinc-800 bg-zinc-900/30 px-4 py-3 text-xs text-zinc-500">
        <ShieldCheck className="h-3.5 w-3.5 shrink-0" />
        Friends and messages never show portfolio value, quantities, cost basis, or email.
      </div>
    </section>
  );
}
