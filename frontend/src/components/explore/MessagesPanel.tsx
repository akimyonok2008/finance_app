import { ArrowLeft, MessageCircle, RefreshCw, ShieldCheck } from "lucide-react";
import { useEffect, useMemo } from "react";
import { useSearchParams } from "react-router-dom";

import { ConversationList } from "@/components/dm/ConversationList";
import { MessageComposer } from "@/components/dm/MessageComposer";
import { MessageThread } from "@/components/dm/MessageThread";
import { Button } from "@/components/ui/button";
import { useConversations, useMessages } from "@/hooks/useDM";
import { useMarkConversationRead } from "@/hooks/useSafety";
import { avatarInitials } from "@/utils/avatarInitials";
import { cn } from "@/utils/cn";

export function MessagesPanel() {
  const [searchParams, setSearchParams] = useSearchParams();
  const selectedId = searchParams.get("conversationId");
  const conversations = useConversations();
  const messages = useMessages(selectedId);
  const markRead = useMarkConversationRead(selectedId);

  // Record a server-side read boundary whenever the open conversation's
  // latest message changes — opening the page alone must not silently clear
  // unread state without this call.
  const latestMessageId = messages.data?.messages.at(-1)?.id;
  useEffect(() => {
    if (selectedId && latestMessageId) {
      markRead.mutate(latestMessageId);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedId, latestMessageId]);

  const selected = useMemo(
    () => conversations.data?.conversations.find((item) => item.id === selectedId),
    [conversations.data?.conversations, selectedId],
  );

  const selectConversation = (id: string) => {
    const next = new URLSearchParams(searchParams);
    next.set("tab", "messages");
    next.set("conversationId", id);
    setSearchParams(next);
  };

  const clearConversation = () => {
    const next = new URLSearchParams(searchParams);
    next.set("tab", "messages");
    next.delete("conversationId");
    setSearchParams(next);
  };

  return (
    <section className="space-y-6">
      <div className="relative overflow-hidden rounded-2xl border border-sky-300/15 bg-[radial-gradient(circle_at_12%_20%,rgba(56,189,248,0.13),transparent_30%),radial-gradient(circle_at_88%_24%,rgba(129,140,248,0.1),transparent_28%),rgba(24,24,27,0.42)] p-5 shadow-[0_24px_70px_rgba(0,0,0,0.18)] sm:p-6">
        <div className="pointer-events-none absolute -right-6 -top-8 h-32 w-32 rounded-full border border-sky-300/[0.08]" />
        <div className="relative flex flex-col justify-between gap-5 sm:flex-row sm:items-end">
          <div>
            <div className="flex items-center gap-2 text-[9px] font-medium uppercase tracking-[0.2em] text-sky-200/60">
              <MessageCircle className="h-3 w-3" />
              Private desk
            </div>
            <h2 className="explore-display mt-2 text-2xl font-semibold text-zinc-100">
              Thoughtful conversations.
            </h2>
            <p className="mt-2 max-w-xl text-sm leading-6 text-zinc-400">
              Continue focused one-to-one discussions with mutual connections.
            </p>
          </div>
          <div className="flex items-center gap-3 rounded-xl border border-emerald-300/15 bg-emerald-400/[0.045] px-3 py-2.5">
            <ShieldCheck className="h-4 w-4 text-emerald-300/70" />
            <div>
              <div className="font-mono text-sm font-medium tabular-nums text-emerald-100">
                {conversations.data?.conversations.length ?? 0}
              </div>
              <div className="text-[8px] uppercase tracking-[0.14em] text-emerald-200/45">Connections</div>
            </div>
          </div>
        </div>
      </div>

      <div className="flex items-center justify-between gap-3 border-b border-sky-300/10 pb-3">
        <div>
          <h3 className="explore-display text-lg font-semibold text-zinc-100">Message workspace</h3>
          <p className="mt-1 text-xs text-zinc-500">Select a conversation and pick up where you left off.</p>
        </div>
        <button
          type="button"
          onClick={() => {
            conversations.refetch();
            messages.refetch();
          }}
          disabled={conversations.isFetching || messages.isFetching}
          aria-label="Refresh messages"
          className="rounded-lg border border-sky-300/10 bg-sky-400/[0.035] p-2.5 text-sky-200/60 transition hover:border-sky-300/25 hover:bg-sky-400/[0.07] hover:text-sky-100 disabled:opacity-50"
        >
          <RefreshCw
            className={cn(
              "h-3.5 w-3.5",
              (conversations.isFetching || messages.isFetching) && "animate-spin",
            )}
          />
        </button>
      </div>

      <div className="grid overflow-hidden rounded-2xl border border-sky-300/10 bg-zinc-900/25 shadow-[0_20px_60px_rgba(0,0,0,0.16)] lg:grid-cols-[330px_minmax(0,1fr)]">
        <aside className={cn("space-y-3 border-sky-300/10 p-4 lg:border-r", selectedId && "hidden lg:block")}>
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-semibold text-zinc-100">Conversations</h3>
            <span className="font-mono text-[9px] text-sky-300/50">
              {conversations.data?.conversations.length ?? 0} total
            </span>
          </div>
          {conversations.isLoading ? (
            <div className="rounded-xl border border-zinc-800 bg-zinc-900/35 p-6 text-sm text-zinc-500">
              Loading conversations...
            </div>
          ) : conversations.isError ? (
            <div className="rounded-xl border border-rose-400/15 bg-rose-400/[0.04] p-6 text-sm text-rose-200">
              Could not load conversations.
            </div>
          ) : (
            <ConversationList
              conversations={conversations.data?.conversations ?? []}
              selectedId={selectedId}
              onSelect={selectConversation}
            />
          )}
        </aside>

        <section className={cn("space-y-3 bg-[radial-gradient(circle_at_top_right,rgba(99,102,241,0.045),transparent_30%)] p-4", !selectedId && "hidden lg:block")}>
          <div className="flex items-center justify-between gap-3 border-b border-indigo-300/10 pb-3">
            <div className="flex items-center gap-3">
              <div className="grid h-9 w-9 place-items-center overflow-hidden rounded-lg border border-indigo-300/15 bg-indigo-400/[0.07] font-mono text-[10px] uppercase text-indigo-100">
                {selected ? avatarInitials(selected.other_user.avatar_key, selected.other_user.handle) : "—"}
              </div>
              <div>
              <h3 className="text-sm font-semibold text-zinc-100">
                {selected ? selected.other_user.display_name : "Select a conversation"}
              </h3>
              {selected && (
                <p className="flex items-center gap-1.5 font-mono text-[10px] text-zinc-500">
                  @{selected.other_user.handle}
                  <span className="h-1.5 w-1.5 rounded-full bg-emerald-300/75" />
                </p>
              )}
              </div>
            </div>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="lg:hidden"
              onClick={clearConversation}
            >
              <ArrowLeft /> Back
            </Button>
          </div>

          {selectedId && messages.isLoading ? (
            <div className="rounded-xl border border-zinc-800 bg-zinc-900/35 p-8 text-sm text-zinc-500">
              Loading messages...
            </div>
          ) : selectedId && messages.isError ? (
            <div className="rounded-xl border border-rose-400/15 bg-rose-400/[0.04] p-8 text-sm text-rose-200">
              Could not load messages.
            </div>
          ) : (
            <MessageThread messages={messages.data?.messages ?? []} conversationId={selectedId ?? ""} />
          )}
          <MessageComposer conversationId={selectedId} />
        </section>
      </div>
    </section>
  );
}
