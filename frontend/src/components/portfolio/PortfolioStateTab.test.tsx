import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { StateView } from "@/components/portfolio/portfolioTabs";
import type { ClosedPosition } from "@/types/portfolio";

const OPEN_EPISODE = "11111111-1111-4111-8111-111111111111";
const CLOSED_EPISODE = "22222222-2222-4222-8222-222222222222";

const state = {
  rows: [] as Array<{ id: string; symbol: string; asset_type: string }>,
  closed: [] as ClosedPosition[],
  summary: undefined as
    | {
        base_currency: string;
        positions: Array<{
          symbol: string;
          asset_type: string;
          current_value_base: number;
        }>;
      }
    | undefined,
};

// The State tab's children each own their own data source; only the pieces this
// test drives are stubbed, so the tab's own logic is what is under test.
vi.mock("@/hooks/usePositionRows", () => ({
  usePositionRows: () => ({
    rows: state.rows,
    isLoading: false,
    isError: false,
    error: null,
  }),
  rowCurrency: () => "USD",
  rowBaseCurrency: () => "USD",
  rowPriceCurrency: () => "USD",
}));
vi.mock("@/hooks/usePositions", () => ({
  useClosedPositions: () => ({
    data: state.closed,
    isLoading: false,
    isError: false,
    error: null,
  }),
}));
vi.mock("@/hooks/usePortfolioSummary", () => ({
  usePortfolioSummary: () => ({
    data: state.summary,
    isLoading: false,
    isError: false,
  }),
}));
vi.mock("@/components/portfolio/PortfolioSummaryCards", () => ({
  PortfolioSummaryCards: () => <div data-testid="summary-cards" />,
}));
vi.mock("@/components/portfolio/CashBalancesCard", () => ({
  CashBalancesCard: () => <div data-testid="cash" />,
}));
vi.mock("@/components/portfolio/AutomaticIncome", () => ({
  AutomaticIncome: () => <div />,
}));
vi.mock("@/components/portfolio/AutomaticAdjustments", () => ({
  AutomaticAdjustments: () => <div />,
}));
vi.mock("@/components/portfolio/PositionsTable", () => ({
  PositionsTable: ({ highlightedId }: { highlightedId?: string }) => (
    <div data-testid="positions-table" data-highlighted={highlightedId ?? ""} />
  ),
}));
vi.mock("@/components/portfolio/PositionCardList", () => ({
  PositionCardList: () => <div data-testid="position-cards" />,
}));

const { PortfolioStateTab } = await import(
  "@/components/portfolio/PortfolioStateTab"
);

function closedPosition(id: string, symbol: string): ClosedPosition {
  return {
    id,
    symbol,
    asset_type: "stock",
    quantity: 5,
    baseline_price: 100,
    baseline_currency: "USD",
    close_price: 120,
    close_price_currency: "USD",
    closed_at: "2026-05-01T00:00:00Z",
    realized_gain_loss_base: 100,
    realized_gain_loss_percentage: 20,
    base_currency: "USD",
  };
}

function renderTab(view: StateView, episodeId?: string) {
  const onViewChange = vi.fn();
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const utils = render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <PortfolioStateTab
          view={view}
          episodeId={episodeId}
          onViewChange={onViewChange}
          onSell={vi.fn()}
          onAddPosition={vi.fn()}
        />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return { ...utils, onViewChange };
}

beforeEach(() => {
  state.rows = [{ id: OPEN_EPISODE, symbol: "AAPL", asset_type: "stock" }];
  state.closed = [closedPosition(CLOSED_EPISODE, "TSLA")];
  state.summary = {
    base_currency: "USD",
    positions: [
      { symbol: "AAPL", asset_type: "stock", current_value_base: 7500 },
      { symbol: "MSFT", asset_type: "stock", current_value_base: 2500 },
    ],
  };
});

describe("episode deep-linking", () => {
  // An activity does not know whether its episode is still open, so the tab
  // must resolve the subview from the data instead of trusting the URL.
  it("switches to the closed view when the episode is a closed one", async () => {
    const { onViewChange } = renderTab("open", CLOSED_EPISODE);
    await waitFor(() => expect(onViewChange).toHaveBeenCalledWith("closed"));
  });

  it("stays on the open view when the episode is still open", async () => {
    const { onViewChange } = renderTab("open", OPEN_EPISODE);
    await waitFor(() =>
      expect(screen.getByTestId("positions-table").dataset.highlighted).toBe(
        OPEN_EPISODE,
      ),
    );
    expect(onViewChange).not.toHaveBeenCalled();
  });

  it("does not switch views for an episode it cannot find", async () => {
    const { onViewChange } = renderTab("open", "does-not-exist");
    await waitFor(() =>
      expect(
        screen.getByText(/no longer in this list/),
      ).toBeTruthy(),
    );
    expect(onViewChange).not.toHaveBeenCalled();
  });

  it("marks the linked closed episode with aria-current, not colour alone", () => {
    renderTab("closed", CLOSED_EPISODE);
    const card = document.getElementById(
      `portfolio-episode-${CLOSED_EPISODE}`,
    );
    expect(card).toBeTruthy();
    expect(card?.getAttribute("aria-current")).toBe("true");
  });

  it("says so when the linked episode is not among the closed ones", () => {
    renderTab("closed", "unknown-episode");
    expect(
      screen.getByText(/not in your closed positions/),
    ).toBeTruthy();
  });

  it("announces the open-episode spotlight as a live status", () => {
    renderTab("open", OPEN_EPISODE);
    const status = screen.getByRole("status");
    expect(status.textContent).toContain("AAPL");
  });
});

describe("allocation subview", () => {
  it("shows each holding's share of total holdings value", () => {
    renderTab("allocation");
    const list = screen.getByRole("list");
    const items = within(list).getAllByRole("listitem");

    // 7500/10000 and 2500/10000, sorted largest first.
    expect(items[0].textContent).toContain("AAPL");
    expect(items[0].textContent).toContain("75.0%");
    expect(items[1].textContent).toContain("MSFT");
    expect(items[1].textContent).toContain("25.0%");
  });

  it("states that cash is excluded rather than silently omitting it", () => {
    renderTab("allocation");
    expect(screen.getByText(/Cash is excluded/)).toBeTruthy();
  });

  it("shows a truthful empty state when there is nothing to allocate", () => {
    state.summary = { base_currency: "USD", positions: [] };
    renderTab("allocation");
    expect(screen.getByText("No allocation to show")).toBeTruthy();
  });
});

describe("accessibility", () => {
  it("labels the subview switcher", () => {
    renderTab("open");
    expect(screen.getByRole("group", { name: "Portfolio view" })).toBeTruthy();
  });

  it("offers every subview including allocation", () => {
    renderTab("open");
    const group = screen.getByRole("group", { name: "Portfolio view" });
    for (const label of [
      "Open positions",
      "Closed positions",
      "Cash",
      "Allocation",
    ]) {
      expect(within(group).getByRole("button", { name: label })).toBeTruthy();
    }
  });
});
