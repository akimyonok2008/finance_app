import { Loader2, Search } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

import { Input } from "@/components/ui/input";
import { useInstrumentSearch } from "@/hooks/useInstruments";
import type { Instrument } from "@/types/instruments";
import { cn } from "@/utils/cn";

type Props = {
  id: string;
  value: string;
  disabled?: boolean;
  invalid?: boolean;
  describedBy?: string;
  onValueChange: (value: string) => void;
  onSelect: (instrument: Instrument) => void;
};

export function SymbolAutocomplete({
  id,
  value,
  disabled,
  invalid,
  describedBy,
  onValueChange,
  onSelect,
}: Props) {
  const [open, setOpen] = useState(false);
  const [debounced, setDebounced] = useState(value);

  useEffect(() => {
    const timer = window.setTimeout(() => setDebounced(value.trim()), 350);
    return () => window.clearTimeout(timer);
  }, [value]);

  const search = useInstrumentSearch(debounced);
  const results = search.data?.results ?? [];
  const source = search.data?.source;
  const showPanel = open && value.trim().length > 0 && !disabled;

  const helper = useMemo(() => {
    if (search.isFetching) return "Searching instruments...";
    if (search.isError) return "Could not search instruments. Try again.";
    if (source) return `Results from ${source}. Select a listed instrument so prices can update automatically.`;
    return "Select a listed instrument so prices can update automatically.";
  }, [search.isError, search.isFetching, source]);

  return (
    <div className="relative">
      <div className="relative">
        <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-zinc-500" />
        <Input
          id={id}
          name="symbol"
          autoComplete="off"
          autoCapitalize="characters"
          placeholder="Search stock or ETF..."
          value={value}
          disabled={disabled}
          aria-invalid={invalid}
          aria-describedby={describedBy}
          className="pl-9 uppercase tabular-nums tracking-wide"
          onFocus={() => setOpen(true)}
          onBlur={() => window.setTimeout(() => setOpen(false), 120)}
          onChange={(e) => {
            onValueChange(e.target.value.toUpperCase());
            setOpen(true);
          }}
        />
        {search.isFetching && (
          <Loader2 className="absolute right-3 top-1/2 h-4 w-4 -translate-y-1/2 animate-spin text-zinc-500" />
        )}
      </div>

      {showPanel && (
        <div className="absolute z-30 mt-2 max-h-72 w-full overflow-auto rounded-xl border border-zinc-800 bg-zinc-950 p-1 shadow-xl shadow-black/30">
          {search.isError ? (
            <div className="px-3 py-3 text-sm text-rose-300">
              Could not search instruments. Try again.
            </div>
          ) : results.length === 0 && !search.isFetching ? (
            <div className="px-3 py-3 text-sm text-zinc-500">
              No matching instruments found.
            </div>
          ) : (
            results.map((item) => (
              <button
                key={`${item.provider}:${item.symbol}`}
                type="button"
                className="w-full rounded-lg px-3 py-2 text-left transition hover:bg-zinc-900 focus-visible:bg-zinc-900 focus-visible:outline-none"
                onMouseDown={(e) => e.preventDefault()}
                onClick={() => {
                  onSelect(item);
                  setOpen(false);
                }}
              >
                <div className="flex items-center justify-between gap-3">
                  <span className="font-mono text-sm font-semibold text-zinc-100">
                    {item.display_symbol || item.symbol}
                  </span>
                  <span className="rounded-full border border-zinc-800 px-2 py-0.5 text-[10px] uppercase text-zinc-500">
                    {item.asset_type}
                  </span>
                </div>
                <div className="mt-0.5 truncate text-xs text-zinc-400">
                  {item.name}
                </div>
                <div className="mt-1 text-[11px] uppercase tracking-wide text-zinc-600">
                  {[item.exchange, item.currency, item.country].filter(Boolean).join(" · ")}
                </div>
              </button>
            ))
          )}
          <div
            className={cn(
              "border-t border-zinc-900 px-3 py-2 text-[11px]",
              search.isError ? "text-rose-300" : "text-zinc-600",
            )}
          >
            {helper}
          </div>
        </div>
      )}
    </div>
  );
}
