import { apiRequest } from "@/api/client";
import type { InstrumentSearchResponse } from "@/types/instruments";

export function searchInstruments(
  q: string,
  signal?: AbortSignal,
): Promise<InstrumentSearchResponse> {
  return apiRequest<InstrumentSearchResponse>(
    `/instruments/search?q=${encodeURIComponent(q)}`,
    { signal },
  );
}
