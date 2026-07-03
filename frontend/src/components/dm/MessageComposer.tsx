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
    <div className="rounded-xl border border-zinc-800 bg-zinc-900/35 p-3">
      <textarea
        value={body}
        onChange={(event) => setBody(event.target.value.slice(0, 1000))}
        disabled={!conversationId || send.isPending}
        placeholder="Write a private message..."
        className="min-h-24 w-full resize-none rounded-lg border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 outline-none transition placeholder:text-zinc-600 focus:border-zinc-600 disabled:opacity-50"
      />
      <div className="mt-2 flex items-center justify-between gap-3">
        <span className="text-xs text-zinc-600">{remaining} characters left</span>
        <Button
          type="button"
          size="sm"
          onClick={submit}
          disabled={!conversationId || !body.trim() || send.isPending}
        >
          <Send /> Send
        </Button>
      </div>
    </div>
  );
}
