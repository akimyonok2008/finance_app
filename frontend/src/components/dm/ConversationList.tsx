import { ChevronRight, MessageSquare } from "lucide-react";

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
      <div className="rounded-xl border border-dashed border-sky-300/10 bg-sky-400/[0.025] p-8 text-center">
        <MessageSquare className="mx-auto h-5 w-5 text-sky-300/45" />
        <p className="mt-3 text-sm font-medium text-zinc-300">No conversations yet</p>
        <p className="mt-1 text-xs text-zinc-500">Start one from a mutual friend.</p>
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
            "group relative w-full overflow-hidden rounded-xl border p-3 text-left transition",
            selectedId === conversation.id
              ? "border-sky-300/25 bg-sky-400/[0.09] shadow-[inset_3px_0_0_rgba(125,211,252,0.65)]"
              : "border-zinc-800/90 bg-zinc-950/35 hover:border-indigo-300/15 hover:bg-indigo-400/[0.035]",
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
            <div className="flex items-center gap-2">
              {conversation.unread_count > 0 && (
                <span className="grid h-5 min-w-5 place-items-center rounded-full bg-sky-400/90 px-1.5 font-mono text-[10px] font-semibold text-zinc-950">
                  {conversation.unread_count > 99 ? "99+" : conversation.unread_count}
                </span>
              )}
              <ChevronRight className={cn("h-4 w-4 transition", selectedId === conversation.id ? "text-sky-200/70" : "text-zinc-600 group-hover:text-indigo-200/60")} />
            </div>
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
