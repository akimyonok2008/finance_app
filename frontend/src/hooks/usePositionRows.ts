import { useMemo } from "react";

import { usePositions } from "@/hooks/usePositions";
import { usePortfolioSummary } from "@/hooks/usePortfolioSummary";
import type { Position, PositionSummary } from "@/types/portfolio";

/**
 * A raw position (the editable source of truth) enriched with pricing fields
 * from the summary when available. Edit/delete always use {@link Position.id}.
 */
export type PositionRow = Position & {
  current_price?: number;
  current_price_currency?: string;
  current_value?: number;
  gain_loss?: number;
  gain_loss_percentage?: number;
  cost_basis?: number;
  local_current_value?: number;
  local_gain_loss?: number;
  local_cost_basis?: number;
  base_currency?: string;
};

type PositionRowsResult = {
  rows: PositionRow[];
  /** True while the canonical positions list is loading. */
  isLoading: boolean;
  /** True only when the positions list itself failed (summary is best-effort). */
  isError: boolean;
  error: Error | null;
};

/**
 * Merge GET /portfolio/positions (canonical, editable) with the enriched
 * positions inside GET /portfolio/summary, matched by id then symbol. The
 * positions query drives loading/empty/error; the summary is best-effort.
 */
export function usePositionRows(): PositionRowsResult {
  const positionsQuery = usePositions();
  const summaryQuery = usePortfolioSummary();

  const rows = useMemo<PositionRow[]>(() => {
    const positions = positionsQuery.data ?? [];
    const summaryPositions = summaryQuery.data?.positions ?? [];

    const byId = new Map<string, PositionSummary>();
    const bySymbol = new Map<string, PositionSummary>();
    for (const sp of summaryPositions) {
      const id = sp.position_id ?? sp.id;
      if (id) byId.set(id, sp);
      bySymbol.set(sp.symbol.toUpperCase(), sp);
    }

    return positions.map((p) => {
      const match =
        byId.get(p.id) ?? bySymbol.get(p.symbol.toUpperCase());
      return {
        ...p,
        current_price: match?.current_price,
        current_price_currency: match?.current_price_currency ?? match?.currency,
        current_value: match?.current_value_base ?? match?.current_value,
        gain_loss: match?.gain_loss_base ?? match?.gain_loss,
        gain_loss_percentage: match?.gain_loss_percentage,
        cost_basis: match?.cost_basis_base ?? match?.cost_basis,
        local_current_value: match?.current_value,
        local_gain_loss: match?.gain_loss,
        local_cost_basis: match?.cost_basis,
        base_currency: match?.base_currency ?? summaryQuery.data?.base_currency,
      };
    });
  }, [positionsQuery.data, summaryQuery.data]);

  return {
    rows,
    isLoading: positionsQuery.isLoading,
    isError: positionsQuery.isError,
    error: (positionsQuery.error as Error | null) ?? null,
  };
}

/** Resolve a row's display currency, falling back to USD. */
export function rowCurrency(row: PositionRow): string {
  return row.currency || "USD";
}

export function rowBaseCurrency(row: PositionRow): string {
  return row.base_currency || "USD";
}

export function rowPriceCurrency(row: PositionRow): string {
  return row.current_price_currency || row.currency || "USD";
}
