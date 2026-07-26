import { ChevronDown, ChevronRight, Loader2 } from "lucide-react";
import { useState } from "react";

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { PreviewRow } from "@/components/portfolio/PreviewRow";
import { useClosePosition, useSalePreview } from "@/hooks/usePositions";
import type { Position } from "@/types/portfolio";
import { formatMoney } from "@/utils/formatMoney";

type Props = {
  position: Position | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

const QUANTITY_SHORTCUTS = [0.25, 0.5, 0.75, 1] as const;

export function ClosePositionDialog({ position, open, onOpenChange }: Props) {
  const closePosition = useClosePosition();
  const pending = closePosition.isPending;
  const [quantity, setQuantity] = useState("");
  const [executionPrice, setExecutionPrice] = useState("");
  const [fee, setFee] = useState("");
  const [effectiveAt, setEffectiveAt] = useState("");
  const [showExecutionDetails, setShowExecutionDetails] = useState(false);

  // Optional execution details. Blank values are simply omitted, so the backend
  // estimates the price from the latest quote, assumes no fee, and uses now.
  const parsedPrice = Number(executionPrice);
  const hasEnteredPrice =
    executionPrice.trim() !== "" && Number.isFinite(parsedPrice) && parsedPrice > 0;
  const parsedFee = Number(fee);
  const hasEnteredFee = fee.trim() !== "" && Number.isFinite(parsedFee) && parsedFee >= 0;
  const parsedDate = effectiveAt.trim() === "" ? null : new Date(effectiveAt);
  const hasValidDate = parsedDate !== null && !Number.isNaN(parsedDate.getTime());
  const executionDetails = {
    ...(hasEnteredPrice ? { execution_price: parsedPrice } : {}),
    ...(hasEnteredFee && parsedFee > 0 ? { fee: parsedFee } : {}),
    ...(hasValidDate ? { effective_at: parsedDate.toISOString() } : {}),
  };

  const numericQuantity = quantity === "" ? 0 : Number(quantity);
  const validQuantity =
    position !== null &&
    Number.isFinite(numericQuantity) &&
    numericQuantity > 0 &&
    numericQuantity <= position.quantity;

  const preview = useSalePreview(
    position && validQuantity
      ? { position_id: position.id, quantity: numericQuantity, ...executionDetails }
      : null,
  );

  const handleConfirm = (e: React.MouseEvent) => {
    e.preventDefault();
    if (!position || !validQuantity) return;
    closePosition.mutate(
      { position_id: position.id, quantity: numericQuantity, ...executionDetails },
      { onSuccess: () => handleOpenChange(false) },
    );
  };

  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen) {
      setQuantity("");
      setExecutionPrice("");
      setFee("");
      setEffectiveAt("");
      setShowExecutionDetails(false);
    }
    onOpenChange(nextOpen);
  };

  const applyShortcut = (fraction: number) => {
    if (!position) return;
    const value = fraction === 1 ? position.quantity : position.quantity * fraction;
    setQuantity(String(value));
  };

  const previewData = preview.data;
  const currency = previewData?.proceeds_currency ?? position?.currency ?? "USD";

  return (
    <AlertDialog open={open} onOpenChange={handleOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Record sale</AlertDialogTitle>
          <AlertDialogDescription>
            Record a sale that already happened. Proceeds enter cash in the
            quote currency. No brokerage order is placed.
          </AlertDialogDescription>
          <label className="mt-3 grid gap-1 text-sm text-zinc-300">
            Quantity to sell (owned: {position?.quantity ?? 0})
            <input
              className="rounded-lg border border-zinc-700 bg-zinc-900 px-3 py-2"
              type="number"
              min="0"
              max={position?.quantity}
              step="any"
              value={quantity}
              placeholder={position ? String(position.quantity) : ""}
              onChange={(event) => setQuantity(event.target.value)}
            />
          </label>
          <div className="mt-2 flex gap-1.5">
            {QUANTITY_SHORTCUTS.map((fraction) => (
              <button
                key={fraction}
                type="button"
                onClick={() => applyShortcut(fraction)}
                disabled={!position}
                className="rounded-md border border-zinc-700 px-2.5 py-1 text-xs font-medium text-zinc-300 transition hover:border-zinc-500 hover:text-zinc-100 disabled:opacity-40"
              >
                {fraction === 1 ? "Max" : `${fraction * 100}%`}
              </button>
            ))}
          </div>

          <div className="mt-3 rounded-lg border border-zinc-800 bg-zinc-900/40">
            <button
              type="button"
              onClick={() => setShowExecutionDetails((open) => !open)}
              aria-expanded={showExecutionDetails}
              className="flex w-full items-center gap-1.5 px-3 py-2 text-left text-xs font-medium text-zinc-300 transition hover:text-zinc-100"
            >
              {showExecutionDetails ? (
                <ChevronDown className="h-3.5 w-3.5" />
              ) : (
                <ChevronRight className="h-3.5 w-3.5" />
              )}
              Execution details (optional)
            </button>
            {showExecutionDetails && (
              <div className="grid gap-3 border-t border-zinc-800 px-3 py-3 text-xs">
                <label className="grid gap-1 text-zinc-300">
                  Execution price — leave blank to estimate from the latest quote
                  <input
                    className="rounded-lg border border-zinc-700 bg-zinc-900 px-3 py-2"
                    type="number"
                    min="0"
                    step="any"
                    value={executionPrice}
                    onChange={(event) => setExecutionPrice(event.target.value)}
                  />
                </label>
                <label className="grid gap-1 text-zinc-300">
                  Transaction fee — leave blank for none
                  <input
                    className="rounded-lg border border-zinc-700 bg-zinc-900 px-3 py-2"
                    type="number"
                    min="0"
                    step="any"
                    value={fee}
                    placeholder="0"
                    onChange={(event) => setFee(event.target.value)}
                  />
                </label>
                <label className="grid gap-1 text-zinc-300">
                  Trade date — leave blank for now
                  <input
                    className="rounded-lg border border-zinc-700 bg-zinc-900 px-3 py-2"
                    type="datetime-local"
                    value={effectiveAt}
                    onChange={(event) => setEffectiveAt(event.target.value)}
                  />
                </label>
              </div>
            )}
          </div>

          {validQuantity && (
            <div
              aria-live="polite"
              className="mt-3 rounded-lg border border-zinc-800 bg-zinc-900/60 p-3 text-xs"
            >
              {preview.isLoading ? (
                <div className="flex items-center gap-2 text-zinc-500">
                  <Loader2 className="h-3 w-3 animate-spin" />
                  Calculating preview…
                </div>
              ) : preview.isError ? (
                <p className="text-rose-300">
                  Could not calculate a preview for this sale.
                </p>
              ) : previewData ? (
                <dl className="grid grid-cols-2 gap-y-1.5">
                  <PreviewRow
                    label={
                      previewData.execution_price_source === "user_recorded"
                        ? "Execution price (entered)"
                        : "Execution price (estimated)"
                    }
                    value={formatMoney(previewData.execution_price, currency)}
                  />
                  <PreviewRow label="Gross proceeds" value={formatMoney(previewData.gross_proceeds, currency)} />
                  <PreviewRow label="Transaction fee" value={`-${formatMoney(previewData.fee, currency)}`} />
                  <PreviewRow
                    label="Net cash received"
                    value={formatMoney(previewData.net_proceeds, currency)}
                    emphasize
                  />
                  <PreviewRow label="Allocated cost basis" value={formatMoney(previewData.allocated_basis, previewData.base_currency)} />
                  <PreviewRow
                    label="Estimated realized P&L"
                    value={formatMoney(previewData.estimated_realized_pnl, previewData.base_currency)}
                    tone={previewData.estimated_realized_pnl >= 0 ? "positive" : "negative"}
                  />
                  <PreviewRow label="Remaining after sale" value={`${previewData.remaining_quantity} shares`} />
                  {previewData.execution_price_source !== "user_recorded" && (
                    <p className="col-span-2 mt-1 text-zinc-500">
                      Price estimated from the latest tracked quote — not a broker
                      confirmation.
                    </p>
                  )}
                  {previewData.will_close_position && (
                    <p className="col-span-2 mt-1 text-amber-300">
                      The position will close automatically.
                    </p>
                  )}
                </dl>
              ) : null}
            </div>
          )}
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={pending}>Cancel</AlertDialogCancel>
          <AlertDialogAction
            onClick={handleConfirm}
            disabled={pending || !validQuantity}
          >
            {pending ? (
              <>
                <Loader2 className="animate-spin" />
                Recording…
              </>
            ) : (
              "Record sale"
            )}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
