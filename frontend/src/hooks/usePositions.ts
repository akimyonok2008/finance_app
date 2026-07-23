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
  getPortfolioArchives,
  getPositions,
  updatePosition,
} from "@/api/portfolioApi";
import { POSITION_MUTATION_INVALIDATIONS, queryKeys } from "@/hooks/queryKeys";
import type {
  CreatePositionInput,
  PortfolioArchiveTimeframe,
  UpdatePositionInput,
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

export function usePortfolioArchives(timeframe: PortfolioArchiveTimeframe) {
  return useQuery({
    queryKey: queryKeys.portfolioArchives(timeframe),
    queryFn: ({ signal }) => getPortfolioArchives(timeframe, signal),
  });
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
    mutationFn: (id: string) => closePosition(id),
    onSuccess: () => {
      invalidate();
      toast.success("Position closed");
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
      toast.success("Position added");
    },
    onError: (err: Error) => {
      toast.error(err.message);
    },
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
