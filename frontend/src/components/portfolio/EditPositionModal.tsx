import { useState } from "react";
import { AlertCircle, Loader2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { AssetTypeBadge } from "@/components/portfolio/AssetTypeBadge";
import { useUpdatePosition } from "@/hooks/usePositions";
import type { Position } from "@/types/portfolio";
import { formatMoney } from "@/utils/formatMoney";

type Props = {
  position: Position | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

/**
 * Quantity-only edit. The symbol and locked baseline price are immutable — to
 * change a holding, delete it and re-add (which locks a fresh baseline at that
 * day's price). This keeps ranked performance fair.
 */
function EditPositionForm({
  position,
  onClose,
}: {
  position: Position;
  onClose: () => void;
}) {
  const [quantity, setQuantity] = useState<string>(() =>
    String(position.quantity),
  );
  const [error, setError] = useState<string | null>(null);
  const [backendError, setBackendError] = useState<string | null>(null);

  const updatePosition = useUpdatePosition();
  const pending = updatePosition.isPending;

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    setBackendError(null);

    const value = Number(quantity);
    if (quantity.trim() === "" || Number.isNaN(value)) {
      setError("Enter a quantity.");
      return;
    }
    if (value <= 0) {
      setError("Quantity must be greater than 0.");
      return;
    }
    setError(null);

    updatePosition.mutate(
      { id: position.id, input: { quantity: value } },
      {
        onSuccess: () => onClose(),
        onError: (err: Error) => setBackendError(err.message),
      },
    );
  };

  return (
    <form onSubmit={handleSubmit} className="grid gap-5" noValidate>
      {backendError && (
        <div
          role="alert"
          className="flex items-start gap-2 rounded-xl border border-rose-500/30 bg-rose-500/10 px-3 py-2.5 text-sm text-rose-200"
        >
          <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />
          <span>{backendError}</span>
        </div>
      )}

      {/* Immutable identity + locked baseline, shown read-only for context. */}
      <div className="flex items-center justify-between rounded-xl border border-zinc-800 bg-zinc-900/40 px-3 py-2.5">
        <div className="flex items-center gap-2">
          <span className="text-base font-semibold tracking-wide">
            {position.symbol}
          </span>
          <AssetTypeBadge type={position.asset_type} />
        </div>
        <div className="text-right">
          <div className="text-[11px] uppercase tracking-wide text-muted-foreground">
            Baseline
          </div>
          <div className="text-sm tabular-nums text-slate-300">
            {formatMoney(position.baseline_price, position.currency)}
          </div>
        </div>
      </div>

      <div className="grid gap-1.5">
        <Label htmlFor="edit-quantity">Quantity</Label>
        <Input
          id="edit-quantity"
          name="quantity"
          type="number"
          inputMode="decimal"
          step="any"
          min="0"
          value={quantity}
          disabled={pending}
          aria-invalid={!!error}
          className="tabular-nums"
          onChange={(e) => {
            setQuantity(e.target.value);
            setError(null);
          }}
        />
        {error && <p className="text-xs text-rose-400">{error}</p>}
        <p className="text-xs text-muted-foreground">
          Symbol and baseline price can’t change. Delete and re-add to reset the
          baseline at today’s price.
        </p>
      </div>

      <DialogFooter>
        <Button
          type="button"
          variant="outline"
          onClick={onClose}
          disabled={pending}
        >
          Cancel
        </Button>
        <Button type="submit" variant="accent" disabled={pending}>
          {pending ? (
            <>
              <Loader2 className="animate-spin" />
              Saving…
            </>
          ) : (
            "Save changes"
          )}
        </Button>
      </DialogFooter>
    </form>
  );
}

export function EditPositionModal({ position, open, onOpenChange }: Props) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Edit position</DialogTitle>
          <DialogDescription>
            Update the quantity of {position?.symbol ?? "this position"}.
          </DialogDescription>
        </DialogHeader>

        {position && (
          <EditPositionForm
            key={position.id}
            position={position}
            onClose={() => onOpenChange(false)}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}
