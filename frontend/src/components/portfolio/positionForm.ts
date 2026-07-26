import type { AssetType, CreatePositionInput } from "@/types/portfolio";

/** Raw string-backed form state (inputs are strings until parsed/validated). */
export type PositionFormState = {
  symbol: string;
  asset_type: AssetType;
  quantity: string;
  /** Optional real execution details. Blank means "let the backend estimate". */
  execution_price: string;
  fee: string;
  effective_at: string;
};

export type PositionFormErrors = Partial<
  Record<keyof PositionFormState, string>
>;

export const EMPTY_POSITION_FORM: PositionFormState = {
  symbol: "",
  asset_type: "stock",
  quantity: "",
  execution_price: "",
  fee: "",
  effective_at: "",
};

const SYMBOL_PATTERN = /^[A-Z0-9.-]+$/;

export type ValidationResult =
  | { ok: true; value: CreatePositionInput }
  | { ok: false; errors: PositionFormErrors };

/**
 * Mirror of the backend's symbol/quantity rules so the user gets instant
 * feedback. There is no price/currency input: the backend locks the baseline at
 * today's market quote, so every position starts at index 100. The backend
 * remains the source of truth (priceability is checked server-side).
 */
export function validatePositionForm(
  state: PositionFormState,
): ValidationResult {
  const errors: PositionFormErrors = {};

  const symbol = state.symbol.trim().toUpperCase();
  if (!symbol) {
    errors.symbol = "Symbol is required.";
  } else if (symbol.length > 20) {
    errors.symbol = "Symbol must be 20 characters or fewer.";
  } else if (!SYMBOL_PATTERN.test(symbol)) {
    errors.symbol = "Use only A–Z, 0–9, dot and dash.";
  }

  const quantity = Number(state.quantity);
  if (state.quantity.trim() === "" || Number.isNaN(quantity)) {
    errors.quantity = "Enter a quantity.";
  } else if (quantity <= 0) {
    errors.quantity = "Quantity must be greater than 0.";
  }

  // Optional execution details. Blank is always valid: the backend then uses the
  // latest quote / zero fee / now and labels the price as an estimate.
  let executionPrice: number | undefined;
  if (state.execution_price.trim() !== "") {
    const parsed = Number(state.execution_price);
    if (!Number.isFinite(parsed) || parsed <= 0) {
      errors.execution_price = "Execution price must be greater than 0.";
    } else {
      executionPrice = parsed;
    }
  }

  let fee: number | undefined;
  if (state.fee.trim() !== "") {
    const parsed = Number(state.fee);
    if (!Number.isFinite(parsed) || parsed < 0) {
      errors.fee = "Fee cannot be negative.";
    } else if (parsed > 0) {
      fee = parsed;
    }
  }

  let effectiveAt: string | undefined;
  if (state.effective_at.trim() !== "") {
    const parsed = new Date(state.effective_at);
    if (Number.isNaN(parsed.getTime())) {
      errors.effective_at = "Enter a valid date.";
    } else if (parsed.getTime() > Date.now() + 60_000) {
      errors.effective_at = "The trade date cannot be in the future.";
    } else {
      effectiveAt = parsed.toISOString();
    }
  }

  if (Object.keys(errors).length > 0) {
    return { ok: false, errors };
  }

  return {
    ok: true,
    value: {
      symbol,
      asset_type: state.asset_type,
      quantity,
      ...(executionPrice !== undefined ? { execution_price: executionPrice } : {}),
      ...(fee !== undefined ? { fee } : {}),
      ...(effectiveAt !== undefined ? { effective_at: effectiveAt } : {}),
    },
  };
}
