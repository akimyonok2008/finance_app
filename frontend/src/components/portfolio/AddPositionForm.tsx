import { useEffect, useState } from "react";
import { AlertCircle, ChevronDown, ChevronRight, Loader2, Plus } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { PositionFormFields } from "@/components/portfolio/PositionFormFields";
import { PreviewRow } from "@/components/portfolio/PreviewRow";
import {
  EMPTY_POSITION_FORM,
  validatePositionForm,
  type PositionFormErrors,
  type PositionFormState,
} from "@/components/portfolio/positionForm";
import { useCreatePosition, useCashBalances } from "@/hooks/usePositions";
import { useQuotes } from "@/hooks/useQuotes";
import { cn } from "@/utils/cn";
import { formatMoney } from "@/utils/formatMoney";
import { decimalToChartNumber } from "@/utils/decimal";

export type AddPositionFormProps = {
  /** Called after a successful add (e.g. to close the mobile drawer). */
  onSuccess?: () => void;
  /** Render without the surrounding Card chrome (used inside the drawer). */
  compact?: boolean;
};

export function AddPositionForm({ onSuccess, compact }: AddPositionFormProps) {
  const [state, setState] = useState<PositionFormState>(EMPTY_POSITION_FORM);
  const [errors, setErrors] = useState<PositionFormErrors>({});
  const [backendError, setBackendError] = useState<string | null>(null);
  const [showExecutionDetails, setShowExecutionDetails] = useState(false);

  const createPosition = useCreatePosition();

  // Live cost preview: debounce the symbol so we don't fire a quote lookup on
  // every keystroke, then compute total cost and remaining cash once both a
  // symbol and a valid quantity are present.
  const [debouncedSymbol, setDebouncedSymbol] = useState(state.symbol);
  useEffect(() => {
    const timer = window.setTimeout(
      () => setDebouncedSymbol(state.symbol.trim().toUpperCase()),
      350,
    );
    return () => window.clearTimeout(timer);
  }, [state.symbol]);

  const quantityValue = Number(state.quantity);
  const hasValidQuantity = Number.isFinite(quantityValue) && quantityValue > 0;
  const quotes = useQuotes(debouncedSymbol && hasValidQuantity ? [debouncedSymbol] : []);
  const quote = quotes.data?.quotes.find((q) => q.symbol === debouncedSymbol);
  const cash = useCashBalances();

  // Execution price: what the user entered, else the latest quote (an estimate).
  const enteredPrice = Number(state.execution_price);
  const hasEnteredPrice =
    state.execution_price.trim() !== "" && Number.isFinite(enteredPrice) && enteredPrice > 0;
  const executionPrice = hasEnteredPrice ? enteredPrice : quote?.price ?? null;

  const enteredFee = Number(state.fee);
  const feeValue =
    state.fee.trim() !== "" && Number.isFinite(enteredFee) && enteredFee >= 0 ? enteredFee : 0;

  // This is a pre-submit UI estimate only — the backend recomputes the
  // authoritative buy preview/result. Converting the decimal-string cash
  // balance to a number here is the sanctioned display-boundary use.
  const grossCost = executionPrice !== null ? quantityValue * executionPrice : null;
  const totalCost = grossCost !== null ? grossCost + feeValue : null;
  const availableCash = quote
    ? decimalToChartNumber(
        cash.data?.cash_balances.find((b) => b.currency === quote.currency)?.amount,
      ) ?? 0
    : null;
  // Automatic funding is the default: a shortfall is funded, not rejected, so it
  // is surfaced as information and never blocks the submit button.
  const automaticFunding =
    totalCost !== null && availableCash !== null ? Math.max(totalCost - availableCash, 0) : null;
  const remainingCash =
    totalCost !== null && availableCash !== null && automaticFunding !== null
      ? availableCash + automaticFunding - totalCost
      : null;

  const onChange = (patch: Partial<PositionFormState>) => {
    setState((prev) => ({ ...prev, ...patch }));
    // Clear the field-level error as the user edits it.
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

    createPosition.mutate(result.value, {
      onSuccess: () => {
        // Clear the form, then let the parent close the drawer if mobile.
        setState(EMPTY_POSITION_FORM);
        onSuccess?.();
      },
      onError: (err: Error) => {
        // Keep the user's values; surface the backend message inline + toast.
        setBackendError(err.message);
      },
    });
  };

  const pending = createPosition.isPending;

  const formBody = (
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
        idPrefix="add"
        state={state}
        errors={errors}
        disabled={pending}
        onChange={onChange}
      />

      <div className="rounded-lg border border-zinc-800 bg-zinc-900/40">
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
                disabled={pending}
                value={state.execution_price}
                placeholder={quote ? String(quote.price) : "Latest quote"}
                onChange={(event) => onChange({ execution_price: event.target.value })}
              />
              {errors.execution_price && (
                <span className="text-rose-300">{errors.execution_price}</span>
              )}
            </label>
            <label className="grid gap-1 text-zinc-300">
              Transaction fee — leave blank for none
              <input
                className="rounded-lg border border-zinc-700 bg-zinc-900 px-3 py-2"
                type="number"
                min="0"
                step="any"
                disabled={pending}
                value={state.fee}
                placeholder="0"
                onChange={(event) => onChange({ fee: event.target.value })}
              />
              {errors.fee && <span className="text-rose-300">{errors.fee}</span>}
            </label>
          </div>
        )}
      </div>

      {debouncedSymbol && hasValidQuantity && (
        <div
          aria-live="polite"
          className="rounded-lg border border-zinc-800 bg-zinc-900/60 p-3 text-xs"
        >
          {quotes.isLoading ? (
            <div className="flex items-center gap-2 text-zinc-500">
              <Loader2 className="h-3 w-3 animate-spin" />
              Calculating cost…
            </div>
          ) : !quote ? (
            <p className="text-rose-300">Price unavailable for {debouncedSymbol}.</p>
          ) : (
            <dl className="grid grid-cols-2 gap-y-1.5">
              <PreviewRow
                label={hasEnteredPrice ? "Execution price (entered)" : "Execution price (estimated)"}
                value={formatMoney(executionPrice, quote.currency)}
              />
              {feeValue > 0 && (
                <PreviewRow label="Transaction fee" value={formatMoney(feeValue, quote.currency)} />
              )}
              <PreviewRow label="Total cost" value={formatMoney(totalCost, quote.currency)} emphasize />
              <PreviewRow label="Available cash" value={formatMoney(availableCash, quote.currency)} />
              {automaticFunding !== null && automaticFunding > 0 && (
                <PreviewRow
                  label="Automatic funding required"
                  value={formatMoney(automaticFunding, quote.currency)}
                />
              )}
              <PreviewRow
                label="Remaining after buy"
                value={formatMoney(remainingCash, quote.currency)}
                tone="positive"
              />
              {!hasEnteredPrice && (
                <p className="col-span-2 mt-1 text-zinc-500">
                  Price estimated from the latest tracked quote — not a broker
                  confirmation. Enter the real price below if you have it.
                </p>
              )}
              {automaticFunding !== null && automaticFunding > 0 && (
                <p className="col-span-2 mt-1 text-amber-300">
                  {formatMoney(automaticFunding, quote.currency)} will be recorded
                  as automatic funding in {quote.currency}. Other currencies are
                  never converted.
                </p>
              )}
            </dl>
          )}
        </div>
      )}

      <Button
        type="submit"
        variant="accent"
        className="w-full"
        disabled={pending}
      >
        {pending ? (
          <>
            <Loader2 className="animate-spin" />
            Recording…
          </>
        ) : (
          <>
            <Plus />
            Record buy
          </>
        )}
      </Button>
    </form>
  );

  if (compact) {
    return formBody;
  }

  return (
    <Card
      className={cn(
        "sticky top-6 overflow-hidden border-cyan-300/15 bg-[radial-gradient(circle_at_top_left,rgba(34,211,238,0.075),transparent_38%),rgba(24,24,27,0.55)] shadow-xl shadow-black/15",
      )}
    >
      <CardHeader>
        <CardTitle className="portfolio-display text-xl">Record buy</CardTitle>
        <CardDescription>
          Uses available cash in the asset’s quote currency. No brokerage order is placed.
        </CardDescription>
      </CardHeader>
      <CardContent>{formBody}</CardContent>
    </Card>
  );
}
