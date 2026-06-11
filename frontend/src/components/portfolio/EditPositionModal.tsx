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
import { PositionFormFields } from "@/components/portfolio/PositionFormFields";
import {
  validatePositionForm,
  type PositionFormErrors,
  type PositionFormState,
} from "@/components/portfolio/positionForm";
import { useUpdatePosition } from "@/hooks/usePositions";
import type { Position } from "@/types/portfolio";

type Props = {
  position: Position | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

function toFormState(position: Position): PositionFormState {
  return {
    symbol: position.symbol,
    asset_type: position.asset_type,
    quantity: String(position.quantity),
    average_buy_price: String(position.average_buy_price),
    currency: position.currency,
  };
}

/**
 * Inner form, initialized directly from the position prop. It is keyed by the
 * position id in the parent so switching positions (or reopening) remounts it
 * with fresh state — no effect-driven syncing required.
 */
function EditPositionForm({
  position,
  onClose,
}: {
  position: Position;
  onClose: () => void;
}) {
  const [state, setState] = useState<PositionFormState>(() =>
    toFormState(position),
  );
  const [errors, setErrors] = useState<PositionFormErrors>({});
  const [backendError, setBackendError] = useState<string | null>(null);

  const updatePosition = useUpdatePosition();

  const onChange = (patch: Partial<PositionFormState>) => {
    setState((prev) => ({ ...prev, ...patch }));
    setErrors((prev) => {
      const next = { ...prev };
      for (const key of Object.keys(patch)) {
        delete next[key as keyof PositionFormState];
      }
      return next;
    });
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    setBackendError(null);

    const result = validatePositionForm(state);
    if (!result.ok) {
      setErrors(result.errors);
      return;
    }
    setErrors({});

    updatePosition.mutate(
      { id: position.id, input: result.value },
      {
        onSuccess: () => onClose(),
        onError: (err: Error) => setBackendError(err.message),
      },
    );
  };

  const pending = updatePosition.isPending;

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

      <PositionFormFields
        idPrefix="edit"
        state={state}
        errors={errors}
        disabled={pending}
        onChange={onChange}
      />

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
            Update the details of {position?.symbol ?? "this position"}.
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
