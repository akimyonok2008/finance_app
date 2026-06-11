import { AnimatePresence, motion } from "framer-motion";
import { Pencil, Trash2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { AssetTypeBadge } from "@/components/portfolio/AssetTypeBadge";
import { PortfolioEmptyState } from "@/components/portfolio/PortfolioEmptyState";
import { PortfolioTableSkeleton } from "@/components/portfolio/PortfolioSkeleton";
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
};

// framer-motion needs a real DOM table row; motion.tr keeps semantics intact.
const MotionTr = motion.tr;

export function PositionsTable({
  rows,
  isLoading,
  isError,
  errorMessage,
  onEdit,
  onDelete,
}: Props) {
  if (isLoading) {
    return <PortfolioTableSkeleton />;
  }

  if (isError) {
    return (
      <Card className="hidden p-8 text-center lg:block">
        <p className="text-sm text-rose-300">
          {errorMessage ?? "Could not load your positions."}
        </p>
      </Card>
    );
  }

  if (rows.length === 0) {
    return (
      <Card className="hidden lg:block">
        <PortfolioEmptyState />
      </Card>
    );
  }

  return (
    <Card className="hidden overflow-hidden lg:block">
      <Table>
        <TableHeader>
          <TableRow className="hover:bg-transparent">
            <TableHead>Symbol</TableHead>
            <TableHead>Type</TableHead>
            <TableHead className="text-right">Quantity</TableHead>
            <TableHead className="text-right">Avg Buy</TableHead>
            <TableHead className="text-right">Current Price</TableHead>
            <TableHead className="text-right">FX Return</TableHead>
            <TableHead className="text-right">Base Gain / Loss</TableHead>
            <TableHead className="text-right">Base Value</TableHead>
            <TableHead className="text-right">
              <span className="sr-only">Actions</span>
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <AnimatePresence initial={false}>
            {rows.map((row) => {
              const currency = rowCurrency(row);
              const baseCurrency = rowBaseCurrency(row);
              const priceCurrency = rowPriceCurrency(row);
              return (
                <MotionTr
                  key={row.id}
                  layout
                  initial={{ opacity: 0, y: 8 }}
                  animate={{ opacity: 1, y: 0 }}
                  exit={{ opacity: 0, scale: 0.98 }}
                  transition={{ type: "spring", stiffness: 260, damping: 24 }}
                  className="border-b border-white/[0.06] transition-colors hover:bg-white/[0.03]"
                >
                  <TableCell className="font-medium tracking-wide">
                    {row.symbol}
                  </TableCell>
                  <TableCell>
                    <AssetTypeBadge type={row.asset_type} />
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {row.quantity}
                  </TableCell>
                  <TableCell className="text-right tabular-nums text-slate-300">
                    {formatMoney(row.average_buy_price, currency)}
                  </TableCell>
                  <TableCell className="text-right tabular-nums text-slate-300">
                    {formatMoney(row.current_price, priceCurrency)}
                  </TableCell>
                  <TableCell
                    className={cn(
                      "text-right font-medium tabular-nums",
                      gainLossColor(row.gain_loss_percentage),
                    )}
                  >
                    {formatPercent(row.gain_loss_percentage)}
                  </TableCell>
                  <TableCell
                    className={cn(
                      "text-right font-medium tabular-nums",
                      gainLossColor(row.gain_loss),
                    )}
                  >
                    {formatMoney(row.gain_loss, baseCurrency)}
                  </TableCell>
                  <TableCell className="text-right font-medium tabular-nums">
                    {formatMoney(row.current_value, baseCurrency)}
                  </TableCell>
                  <TableCell className="text-right">
                    <div className="flex justify-end gap-1">
                      <Button
                        variant="ghost"
                        size="icon"
                        aria-label={`Edit ${row.symbol}`}
                        onClick={() => onEdit(row)}
                      >
                        <Pencil />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        aria-label={`Delete ${row.symbol}`}
                        className="text-slate-400 hover:text-rose-300"
                        onClick={() => onDelete(row)}
                      >
                        <Trash2 />
                      </Button>
                    </div>
                  </TableCell>
                </MotionTr>
              );
            })}
          </AnimatePresence>
        </TableBody>
      </Table>
    </Card>
  );
}
