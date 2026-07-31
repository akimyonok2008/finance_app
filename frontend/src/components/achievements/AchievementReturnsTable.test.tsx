import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { getAchievementReturns } from "@/api/achievements";
import { AchievementReturnsTable } from "@/components/achievements/AchievementReturnsTable";

vi.mock("@/api/achievements", () => ({ getAchievementReturns: vi.fn() }));

const mockedReturns = vi.mocked(getAchievementReturns);

function renderTable() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <AchievementReturnsTable timeframe="1M" onTimeframeChange={vi.fn()} />
    </QueryClientProvider>,
  );
}

describe("AchievementReturnsTable", () => {
  beforeEach(() => {
    mockedReturns.mockResolvedValue({
      timeframe: "1M",
      to: "2026-08-01T00:00:00Z",
      rows: Array.from({ length: 8 }, (_, index) => ({
        key: `badge-${index + 1}`,
        name: `Achievement ${index + 1}`,
        difficulty: "medium",
        native_period: "30d",
        available: true,
        portfolio_return_percentage: index,
        benchmark_return_percentage: index - 1,
        edge_points: 1,
      })),
    });
  });

  it("shows five rows by default and toggles the complete return list", async () => {
    renderTable();
    await waitFor(() => expect(screen.getByText("Achievement 5")).not.toBeNull());
    expect(screen.queryByText("Achievement 6")).toBeNull();

    const expand = screen.getByRole("button", { name: "Show all 8 achievements" });
    expect(expand.getAttribute("aria-expanded")).toBe("false");
    fireEvent.click(expand);

    expect(screen.getByText("Achievement 8")).not.toBeNull();
    const collapse = screen.getByRole("button", { name: "Show fewer achievements" });
    expect(collapse.getAttribute("aria-expanded")).toBe("true");
    fireEvent.click(collapse);
    expect(screen.queryByText("Achievement 6")).toBeNull();
  });
});
