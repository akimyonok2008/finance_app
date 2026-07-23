import type { Message } from "@/types/dm";
import { cn } from "@/utils/cn";

type Props = {
  messages: Message[];
};

export function MessageThread({ messages }: Props) {
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
          className={cn("flex", message.sent_by_me ? "justify-end" : "justify-start")}
        >
          <div
            className={cn(
              "max-w-[78%] rounded-xl border px-3 py-2.5 shadow-sm",
              message.sent_by_me
                ? "border-sky-300/20 bg-gradient-to-br from-sky-400/[0.12] to-indigo-400/[0.08]"
                : "border-violet-300/15 bg-violet-400/[0.055]",
            )}
          >
            <div className="text-sm text-zinc-100">{message.body}</div>
            <div className={cn("mt-1.5 font-mono text-[9px] uppercase", message.sent_by_me ? "text-sky-200/40" : "text-violet-200/35")}>
              {new Date(message.created_at).toLocaleString()}
            </div>
          </div>
        </div>
      ))}
    </div>
  );
}
