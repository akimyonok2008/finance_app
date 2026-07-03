import { ArrowLeft, RefreshCw } from "lucide-react";
import { useMemo } from "react";
import { useSearchParams } from "react-router-dom";

import { ConversationList } from "@/components/dm/ConversationList";
import { MessageComposer } from "@/components/dm/MessageComposer";
import { MessageThread } from "@/components/dm/MessageThread";
import { Button } from "@/components/ui/button";
import { useConversations, useMessages } from "@/hooks/useDM";
import { cn } from "@/utils/cn";

export function MessagesPanel() {
  const [searchParams, setSearchParams] = useSearchParams();
  const selectedId = searchParams.get("conversationId");
  const conversations = useConversations();
  const messages = useMessages(selectedId);

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
    <section className="space-y-5">
      <div className="flex flex-col justify-between gap-3 sm:flex-row sm:items-start">
        <div>
          <h2 className="text-lg font-semibold text-zinc-100">Messages</h2>
          <p className="mt-1 max-w-2xl text-sm text-zinc-400">
            Private one-to-one messages are available only between mutual friends.
          </p>
        </div>
        <button
          type="button"
          onClick={() => {
            conversations.refetch();
            messages.refetch();
          }}
          disabled={conversations.isFetching || messages.isFetching}
          aria-label="Refresh messages"
          className="self-start rounded-lg border border-zinc-800 p-2 text-zinc-400 transition hover:bg-zinc-800/70 hover:text-zinc-100 disabled:opacity-50"
        >
          <RefreshCw
            className={cn(
              "h-3.5 w-3.5",
              (conversations.isFetching || messages.isFetching) && "animate-spin",
            )}
          />
        </button>
      </div>

      <div className="grid gap-5 lg:grid-cols-[340px_minmax(0,1fr)]">
        <aside className={cn("space-y-3", selectedId && "hidden lg:block")}>
          <h3 className="text-sm font-semibold text-zinc-100">Conversations</h3>
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

        <section className={cn("space-y-3", !selectedId && "hidden lg:block")}>
          <div className="flex items-center justify-between gap-3">
            <div>
              <h3 className="text-sm font-semibold text-zinc-100">
                {selected ? selected.other_user.display_name : "Select a conversation"}
              </h3>
              {selected && (
                <p className="text-xs text-zinc-500">@{selected.other_user.handle}</p>
              )}
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
            <MessageThread messages={messages.data?.messages ?? []} />
          )}
          <MessageComposer conversationId={selectedId} />
        </section>
      </div>
    </section>
  );
}
