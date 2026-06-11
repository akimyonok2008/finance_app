import { apiRequest } from "@/api/client";
import type {
  CreatePositionInput,
  PortfolioSummary,
  Position,
  UpdatePositionInput,
} from "@/types/portfolio";

export function getPositions(signal?: AbortSignal): Promise<Position[]> {
  return apiRequest<Position[]>("/portfolio/positions", { signal });
}

export function getPortfolioSummary(
  signal?: AbortSignal,
): Promise<PortfolioSummary> {
  return apiRequest<PortfolioSummary>("/portfolio/summary", { signal });
}

export function createPosition(
  input: CreatePositionInput,
): Promise<Position> {
  return apiRequest<Position>("/portfolio/positions", {
    method: "POST",
    body: input,
  });
}

export function updatePosition(
  id: string,
  input: UpdatePositionInput,
): Promise<Position> {
  return apiRequest<Position>(`/portfolio/positions/${id}`, {
    method: "PUT",
    body: input,
  });
}

export function deletePosition(id: string): Promise<void> {
  return apiRequest<void>(`/portfolio/positions/${id}`, { method: "DELETE" });
}
