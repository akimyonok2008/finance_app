import { Sparkles } from "lucide-react";

import type { ReactNode } from "react";

export function BadgeEmptyState({
  title,
  description,
  icon,
}: {
  title: string;
  description: string;
  icon?: ReactNode;
}) {
  return (
    <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-zinc-800 bg-zinc-900/30 px-6 py-14 text-center">
      <div className="grid h-11 w-11 place-items-center rounded-full border border-zinc-800 bg-zinc-900 text-zinc-500">
        {icon ?? <Sparkles className="h-5 w-5" />}
      </div>
      <h3 className="mt-4 text-sm font-semibold text-zinc-200">{title}</h3>
      <p className="mt-1.5 max-w-md text-sm leading-6 text-zinc-500">
        {description}
      </p>
    </div>
  );
}
