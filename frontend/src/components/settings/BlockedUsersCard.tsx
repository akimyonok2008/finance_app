import { ShieldOff, Users } from "lucide-react";

import { Button } from "@/components/ui/button";
import { useBlockedUsers, useUnblockUser } from "@/hooks/useSafety";

/** Settings-page card listing accounts the user has blocked, with unblock. */
export function BlockedUsersCard() {
  const blocked = useBlockedUsers();
  const unblock = useUnblockUser();

  return (
    <div className="rounded-2xl border border-zinc-800 bg-zinc-900/40 p-5">
      <div className="mb-3 flex items-center gap-2 text-sm font-semibold text-zinc-100">
        <ShieldOff className="h-4 w-4 text-rose-300/70" />
        Blocked users
      </div>

      {blocked.isLoading ? (
        <p className="text-sm text-zinc-500">Loading…</p>
      ) : blocked.isError ? (
        <p className="text-sm text-rose-300">Could not load blocked users.</p>
      ) : !blocked.data || blocked.data.blocked_users.length === 0 ? (
        <div className="flex items-center gap-2 text-sm text-zinc-500">
          <Users className="h-4 w-4" />
          You haven't blocked anyone.
        </div>
      ) : (
        <ul className="space-y-2">
          {blocked.data.blocked_users.map((item) => (
            <li
              key={item.handle}
              className="flex items-center justify-between rounded-lg border border-zinc-800/80 bg-zinc-950/40 px-3 py-2"
            >
              <div>
                <div className="text-sm font-medium text-zinc-100">{item.display_name}</div>
                <div className="text-xs text-zinc-500">@{item.handle}</div>
              </div>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => unblock.mutate(item.handle)}
                disabled={unblock.isPending}
              >
                Unblock
              </Button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
