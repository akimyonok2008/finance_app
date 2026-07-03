import { MessageSquare } from "lucide-react";

import type { ConversationSummary } from "@/types/dm";
import { cn } from "@/utils/cn";

type Props = {
  conversations: ConversationSummary[];
  selectedId: string | null;
  onSelect: (id: string) => void;
};

export function ConversationList({ conversations, selectedId, onSelect }: Props) {
  if (conversations.length === 0) {
    return (
      <div className="rounded-xl border border-zinc-800 bg-zinc-900/35 p-6 text-sm text-zinc-500">
        No conversations yet. Start from a mutual friend.
      </div>
    );
  }

  return (
    <div className="space-y-2">
      {conversations.map((conversation) => (
        <button
          key={conversation.id}
          type="button"
          onClick={() => onSelect(conversation.id)}
          className={cn(
            "w-full rounded-xl border p-3 text-left transition",
            selectedId === conversation.id
              ? "border-zinc-500 bg-zinc-800/70"
              : "border-zinc-800 bg-zinc-900/35 hover:bg-zinc-900",
          )}
        >
          <div className="flex items-center justify-between gap-3">
            <div>
              <div className="font-medium text-zinc-100">
                {conversation.other_user.display_name}
              </div>
              <div className="text-xs text-zinc-500">
                @{conversation.other_user.handle}
              </div>
            </div>
            <MessageSquare className="h-4 w-4 text-zinc-500" />
          </div>
          <div className="mt-2 truncate text-xs text-zinc-500">
            {conversation.last_message
              ? `${conversation.last_message.sent_by_me ? "You: " : ""}${conversation.last_message.body_preview}`
              : "No messages yet"}
          </div>
        </button>
      ))}
    </div>
  );
}
