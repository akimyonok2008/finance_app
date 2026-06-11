import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  ASSET_TYPES,
  CURRENCIES,
  DEMO_SYMBOLS,
  type AssetType,
} from "@/types/portfolio";
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

      {/* Asset type + Currency */}
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
          <Label htmlFor={id("currency")}>Currency</Label>
          <Select
            value={state.currency}
            disabled={disabled}
            onValueChange={(v) => onChange({ currency: v })}
          >
            <SelectTrigger id={id("currency")} aria-label="Currency">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {CURRENCIES.map((c) => (
                <SelectItem key={c} value={c}>
                  {c}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <FieldError message={errors.currency} />
        </div>
      </div>

      {/* Quantity + Avg buy price */}
      <div className="grid grid-cols-2 gap-4">
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

        <div className="grid gap-1.5">
          <Label htmlFor={id("average_buy_price")}>Avg Buy Price</Label>
          <Input
            id={id("average_buy_price")}
            name="average_buy_price"
            type="number"
            inputMode="decimal"
            step="any"
            min="0"
            placeholder="180.00"
            value={state.average_buy_price}
            disabled={disabled}
            aria-invalid={!!errors.average_buy_price}
            className="tabular-nums"
            onChange={(e) => onChange({ average_buy_price: e.target.value })}
          />
          <FieldError message={errors.average_buy_price} />
        </div>
      </div>
    </div>
  );
}
