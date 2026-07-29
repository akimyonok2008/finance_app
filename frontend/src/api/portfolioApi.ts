import { apiRequest } from "@/api/client";
import type {
  CreatePositionInput,
  ClosedPosition,
  PortfolioSettings,
  PortfolioSummary,
  Position,
  UpdatePositionInput,
  ActivityMutationResponse,
  CashFlowInput,
  CashResponse,
  PortfolioActivity,
  SellPositionInput,
  SellPreview,
} from "@/types/portfolio";
import type { DecimalString } from "@/utils/decimal";
import {
  assertShape,
  cashResponseSchema,
  sellPreviewSchema,
  activityMutationResponseSchema,
  portfolioSummaryShapeSchema,
} from "@/api/schemas";

export function getPositions(signal?: AbortSignal): Promise<Position[]> {
  return apiRequest<Position[]>("/portfolio/positions", { signal });
}

export function getPortfolioSettings(
  signal?: AbortSignal,
): Promise<PortfolioSettings> {
  return apiRequest<PortfolioSettings>("/portfolio", { signal });
}

export function updatePortfolioSettings(
  input: Partial<Pick<PortfolioSettings, "auto_fund_purchases">>,
): Promise<PortfolioSettings> {
  return apiRequest<PortfolioSettings>("/portfolio/settings", {
    method: "PATCH",
    body: input,
  });
}

export function getClosedPositions(signal?: AbortSignal): Promise<ClosedPosition[]> {
  return apiRequest<ClosedPosition[]>("/portfolio/positions/closed", { signal });
}

export function getPortfolioSummary(
  signal?: AbortSignal,
): Promise<PortfolioSummary> {
  return apiRequest<PortfolioSummary>("/portfolio/summary", { signal }).then((data) => {
    assertShape(portfolioSummaryShapeSchema, data, "GET /portfolio/summary");
    return data;
  });
}

export function createPosition(
  input: CreatePositionInput,
): Promise<Position> {
  return apiRequest<ActivityMutationResponse>("/portfolio/buys", {
    method: "POST",
    body: input,
    idempotencyKey: crypto.randomUUID(),
  }).then((response) => {
    if (!response.position) throw new Error("Buy completed without a position");
    return response.position;
  });
}

export function updatePosition(
  id: string,
  input: UpdatePositionInput,
): Promise<Position> {
  return apiRequest<Position>(`/portfolio/positions/${id}`, {
    method: "PUT",
    body: input,
    idempotencyKey: crypto.randomUUID(),
  });
}

export function closePosition(input: SellPositionInput): Promise<ActivityMutationResponse> {
  return apiRequest<ActivityMutationResponse>("/portfolio/sells", {
    method: "POST",
    body: input,
    idempotencyKey: crypto.randomUUID(),
  }).then((data) => {
    assertShape(activityMutationResponseSchema, data, "POST /portfolio/sells");
    return data;
  });
}

export function previewSale(input: SellPositionInput): Promise<SellPreview> {
  return apiRequest<SellPreview>("/portfolio/sells/preview", {
    method: "POST",
    body: input,
  }).then((data) => {
    assertShape(sellPreviewSchema, data, "POST /portfolio/sells/preview");
    return data;
  });
}

export function getCash(signal?: AbortSignal): Promise<CashResponse> {
  return apiRequest<CashResponse>("/portfolio/cash", { signal }).then((data) => {
    assertShape(cashResponseSchema, data, "GET /portfolio/cash");
    return data;
  });
}

export function getActivities(signal?: AbortSignal): Promise<PortfolioActivity[]> {
  return apiRequest<PortfolioActivity[]>("/portfolio/activities", { signal });
}

export function depositCash(input: CashFlowInput): Promise<ActivityMutationResponse> {
  return apiRequest<ActivityMutationResponse>("/portfolio/deposits", {
    method: "POST", body: input, idempotencyKey: crypto.randomUUID(),
  });
}

export function withdrawCash(input: CashFlowInput): Promise<ActivityMutationResponse> {
  return apiRequest<ActivityMutationResponse>("/portfolio/withdrawals", {
    method: "POST", body: input, idempotencyKey: crypto.randomUUID(),
  });
}

export function deletePosition(id: string): Promise<void> {
  return apiRequest<void>(`/portfolio/positions/${id}`, {
    method: "DELETE",
    idempotencyKey: crypto.randomUUID(),
  });
}

// --- income, fees, corporate actions (user-reported tracking only) -----------

export type FeeInput = {
  type: "management_fee" | "custody_fee" | "other_fee";
  currency: string;
  amount: DecimalString;
  symbol?: string;
  description?: string;
  effective_at?: string;
};

// CorporateActionView is the owner-private, read-only projection of an
// automatically-applied (or pending) corporate action. Users never create these.
export type CorporateActionView = {
  id: string;
  event_type: string;
  display_symbol: string;
  effective_at?: string;
  status: string;
  explanation: string;
  system_generated: boolean;
  applied_at?: string;
};

export function getCorporateActions(
  signal?: AbortSignal,
): Promise<CorporateActionView[]> {
  return apiRequest<CorporateActionView[]>("/portfolio/corporate-actions", { signal });
}

// IncomeEventView is the owner-private, read-only projection of an automatically
// detected and applied income event (dividend, ETF/fund distribution, bond
// coupon, return of capital, stock dividend). Users NEVER create income; the
// background pipeline detects and credits it. The owner may see their own
// amounts; these are never exposed on any public surface.
export type IncomeEventView = {
  id: string;
  event_type: string;
  symbol: string;
  currency: string;
  gross_amount: number;
  withholding_amount: number;
  fee_amount: number;
  net_amount: number;
  reinvestment_quantity?: number;
  estimated: boolean;
  status: string;
  provider: string;
  explanation: string;
  correctable: boolean;
  payment_date?: string;
  applied_at?: string;
  system_generated: boolean;
};

// IncomeCorrectionInput is the constrained, account-specific correction to an
// already-detected event. There is deliberately NO "create income" call.
export type IncomeCorrectionInput = {
  kind:
    | "actual_net"
    | "actual_withholding"
    | "actual_fee"
    | "mark_not_received"
    | "actual_reinvestment";
  /**
   * Verified against internal/portfolio/handler_activities.go: this request
   * struct decodes ActualNet/ActualWithholding/ActualFee as `money.Amount`
   * and ActualReinvestmentQty/Price as `money.Quantity`/`money.Price` — all
   * decimal strings on the wire — even though the income *read* model
   * (IncomeEventView above) is still float64 (untouched by this migration).
   */
  actual_net?: DecimalString;
  actual_withholding?: DecimalString;
  actual_fee?: DecimalString;
  actual_reinvestment_quantity?: DecimalString;
  actual_reinvestment_price?: DecimalString;
};

export function getIncomeEvents(signal?: AbortSignal): Promise<IncomeEventView[]> {
  return apiRequest<IncomeEventView[]>("/portfolio/income-events", { signal });
}

export function getIncomeEvent(
  id: string,
  signal?: AbortSignal,
): Promise<IncomeEventView> {
  return apiRequest<IncomeEventView>(`/portfolio/income-events/${id}`, { signal });
}

export function correctIncomeEvent(
  id: string,
  input: IncomeCorrectionInput,
): Promise<IncomeEventView> {
  return apiRequest<IncomeEventView>(`/portfolio/income-events/${id}/correction`, {
    method: "POST",
    body: input,
    idempotencyKey: crypto.randomUUID(),
  });
}

export function recordFee(input: FeeInput): Promise<ActivityMutationResponse> {
  return apiRequest<ActivityMutationResponse>("/portfolio/fees", {
    method: "POST",
    body: input,
    idempotencyKey: crypto.randomUUID(),
  });
}
