import { RefreshCw, UserCheck, UserPlus, UsersRound } from "lucide-react";
import { useState } from "react";

import { FriendsList } from "@/components/social/FriendsList";
import { Button } from "@/components/ui/button";
import { useFollowers, useFollowing, useFriends } from "@/hooks/useSocial";
import { cn } from "@/utils/cn";

const RELATIONSHIP_TABS = ["Friends", "Following", "Followers"] as const;
type RelationshipTab = (typeof RELATIONSHIP_TABS)[number];
const RELATIONSHIP_ICONS = {
  Friends: UserCheck,
  Following: UserPlus,
  Followers: UsersRound,
} satisfies Record<RelationshipTab, typeof UsersRound>;
const RELATIONSHIP_TONES: Record<RelationshipTab, { card: string; active: string; count: string }> = {
  Friends: {
    card: "border-violet-300/15 bg-violet-400/[0.05]",
    active: "border-violet-300/20 bg-violet-400/[0.12] text-violet-50",
    count: "text-violet-200",
  },
  Following: {
    card: "border-sky-300/15 bg-sky-400/[0.045]",
    active: "border-sky-300/20 bg-sky-400/[0.11] text-sky-50",
    count: "text-sky-200",
  },
  Followers: {
    card: "border-emerald-300/15 bg-emerald-400/[0.045]",
    active: "border-emerald-300/20 bg-emerald-400/[0.1] text-emerald-50",
    count: "text-emerald-200",
  },
};

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
  const counts: Record<RelationshipTab, number> = {
    Friends: friends.data?.friends.length ?? 0,
    Following: following.data?.users.length ?? 0,
    Followers: followers.data?.users.length ?? 0,
  };

  return (
    <section className="space-y-6">
      <div className="relative overflow-hidden rounded-2xl border border-violet-300/15 bg-[radial-gradient(circle_at_8%_18%,rgba(167,139,250,0.13),transparent_30%),radial-gradient(circle_at_88%_22%,rgba(56,189,248,0.09),transparent_28%),rgba(24,24,27,0.42)] p-5 shadow-[0_24px_70px_rgba(0,0,0,0.18)] sm:p-6">
        <div className="pointer-events-none absolute -right-8 -top-10 h-32 w-32 rounded-full border border-violet-300/[0.07]" />
        <div className="relative flex flex-col justify-between gap-5 sm:flex-row sm:items-end">
          <div>
            <div className="text-[9px] font-medium uppercase tracking-[0.2em] text-violet-200/55">
              Your network
            </div>
            <h2 className="explore-display mt-2 text-2xl font-semibold text-zinc-100">
              Relationships with context.
            </h2>
            <p className="mt-2 max-w-xl text-sm leading-6 text-zinc-400">
              Keep track of the public investors you follow and continue conversations with mutual connections.
            </p>
          </div>
          <div className="grid grid-cols-3 gap-2">
            {RELATIONSHIP_TABS.map((item) => (
              <div key={item} className={cn("min-w-20 rounded-xl border px-3 py-2 text-center backdrop-blur-sm", RELATIONSHIP_TONES[item].card)}>
                <div className={cn("font-mono text-sm font-medium tabular-nums", RELATIONSHIP_TONES[item].count)}>{counts[item]}</div>
                <div className="mt-0.5 text-[8px] uppercase tracking-[0.14em] text-zinc-500">{item}</div>
              </div>
            ))}
          </div>
        </div>
      </div>

      <div className="flex items-center justify-between gap-3 border-b border-zinc-800/80 pb-3">
        <div className="inline-flex max-w-full gap-1 overflow-x-auto rounded-xl border border-zinc-800 bg-zinc-900/45 p-1">
          {RELATIONSHIP_TABS.map((item) => {
            const Icon = RELATIONSHIP_ICONS[item];
            return (
              <button
                key={item}
                type="button"
                onClick={() => setTab(item)}
                className={cn(
                  "flex items-center gap-2 rounded-lg px-3 py-2 text-sm font-medium transition",
                  tab === item
                    ? cn("border shadow-sm", RELATIONSHIP_TONES[item].active)
                    : "text-zinc-400 hover:bg-zinc-800/70 hover:text-zinc-100",
                )}
              >
                <Icon className="h-3.5 w-3.5" />
                {item}
                <span className={cn("font-mono text-[9px]", tab === item ? RELATIONSHIP_TONES[item].count : "text-zinc-600")}>
                  {counts[item]}
                </span>
              </button>
            );
          })}
        </div>
        <button
          type="button"
          onClick={() => activeQuery.refetch()}
          disabled={activeQuery.isFetching}
          aria-label="Refresh friends"
          className="rounded-lg border border-sky-300/10 bg-sky-400/[0.035] p-2.5 text-sky-200/60 transition hover:border-sky-300/25 hover:bg-sky-400/[0.07] hover:text-sky-100 disabled:opacity-50"
        >
          <RefreshCw
            className={cn("h-3.5 w-3.5", activeQuery.isFetching && "animate-spin")}
          />
        </button>
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
    </section>
  );
}
