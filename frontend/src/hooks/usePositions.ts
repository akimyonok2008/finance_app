import {
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { toast } from "sonner";

import {
  closePosition,
  createPosition,
  deletePosition,
  getClosedPositions,
  getPositions,
  updatePosition,
  depositCash,
  getActivities,
  getCash,
  withdrawCash,
  recordFee,
  getCorporateActions,
  getIncomeEvents,
  correctIncomeEvent,
  type FeeInput,
  type IncomeCorrectionInput,
  previewSale,
} from "@/api/portfolioApi";
import { POSITION_MUTATION_INVALIDATIONS, queryKeys } from "@/hooks/queryKeys";
import { compareDecimal, isValidDecimalString } from "@/utils/decimal";
import type {
  CreatePositionInput,
  UpdatePositionInput,
  CashFlowInput,
  SellPositionInput,
} from "@/types/portfolio";

export function usePositions() {
  return useQuery({
    queryKey: queryKeys.positions,
    queryFn: ({ signal }) => getPositions(signal),
  });
}

export function useClosedPositions() {
  return useQuery({
    queryKey: queryKeys.closedPositions,
    queryFn: ({ signal }) => getClosedPositions(signal),
  });
}

export function useCashBalances() {
  return useQuery({ queryKey: queryKeys.cash, queryFn: ({ signal }) => getCash(signal) });
}

export function usePortfolioActivities() {
  return useQuery({ queryKey: queryKeys.activities, queryFn: ({ signal }) => getActivities(signal) });
}

/** Invalidate positions + summary + leaderboard + achievements together. */
function useInvalidatePortfolio() {
  const queryClient = useQueryClient();
  return () => {
    for (const queryKey of POSITION_MUTATION_INVALIDATIONS) {
      queryClient.invalidateQueries({ queryKey });
    }
  };
}

export function useClosePosition() {
  const invalidate = useInvalidatePortfolio();
  return useMutation({
    mutationFn: (input: SellPositionInput) => closePosition(input),
    onSuccess: () => {
      invalidate();
      toast.success("Sale recorded");
    },
    onError: (err: Error) => {
      toast.error(err.message);
    },
  });
}

export function useCreatePosition() {
  const invalidate = useInvalidatePortfolio();
  return useMutation({
    mutationFn: (input: CreatePositionInput) => createPosition(input),
    onSuccess: () => {
      invalidate();
      toast.success("Buy recorded");
    },
    onError: (err: Error) => {
      toast.error(err.message);
    },
  });
}

export function useDepositCash() {
  const invalidate = useInvalidatePortfolio();
  return useMutation({
    mutationFn: (input: CashFlowInput) => depositCash(input),
    onSuccess: () => { invalidate(); toast.success("Deposit recorded"); },
    onError: (err: Error) => toast.error(err.message),
  });
}

export function useWithdrawCash() {
  const invalidate = useInvalidatePortfolio();
  return useMutation({
    mutationFn: (input: CashFlowInput) => withdrawCash(input),
    onSuccess: () => { invalidate(); toast.success("Withdrawal recorded"); },
    onError: (err: Error) => toast.error(err.message),
  });
}

export function useRecordFee() {
  const invalidate = useInvalidatePortfolio();
  return useMutation({
    mutationFn: (input: FeeInput) => recordFee(input),
    onSuccess: () => { invalidate(); toast.success("Fee recorded for tracking"); },
    onError: (err: Error) => toast.error(err.message),
  });
}

export function useSalePreview(input: SellPositionInput | null) {
  return useQuery({
    queryKey: queryKeys.portfolio.sellPreview(
      input?.position_id ?? "",
      input?.quantity ?? "0",
      input?.execution_price ?? "0",
      input?.fee ?? "0",
    ),
    queryFn: () => previewSale(input!),
    enabled: Boolean(
      input && isValidDecimalString(input.quantity) && compareDecimal(input.quantity, "0") > 0,
    ),
    retry: false,
  });
}

// useCorporateActions reads the owner-private, read-only automatic-adjustments
// history. Corporate actions are applied by the background pipeline; there is no
// create/edit mutation.
export function useCorporateActions() {
  return useQuery({
    queryKey: queryKeys.corporateActions,
    queryFn: ({ signal }) => getCorporateActions(signal),
  });
}

// useIncomeEvents reads the owner-private, read-only automatic-income history
// (dividends, distributions, coupons, return of capital, stock dividends).
// Income is detected and credited by the background pipeline; there is no
// create mutation — only a constrained correction path.
export function useIncomeEvents() {
  return useQuery({
    queryKey: queryKeys.incomeEvents,
    queryFn: ({ signal }) => getIncomeEvents(signal),
  });
}

// useCorrectIncomeEvent reconciles an already-detected event to the actual
// broker outcome. It cannot fabricate arbitrary income.
export function useCorrectIncomeEvent() {
  const invalidate = useInvalidatePortfolio();
  return useMutation({
    mutationFn: ({ id, input }: { id: string; input: IncomeCorrectionInput }) =>
      correctIncomeEvent(id, input),
    onSuccess: () => {
      invalidate();
      toast.success("Income details corrected");
    },
    onError: () => toast.error("Could not correct income details"),
  });
}

export function useUpdatePosition() {
  const invalidate = useInvalidatePortfolio();
  return useMutation({
    mutationFn: ({ id, input }: { id: string; input: UpdatePositionInput }) =>
      updatePosition(id, input),
    onSuccess: () => {
      invalidate();
      toast.success("Position updated");
    },
    onError: (err: Error) => {
      toast.error(err.message);
    },
  });
}

export function useDeletePosition() {
  const invalidate = useInvalidatePortfolio();
  return useMutation({
    mutationFn: (id: string) => deletePosition(id),
    onSuccess: () => {
      invalidate();
      toast.success("Position deleted");
    },
    onError: (err: Error) => {
      toast.error(err.message);
    },
  });
}
