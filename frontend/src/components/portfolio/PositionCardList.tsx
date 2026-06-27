import { AnimatePresence, motion } from "framer-motion";
import { Pencil, Trash2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { AssetTypeBadge } from "@/components/portfolio/AssetTypeBadge";
import { PortfolioEmptyState } from "@/components/portfolio/PortfolioEmptyState";
import { PortfolioCardsSkeleton } from "@/components/portfolio/PortfolioSkeleton";
import {
  rowBaseCurrency,
  rowCurrency,
  rowPriceCurrency,
  type PositionRow,
} from "@/hooks/usePositionRows";
import { cn } from "@/utils/cn";
import { formatMoney } from "@/utils/formatMoney";
import { formatPercent } from "@/utils/formatPercent";
import { gainLossColor } from "@/utils/gainLoss";

type Props = {
  rows: PositionRow[];
  isLoading: boolean;
  isError: boolean;
  errorMessage?: string;
  onEdit: (position: PositionRow) => void;
  onDelete: (position: PositionRow) => void;
  /** Opens the add drawer from the empty state. */
  onAdd?: () => void;
  className?: string;
};

function Stat({
  label,
  value,
  className,
}: {
  label: string;
  value: string;
  className?: string;
}) {
  return (
    <div className="space-y-0.5">
      <div className="text-[11px] uppercase tracking-wide text-muted-foreground">
        {label}
      </div>
      <div className={cn("text-sm tabular-nums", className)}>{value}</div>
    </div>
  );
}

export function PositionCardList({
  rows,
  isLoading,
  isError,
  errorMessage,
  onEdit,
  onDelete,
  onAdd,
  className,
}: Props) {
  if (isLoading) {
    return (
      <div className={className}>
        <PortfolioCardsSkeleton />
      </div>
    );
  }

  if (isError) {
    return (
      <div className={cn("lg:hidden", className)}>
        <Card className="p-6 text-center">
          <p className="text-sm text-rose-300">
            {errorMessage ?? "Could not load your positions."}
          </p>
        </Card>
      </div>
    );
  }

  if (rows.length === 0) {
    return (
      <div className={cn("lg:hidden", className)}>
        <Card>
          <PortfolioEmptyState onAdd={onAdd} />
        </Card>
      </div>
    );
  }

  return (
    <div className={cn("grid gap-3 lg:hidden", className)}>
      <AnimatePresence initial={false}>
        {rows.map((row) => {
          const currency = rowCurrency(row);
          const baseCurrency = rowBaseCurrency(row);
          const priceCurrency = rowPriceCurrency(row);
          return (
            <motion.div
              key={row.id}
              layout
              initial={{ opacity: 0, y: 10, scale: 0.98 }}
              animate={{ opacity: 1, y: 0, scale: 1 }}
              exit={{ opacity: 0, y: -8, scale: 0.98 }}
              transition={{ type: "spring", stiffness: 260, damping: 24 }}
            >
              <Card className="p-4">
                <div className="flex items-start justify-between gap-3">
                  <div className="flex items-center gap-2">
                    <span className="text-base font-semibold tracking-wide">
                      {row.symbol}
                    </span>
                    <AssetTypeBadge type={row.asset_type} />
                  </div>
                  <div
                    className={cn(
                      "text-sm font-semibold tabular-nums",
                      gainLossColor(row.gain_loss_percentage),
                    )}
                  >
                    {formatPercent(row.gain_loss_percentage)}
                  </div>
                </div>

                <div className="mt-4 grid grid-cols-2 gap-3">
                  <Stat label="Quantity" value={String(row.quantity)} />
                  <Stat
                    label="Baseline"
                    value={formatMoney(row.baseline_price, currency)}
                    className="text-slate-300"
                  />
                  <Stat
                    label="Current Price"
                    value={formatMoney(row.current_price, priceCurrency)}
                    className="text-slate-300"
                  />
                  <Stat
                    label={`Current Value (${baseCurrency})`}
                    value={formatMoney(row.current_value, baseCurrency)}
                    className="font-medium"
                  />
                  <Stat
                    label={`Gain / Loss (${baseCurrency})`}
                    value={formatMoney(row.gain_loss, baseCurrency)}
                    className={gainLossColor(row.gain_loss)}
                  />
                  <Stat
                    label="FX-normalized return"
                    value={formatPercent(row.gain_loss_percentage)}
                    className={gainLossColor(row.gain_loss_percentage)}
                  />
                </div>

                <div className="mt-4 flex gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    className="flex-1"
                    onClick={() => onEdit(row)}
                  >
                    <Pencil />
                    Edit
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    className="flex-1 text-rose-300 hover:text-rose-200"
                    onClick={() => onDelete(row)}
                  >
                    <Trash2 />
                    Delete
                  </Button>
                </div>
              </Card>
            </motion.div>
          );
        })}
      </AnimatePresence>
    </div>
  );
}
