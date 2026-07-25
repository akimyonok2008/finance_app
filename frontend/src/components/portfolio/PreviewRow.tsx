import { cn } from "@/utils/cn";

/** One label/value row inside a live cost or proceeds preview panel. */
export function PreviewRow({
  label,
  value,
  emphasize,
  tone,
}: {
  label: string;
  value: string;
  emphasize?: boolean;
  tone?: "positive" | "negative";
}) {
  return (
    <>
      <dt className="text-zinc-500">{label}</dt>
      <dd
        className={cn(
          "text-right font-mono tabular-nums text-zinc-200",
          emphasize && "font-semibold text-zinc-50",
          tone === "positive" && "text-emerald-400",
          tone === "negative" && "text-rose-400",
        )}
      >
        {value}
      </dd>
    </>
  );
}
