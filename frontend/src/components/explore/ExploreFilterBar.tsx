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
    <form onSubmit={submit} className="mb-8 rounded-2xl border border-zinc-800 bg-zinc-900/40 p-3">
      <div className="grid gap-3 lg:grid-cols-[1fr_180px_150px_auto]">
        <div className="relative">
          <Search className="pointer-events-none absolute left-3 top-3 h-4 w-4 text-zinc-600" />
          <Input
            aria-label="Search profiles or symbols"
            placeholder="Search profiles or symbols…"
            value={query}
            onChange={(event) => onQueryChange(event.target.value)}
            className="pl-9"
          />
        </div>
        <Input
          aria-label="Symbol filter"
          placeholder="Symbol, e.g. NVDA"
          value={symbol}
          onChange={(event) => onSymbolChange(event.target.value.toUpperCase())}
          className="font-mono uppercase"
        />
        <Select value={sort} onValueChange={(value) => onSortChange(value as ExploreSort)}>
          <SelectTrigger aria-label="Sort Explore results">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="top">Top</SelectItem>
            <SelectItem value="return">Return</SelectItem>
            <SelectItem value="rank">Rank</SelectItem>
            <SelectItem value="recent">Recent</SelectItem>
          </SelectContent>
        </Select>
        <div className="flex gap-2">
          <Button type="submit" className="flex-1 lg:flex-none">Search</Button>
          {changed ? (
            <Button type="button" variant="ghost" onClick={onClear} aria-label="Clear Explore filters">
              <X /> Clear
            </Button>
          ) : null}
        </div>
      </div>
    </form>
  );
}
