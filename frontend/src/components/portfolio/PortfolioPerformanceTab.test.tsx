import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type {
  BenchmarkComparison,
  ContributionAnalysis,
  EconomicBreakdown,
  PerformanceSummary,
  RankedPerformanceHistory,
  RiskConsistency,
} from "@/types/performance";

// Recharts needs layout it cannot get in jsdom; the chart itself is not what
// these tests are about.
vi.mock("recharts", async () => {
  const passthrough =
    (name: string) =>
    ({ children }: { children?: React.ReactNode }) => (
      <div data-testid={`chart-${name}`}>{children}</div>
    );
  return {
    Area: passthrough("area"),
    AreaChart: passthrough("areachart"),
    CartesianGrid: passthrough("grid"),
    ResponsiveContainer: passthrough("container"),
    Tooltip: passthrough("tooltip"),
    XAxis: passthrough("xaxis"),
    YAxis: passthrough("yaxis"),
  };
});

const state = {
  history: undefined as RankedPerformanceHistory | undefined,
  performance: undefined as PerformanceSummary | undefined,
  standing: undefined as
    | { rank: number | null; percentile: number; participant_count: number; reason: string }
    | undefined,
};

vi.mock("@/hooks/usePerformance", () => ({
  usePerformanceHistory: () => ({
    data: state.history,
    isLoading: false,
    isError: false,
  }),
  usePerformanceSummary: () => ({
    data: state.performance,
    isLoading: false,
    isError: false,
  }),
  usePortfolioValueHistory: () => ({
    data: undefined,
    isLoading: false,
    isError: false,
  }),
}));
vi.mock("@/hooks/usePortfolioSummary", () => ({
  usePortfolioSummary: () => ({ data: { base_currency: "USD" } }),
}));
vi.mock("@/hooks/useLeaderboardStanding", () => ({
  useLeaderboardStanding: () => ({ data: state.standing, isLoading: false }),
}));

const { PortfolioPerformanceTab } = await import(
  "@/components/portfolio/PortfolioPerformanceTab"
);

const emptyRisk: RiskConsistency = {
  max_drawdown_percentage: null,
  current_drawdown_percentage: null,
  positive_weeks_percentage: null,
  positive_weeks: 0,
  complete_weeks: 0,
  best_month: null,
  worst_month: null,
  complete_months: 0,
  weeks_reason: "Not enough history: fewer than one complete calendar week.",
  months_reason: "Not enough history: fewer than one complete calendar month.",
  drawdown_reason: "Performance history will appear after the first trusted snapshot.",
  calculation_base: "ranked_index",
};

const unavailableBenchmark: BenchmarkComparison = {
  available: false,
  reason: "Benchmark comparison is unavailable: no benchmark price source is configured.",
};

function historyWith(
  overrides: Partial<RankedPerformanceHistory> = {},
): RankedPerformanceHistory {
  return {
    timeframe: "1M",
    from: "2026-06-26T00:00:00Z",
    to: "2026-07-26T00:00:00Z",
    available: false,
    points: [],
    risk: emptyRisk,
    benchmark: unavailableBenchmark,
    ...overrides,
  };
}

const completeBreakdown: EconomicBreakdown = {
  realized_pnl_base: 150,
  unrealized_pnl_base: 400,
  net_income_base: 60,
  standalone_fees_base: 20,
  attributed_total_base: 590,
  total_economic_pnl_base: 590,
  unattributed_base: 0,
  calculation_status: "complete",
  is_complete: true,
};

const contributions: ContributionAnalysis = {
  basis: "since_inception",
  calculation_status: "complete",
  available: true,
  total_capital_base: 10000,
  contributors: [
    {
      symbol: "BIG",
      weight_percentage: 90,
      instrument_return_percentage: 10,
      contribution_percentage_points: 9,
      economic_result_base: 900,
      income_base: 0,
    },
    {
      symbol: "MOON",
      weight_percentage: 1,
      instrument_return_percentage: 100,
      contribution_percentage_points: 1,
      economic_result_base: 100,
      income_base: 0,
    },
  ],
  detractors: [
    {
      symbol: "BAD",
      weight_percentage: 9,
      instrument_return_percentage: -20,
      contribution_percentage_points: -1.8,
      economic_result_base: -180,
      income_base: 0,
    },
  ],
  unattributed_percentage_points: 0,
};

function summaryWith(
  overrides: Partial<PerformanceSummary> = {},
): PerformanceSummary {
  return {
    base_currency: "USD",
    ranked: { index: 110, return_percentage: 10, tracking_status: "active" },
    economic_breakdown: completeBreakdown,
    contributions,
    reconciliation: { is_complete: true, is_consistent: true, difference: 0 },
    ...overrides,
  };
}

function renderTab() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <PortfolioPerformanceTab />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  state.history = historyWith();
  state.performance = summaryWith();
  state.standing = undefined;
});

describe("economic breakdown", () => {
  it("links each row to the transactions view that produced it", () => {
    renderTab();
    const section = screen.getByRole("region", { name: "Economic breakdown" });

    const href = (name: RegExp) =>
      within(section).getByRole("link", { name }).getAttribute("href");

    expect(href(/Realized P&L/)).toBe(
      "/portfolio?tab=transactions&category=trades",
    );
    expect(href(/Net Income/)).toBe(
      "/portfolio?tab=transactions&category=income",
    );
    expect(href(/Standalone Fees/)).toBe(
      "/portfolio?tab=transactions&category=fees",
    );
    expect(href(/Unrealized P&L/)).toBe("/portfolio?tab=state&view=open");
  });

  it("shows every value with an explicit sign, never colour alone", () => {
    renderTab();
    const section = screen.getByRole("region", { name: "Economic breakdown" });

    expect(within(section).getByText("+$150.00")).toBeTruthy();
    expect(within(section).getByText("+$400.00")).toBeTruthy();
    // Standalone fees reduce the result, so they are rendered negative.
    expect(within(section).getByText("-$20.00")).toBeTruthy();
    expect(within(section).getByText("+$590.00")).toBeTruthy();
  });

  it("shows an em dash and an explanation, not 0, when the ledger is incomplete", () => {
    state.performance = summaryWith({
      economic_breakdown: {
        ...completeBreakdown,
        total_economic_pnl_base: null,
        unattributed_base: null,
        calculation_status: "legacy_estimate",
        is_complete: false,
      },
    });
    renderTab();
    const section = screen.getByRole("region", { name: "Economic breakdown" });

    expect(within(section).queryByText("+$590.00")).toBeNull();
    expect(
      within(section).getByText(/does not cover your full holding history/),
    ).toBeTruthy();
    expect(within(section).getByText(/legacy estimate/)).toBeTruthy();
  });
});

describe("risk & consistency", () => {
  it("says 'not enough history' instead of reporting zeros", () => {
    renderTab();
    const section = screen.getByRole("region", { name: "Risk & consistency" });

    expect(within(section).getAllByText("—").length).toBeGreaterThanOrEqual(4);
    expect(within(section).queryByText("0.00%")).toBeNull();
    expect(
      within(section).getAllByText(/Not enough history/).length,
    ).toBeGreaterThanOrEqual(2);
  });

  it("renders the backend's risk figures without recomputing them", () => {
    state.history = historyWith({
      available: true,
      risk: {
        max_drawdown_percentage: -25,
        current_drawdown_percentage: -10,
        positive_weeks_percentage: 60,
        positive_weeks: 3,
        complete_weeks: 5,
        best_month: { label: "2026-01", return_percentage: 12 },
        worst_month: { label: "2026-02", return_percentage: -8 },
        complete_months: 3,
        calculation_base: "ranked_index",
      },
    });
    renderTab();
    const section = screen.getByRole("region", { name: "Risk & consistency" });

    expect(within(section).getByText("-25.00%")).toBeTruthy();
    expect(within(section).getByText("-10.00%")).toBeTruthy();
    expect(within(section).getByText("60%")).toBeTruthy();
    expect(within(section).getByText("3 of 5 complete weeks")).toBeTruthy();
    expect(within(section).getByText("+12.00% / -8.00%")).toBeTruthy();
  });
});

describe("benchmark & competition", () => {
  it("shows the reason, not a 0-point difference, when alignment is impossible", () => {
    renderTab();
    const section = screen.getByRole("region", {
      name: "Benchmark & competition",
    });

    expect(within(section).queryByText("0.00%")).toBeNull();
    expect(
      within(section).getByText(/no benchmark price source is configured/),
    ).toBeTruthy();
  });

  it("discloses the aligned boundary dates alongside the difference", () => {
    state.history = historyWith({
      available: true,
      benchmark: {
        available: true,
        recipe_id: "SPY",
        name: "S&P 500",
        aligned_from: "2026-06-26",
        aligned_to: "2026-07-24",
        benchmark_return_percentage: 4,
        portfolio_return_percentage: 10,
        difference_percentage_points: 6,
        data_quality: "verified",
      },
    });
    state.standing = {
      rank: 12,
      percentile: 88,
      participant_count: 100,
      reason: "",
    };
    renderTab();
    const section = screen.getByRole("region", {
      name: "Benchmark & competition",
    });

    expect(within(section).getByText("+6.00%")).toBeTruthy();
    expect(
      within(section).getByText(/2026-06-26 → 2026-07-24/),
    ).toBeTruthy();
    expect(within(section).getByText(/S&P 500/)).toBeTruthy();
    expect(within(section).getByText(/#12/)).toBeTruthy();
    expect(within(section).getByText(/Top 12%/)).toBeTruthy();
  });

  it("explains an absent rank rather than showing rank 0", () => {
    state.standing = {
      rank: null,
      percentile: 0,
      participant_count: 0,
      reason: "Not enough ranked history for this timeframe yet.",
    };
    renderTab();
    const section = screen.getByRole("region", {
      name: "Benchmark & competition",
    });

    expect(within(section).queryByText("#0")).toBeNull();
    expect(
      within(section).getByText("Not enough ranked history for this timeframe yet."),
    ).toBeTruthy();
  });
});

describe("contributors & detractors", () => {
  it("orders by contribution in percentage points, not standalone return", () => {
    renderTab();
    const section = screen.getByRole("region", {
      name: "Contributors & detractors",
    });

    const links = within(section).getAllByRole("link");
    // BIG (+9.00 points) outranks MOON (+100% standalone return, +1.00 point).
    expect(links[0].textContent).toContain("BIG");
    expect(links[0].textContent).toContain("+9.00%");
    expect(links[1].textContent).toContain("MOON");
    expect(links[1].textContent).toContain("+1.00%");
    expect(within(section).getByText("-1.80%")).toBeTruthy();
  });

  it("warns that the basis is since-inception when a shorter timeframe is selected", () => {
    renderTab();
    // The tab defaults to 1M, so the since-inception caveat must be visible.
    expect(
      screen.getByText(/cover your whole history, not the selected 1M window/),
    ).toBeTruthy();
  });

  it("shows the backend's reason when contribution analysis is unavailable", () => {
    state.performance = summaryWith({
      contributions: {
        ...contributions,
        available: false,
        calculation_status: "incomplete",
        contributors: [],
        detractors: [],
        reason:
          "Contribution analysis is unavailable: no committed capital has been recorded yet.",
      },
    });
    renderTab();
    const section = screen.getByRole("region", {
      name: "Contributors & detractors",
    });

    expect(
      within(section).getByText(/no committed capital has been recorded/),
    ).toBeTruthy();
  });

  it("discloses an unattributed remainder instead of assigning it to an instrument", () => {
    state.performance = summaryWith({
      contributions: { ...contributions, unattributed_percentage_points: -0.25 },
    });
    renderTab();

    expect(
      screen.getByText(/belongs to no single instrument/),
    ).toBeTruthy();
    expect(screen.getByText(/-0\.25%/)).toBeTruthy();
  });
});

describe("accessibility", () => {
  it("labels the chart mode and timeframe control groups", () => {
    renderTab();
    expect(screen.getByRole("group", { name: "Chart mode" })).toBeTruthy();
    expect(screen.getByRole("group", { name: "Timeframe" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Timeframe 1W" })).toBeTruthy();
  });

  it("exposes each analytic section as a labelled region", () => {
    renderTab();
    for (const name of [
      "Economic breakdown",
      "Risk & consistency",
      "Benchmark & competition",
      "Contributors & detractors",
    ]) {
      expect(screen.getByRole("region", { name })).toBeTruthy();
    }
  });
});
