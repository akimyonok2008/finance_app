import { Send } from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { useSendMessage } from "@/hooks/useDM";

type Props = {
  conversationId: string | null;
};

export function MessageComposer({ conversationId }: Props) {
  const [body, setBody] = useState("");
  const send = useSendMessage(conversationId);
  const remaining = 1000 - body.length;

  const submit = async () => {
    const text = body.trim();
    if (!text || !conversationId) return;
    await send.mutateAsync(text);
    setBody("");
  };

  return (
    <div className="rounded-xl border border-sky-300/10 bg-[linear-gradient(135deg,rgba(56,189,248,0.035),rgba(99,102,241,0.025))] p-3">
      <textarea
        value={body}
        onChange={(event) => setBody(event.target.value.slice(0, 1000))}
        disabled={!conversationId || send.isPending}
        placeholder="Write a private message..."
        className="min-h-24 w-full resize-none rounded-lg border border-zinc-800 bg-zinc-950/80 px-3 py-2 text-sm text-zinc-100 outline-none transition placeholder:text-zinc-600 focus:border-sky-300/30 focus:ring-2 focus:ring-sky-400/[0.06] disabled:opacity-50"
      />
      <div className="mt-2 flex items-center justify-between gap-3">
        <span className="text-xs text-zinc-600">{remaining} characters left</span>
        <Button
          type="button"
          size="sm"
          onClick={submit}
          disabled={!conversationId || !body.trim() || send.isPending}
          className="bg-gradient-to-r from-sky-100 to-indigo-100 text-zinc-950 hover:from-white hover:to-violet-100"
        >
          <Send /> Send
        </Button>
      </div>
    </div>
  );
}
