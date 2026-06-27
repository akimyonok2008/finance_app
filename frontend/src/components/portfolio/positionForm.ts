import type { AssetType, CreatePositionInput } from "@/types/portfolio";

/** Raw string-backed form state (inputs are strings until parsed/validated). */
export type PositionFormState = {
  symbol: string;
  asset_type: AssetType;
  quantity: string;
};

export type PositionFormErrors = Partial<
  Record<keyof PositionFormState, string>
>;

export const EMPTY_POSITION_FORM: PositionFormState = {
  symbol: "",
  asset_type: "stock",
  quantity: "",
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

  if (Object.keys(errors).length > 0) {
    return { ok: false, errors };
  }

  return {
    ok: true,
    value: {
      symbol,
      asset_type: state.asset_type,
      quantity,
    },
  };
}
