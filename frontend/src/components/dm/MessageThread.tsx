import type { Message } from "@/types/dm";
import { cn } from "@/utils/cn";

type Props = {
  messages: Message[];
};

export function MessageThread({ messages }: Props) {
  if (messages.length === 0) {
    return (
      <div className="flex min-h-64 items-center justify-center rounded-xl border border-zinc-800 bg-zinc-900/25 p-6 text-sm text-zinc-500">
        No messages yet.
      </div>
    );
  }

  return (
    <div className="min-h-64 space-y-3 rounded-xl border border-zinc-800 bg-zinc-950/50 p-4">
      {messages.map((message) => (
        <div
          key={message.id}
          className={cn("flex", message.sent_by_me ? "justify-end" : "justify-start")}
        >
          <div
            className={cn(
              "max-w-[78%] rounded-xl border px-3 py-2",
              message.sent_by_me
                ? "border-emerald-400/20 bg-emerald-400/[0.06]"
                : "border-zinc-800 bg-zinc-900/70",
            )}
          >
            <div className="text-sm text-zinc-100">{message.body}</div>
            <div className="mt-1 text-[10px] uppercase text-zinc-600">
              {new Date(message.created_at).toLocaleString()}
            </div>
          </div>
        </div>
      ))}
    </div>
  );
}
