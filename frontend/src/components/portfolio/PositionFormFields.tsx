import { LockKeyhole } from "lucide-react";

import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ASSET_TYPES, DEMO_SYMBOLS, type AssetType } from "@/types/portfolio";
import type {
  PositionFormErrors,
  PositionFormState,
} from "@/components/portfolio/positionForm";

const ASSET_LABELS: Record<AssetType, string> = {
  stock: "Stock",
  etf: "ETF",
  crypto: "Crypto",
};

type Props = {
  /** Stable prefix so labels/inputs get unique ids across multiple instances. */
  idPrefix: string;
  state: PositionFormState;
  errors: PositionFormErrors;
  disabled?: boolean;
  onChange: (patch: Partial<PositionFormState>) => void;
};

function FieldError({ message }: { message?: string }) {
  if (!message) return null;
  return <p className="text-xs text-rose-400">{message}</p>;
}

export function PositionFormFields({
  idPrefix,
  state,
  errors,
  disabled,
  onChange,
}: Props) {
  const id = (name: string) => `${idPrefix}-${name}`;

  return (
    <div className="grid gap-4">
      {/* Symbol */}
      <div className="grid gap-1.5">
        <Label htmlFor={id("symbol")}>Symbol</Label>
        <Input
          id={id("symbol")}
          name="symbol"
          autoComplete="off"
          autoCapitalize="characters"
          placeholder="AAPL"
          value={state.symbol}
          disabled={disabled}
          aria-invalid={!!errors.symbol}
          aria-describedby={id("symbol-help")}
          className="uppercase tabular-nums tracking-wide"
          onChange={(e) => onChange({ symbol: e.target.value.toUpperCase() })}
        />
        <p id={id("symbol-help")} className="text-xs text-muted-foreground">
          Demo: {DEMO_SYMBOLS.slice(0, 7).join(", ")}
        </p>
        <FieldError message={errors.symbol} />
      </div>

      {/* Asset type + Quantity */}
      <div className="grid grid-cols-2 gap-4">
        <div className="grid gap-1.5">
          <Label htmlFor={id("asset_type")}>Asset Type</Label>
          <Select
            value={state.asset_type}
            disabled={disabled}
            onValueChange={(v) => onChange({ asset_type: v as AssetType })}
          >
            <SelectTrigger id={id("asset_type")} aria-label="Asset type">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {ASSET_TYPES.map((t) => (
                <SelectItem key={t} value={t}>
                  {ASSET_LABELS[t]}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <FieldError message={errors.asset_type} />
        </div>

        <div className="grid gap-1.5">
          <Label htmlFor={id("quantity")}>Quantity</Label>
          <Input
            id={id("quantity")}
            name="quantity"
            type="number"
            inputMode="decimal"
            step="any"
            min="0"
            placeholder="10"
            value={state.quantity}
            disabled={disabled}
            aria-invalid={!!errors.quantity}
            className="tabular-nums"
            onChange={(e) => onChange({ quantity: e.target.value })}
          />
          <FieldError message={errors.quantity} />
        </div>
      </div>

      {/* Baseline note — there is no price/currency input by design. */}
      <div className="flex items-start gap-2 rounded-xl border border-zinc-800 bg-zinc-900/40 px-3 py-2.5 text-xs text-zinc-400">
        <LockKeyhole className="mt-0.5 h-3.5 w-3.5 shrink-0 text-emerald-300" />
        <span>
          Added at today’s market price. Your index starts at{" "}
          <span className="font-medium text-zinc-200">100</span> and tracks
          performance from here — no buy price needed.
        </span>
      </div>
    </div>
  );
}
