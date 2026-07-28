import { Flag, Trash2 } from "lucide-react";

import { ReportModal } from "@/components/social/ReportModal";
import { useHideMessage } from "@/hooks/useSafety";
import type { Message } from "@/types/dm";
import { cn } from "@/utils/cn";

type Props = {
  messages: Message[];
  conversationId: string;
};

export function MessageThread({ messages, conversationId }: Props) {
  const hideMessage = useHideMessage(conversationId);

  if (messages.length === 0) {
    return (
      <div className="flex min-h-72 items-center justify-center rounded-xl border border-dashed border-indigo-300/10 bg-indigo-400/[0.02] p-6 text-sm text-zinc-500">
        Start the conversation when you are ready.
      </div>
    );
  }

  return (
    <div className="min-h-72 space-y-3 rounded-xl border border-indigo-300/10 bg-[radial-gradient(circle_at_top_right,rgba(99,102,241,0.045),transparent_34%),rgba(9,9,11,0.58)] p-4">
      {messages.map((message) => (
        <div
          key={message.id}
          className={cn("group flex items-center gap-1.5", message.sent_by_me ? "justify-end" : "justify-start")}
        >
          {!message.sent_by_me && !message.removed && (
            <MessageActions
              onHide={() => hideMessage.mutate(message.id)}
              messageId={message.id}
              showReport
            />
          )}
          <div
            className={cn(
              "max-w-[78%] rounded-xl border px-3 py-2.5 shadow-sm",
              message.removed
                ? "border-dashed border-zinc-500/20 bg-zinc-500/[0.04] italic text-zinc-500"
                : message.sent_by_me
                  ? "border-sky-300/20 bg-gradient-to-br from-sky-400/[0.12] to-indigo-400/[0.08]"
                  : "border-violet-300/15 bg-violet-400/[0.055]",
            )}
          >
            <div className="text-sm text-zinc-100">{message.body}</div>
            <div className={cn("mt-1.5 font-mono text-[9px] uppercase", message.sent_by_me ? "text-sky-200/40" : "text-violet-200/35")}>
              {new Date(message.created_at).toLocaleString()}
            </div>
          </div>
          {message.sent_by_me && !message.removed && (
            <MessageActions onHide={() => hideMessage.mutate(message.id)} showReport={false} />
          )}
        </div>
      ))}
    </div>
  );
}

function MessageActions({
  onHide,
  messageId,
  showReport,
}: {
  onHide: () => void;
  messageId?: string;
  showReport: boolean;
}) {
  return (
    <div className="flex gap-0.5 opacity-0 transition-opacity group-hover:opacity-100">
      <button
        type="button"
        onClick={onHide}
        title="Delete for me"
        className="rounded p-1 text-zinc-600 hover:bg-white/5 hover:text-zinc-300"
      >
        <Trash2 className="h-3.5 w-3.5" />
      </button>
      {showReport && messageId && (
        <ReportModal
          target="message"
          messageId={messageId}
          trigger={
            <button
              type="button"
              title="Report message"
              className="rounded p-1 text-zinc-600 hover:bg-white/5 hover:text-rose-300"
            >
              <Flag className="h-3.5 w-3.5" />
            </button>
          }
        />
      )}
    </div>
  );
}
