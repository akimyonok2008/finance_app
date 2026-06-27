import { useMutation } from "@tanstack/react-query";

import { requestPortfolioCoach } from "@/api/coach";
import type { CoachMode, CoachResponse } from "@/types/coach";

/**
 * Portfolio Coach is request-driven: the user explicitly clicks a mode, so this
 * is a mutation, not a query. The latest result lives in the mutation state —
 * we deliberately do NOT cache or persist AI responses.
 */
export function useCoach() {
  return useMutation<CoachResponse, Error, CoachMode>({
    mutationFn: (mode) => requestPortfolioCoach(mode),
  });
}
