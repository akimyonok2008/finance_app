import { Search, X } from "lucide-react";
import type { FormEvent } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { ExploreSort } from "@/types/explore";

export function ExploreFilterBar({
  query,
  symbol,
  sort,
  onQueryChange,
  onSymbolChange,
  onSortChange,
  onSubmit,
  onClear,
}: {
  query: string;
  symbol: string;
  sort: ExploreSort;
  onQueryChange: (value: string) => void;
  onSymbolChange: (value: string) => void;
  onSortChange: (value: ExploreSort) => void;
  onSubmit: () => void;
  onClear: () => void;
}) {
  const changed = query.length > 0 || symbol.length > 0 || sort !== "top";
  const submit = (event: FormEvent) => {
    event.preventDefault();
    onSubmit();
  };

  return (
    <form
      onSubmit={submit}
      className="mb-7 rounded-2xl border border-sky-300/10 bg-[radial-gradient(circle_at_top_left,rgba(56,189,248,0.055),transparent_36%),rgba(24,24,27,0.42)] p-4 shadow-[0_18px_60px_rgba(0,0,0,0.16)]"
    >
      <div className="mb-3 flex flex-wrap items-end justify-between gap-2">
        <div>
          <h3 className="text-sm font-semibold text-zinc-100">Find a public strategy</h3>
          <p className="mt-1 text-[11px] text-zinc-500">
            Search results stay separate from your curated discovery sections.
          </p>
        </div>
        {changed ? (
          <Button type="button" variant="ghost" size="sm" onClick={onClear} aria-label="Clear Explore filters">
            <X /> Clear filters
          </Button>
        ) : null}
      </div>
      <div className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_190px_150px_auto]">
        <label className="space-y-1.5">
          <span className="text-[9px] font-medium uppercase tracking-[0.16em] text-zinc-600">Name or handle</span>
          <div className="relative">
            <Search className="pointer-events-none absolute left-3 top-3.5 h-4 w-4 text-sky-300/45" />
            <Input
              aria-label="Search by name or handle"
              placeholder="Name or @handle"
              value={query}
              onChange={(event) => onQueryChange(event.target.value)}
              className="h-11 border-zinc-800/90 bg-zinc-950/65 pl-9"
            />
          </div>
        </label>
        <label className="space-y-1.5">
          <span className="text-[9px] font-medium uppercase tracking-[0.16em] text-zinc-600">Public holding</span>
          <Input
            aria-label="Public holding symbol"
            placeholder="e.g. NVDA"
            value={symbol}
            onChange={(event) => onSymbolChange(event.target.value.toUpperCase())}
            className="h-11 border-zinc-800/90 bg-zinc-950/65 font-mono uppercase"
          />
        </label>
        <label className="space-y-1.5">
          <span className="text-[9px] font-medium uppercase tracking-[0.16em] text-zinc-600">Order results</span>
          <Select value={sort} onValueChange={(value) => onSortChange(value as ExploreSort)}>
            <SelectTrigger aria-label="Sort search results" className="h-11 border-zinc-800/90 bg-zinc-950/65">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="top">Relevance</SelectItem>
              <SelectItem value="return">Return</SelectItem>
              <SelectItem value="rank">Rank</SelectItem>
              <SelectItem value="recent">Recent</SelectItem>
            </SelectContent>
          </Select>
        </label>
        <div className="flex items-end">
          <Button type="submit" className="h-11 w-full px-5 lg:w-auto">
            <Search /> Search
          </Button>
        </div>
      </div>
    </form>
  );
}
