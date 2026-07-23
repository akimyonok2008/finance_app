import { apiRequest } from "@/api/client";
import type {
  CreatePositionInput,
  ClosedPosition,
  PortfolioArchives,
  PortfolioArchiveTimeframe,
  PortfolioSummary,
  Position,
  UpdatePositionInput,
} from "@/types/portfolio";

export function getPositions(signal?: AbortSignal): Promise<Position[]> {
  return apiRequest<Position[]>("/portfolio/positions", { signal });
}

export function getClosedPositions(signal?: AbortSignal): Promise<ClosedPosition[]> {
  return apiRequest<ClosedPosition[]>("/portfolio/positions/closed", { signal });
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

export function closePosition(id: string): Promise<ClosedPosition> {
  return apiRequest<ClosedPosition>(`/portfolio/positions/${id}/close`, {
    method: "POST",
    body: {},
  });
}

export function deletePosition(id: string): Promise<void> {
  return apiRequest<void>(`/portfolio/positions/${id}`, { method: "DELETE" });
}

export function getPortfolioArchives(
  timeframe: PortfolioArchiveTimeframe,
  signal?: AbortSignal,
): Promise<PortfolioArchives> {
  return apiRequest<PortfolioArchives>(`/portfolio/archives?timeframe=${timeframe}`, {
    signal,
  });
}
