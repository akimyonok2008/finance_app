import { useEffect } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, act, fireEvent, within } from "@testing-library/react";
import {
  MemoryRouter,
  Navigate,
  Route,
  Routes,
  useLocation,
  useNavigate,
} from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { LEGACY_PORTFOLIO_REDIRECTS } from "@/routes/legacyRedirects";

// The three tabs own three INDEPENDENT data sources. They are stubbed here so
// the routing contract is tested without network access; each stub records how
// often it mounted so we can assert tab isolation.
const mountCounts = { transactions: 0, state: 0, performance: 0 };

vi.mock("@/components/portfolio/PortfolioTransactionsTab", () => ({
  PortfolioTransactionsTab: () => {
    mountCounts.transactions += 1;
    return <div data-testid="tab-transactions">transactions</div>;
  },
}));
vi.mock("@/components/portfolio/PortfolioStateTab", () => ({
  PortfolioStateTab: () => {
    mountCounts.state += 1;
    return <div data-testid="tab-state">state</div>;
  },
}));
vi.mock("@/components/portfolio/PortfolioPerformanceTab", () => ({
  PortfolioPerformanceTab: ({
    timeframe,
    onTimeframeChange,
  }: {
    timeframe: string;
    onTimeframeChange: (value: "3M") => void;
  }) => {
    mountCounts.performance += 1;
    return (
      <div data-testid="tab-performance">
        performance:{timeframe}
        <button type="button" onClick={() => onTimeframeChange("3M")}>
          choose 3M
        </button>
      </div>
    );
  },
}));
vi.mock("@/components/layout/AppNav", () => ({
  AppNav: () => <nav data-testid="app-nav" />,
}));
vi.mock("@/auth/useAuth", () => ({
  useAuth: () => ({
    user: { display_name: "Tester" },
    isAuthenticated: true,
    isBootstrapping: false,
  }),
}));

const { PortfolioPage } = await import("@/pages/PortfolioPage");

// A mutable holder (not a reassigned module binding) so the test can drive the
// in-memory router's history the way the browser back/forward buttons would.
const router: { go: (delta: number) => void } = { go: () => {} };

function LocationProbe() {
  const location = useLocation();
  const navigate = useNavigate();
  useEffect(() => {
    router.go = (delta) => navigate(delta);
  }, [navigate]);
  return (
    <div data-testid="location">{location.pathname + location.search}</div>
  );
}

function renderAt(path: string) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <LocationProbe />
        <Routes>
          <Route path="/portfolio" element={<PortfolioPage />} />
          {Object.entries(LEGACY_PORTFOLIO_REDIRECTS).map(([from, to]) => (
            <Route
              key={from}
              path={from}
              element={<Navigate to={to} replace />}
            />
          ))}
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  mountCounts.transactions = 0;
  mountCounts.state = 0;
  mountCounts.performance = 0;
});

describe("unified /portfolio tab routing", () => {
  it("defaults to the state tab with no tab param", () => {
    renderAt("/portfolio");
    expect(screen.getByTestId("tab-state")).toBeDefined();
    expect(screen.queryByTestId("tab-transactions")).toBeNull();
    expect(screen.queryByTestId("tab-performance")).toBeNull();
  });

  it("falls back to the state tab for an unknown tab param", () => {
    renderAt("/portfolio?tab=nonsense");
    expect(screen.getByTestId("tab-state")).toBeDefined();
  });

  it("renders the transactions tab for ?tab=transactions", () => {
    renderAt("/portfolio?tab=transactions");
    expect(screen.getByTestId("tab-transactions")).toBeDefined();
    expect(screen.queryByTestId("tab-state")).toBeNull();
  });

  it("renders the state tab for ?tab=state", () => {
    renderAt("/portfolio?tab=state");
    expect(screen.getByTestId("tab-state")).toBeDefined();
  });

  it("renders the performance tab for ?tab=performance", () => {
    renderAt("/portfolio?tab=performance");
    expect(screen.getByTestId("tab-performance")).toBeDefined();
    expect(screen.queryByTestId("tab-state")).toBeNull();
  });

  it("reads the selected timeframe from the URL and writes changes back", async () => {
    renderAt("/portfolio?tab=performance&timeframe=6M");
    expect(screen.getByTestId("tab-performance").textContent).toContain("performance:6M");

    await act(async () => {
      screen.getByRole("button", { name: "choose 3M" }).click();
    });
    expect(screen.getByTestId("location").textContent).toBe(
      "/portfolio?tab=performance&timeframe=3M",
    );
  });

  it("falls back safely for an invalid performance timeframe", () => {
    renderAt("/portfolio?tab=performance&timeframe=forever");
    expect(screen.getByTestId("tab-performance").textContent).toContain("performance:1M");
  });

  it("browser navigation restores the selected timeframe", async () => {
    renderAt("/portfolio?tab=performance&timeframe=6M");
    await act(async () => {
      screen.getByRole("button", { name: "choose 3M" }).click();
    });
    await act(async () => {
      router.go(-1);
    });
    expect(screen.getByTestId("tab-performance").textContent).toContain("performance:6M");
    await act(async () => {
      router.go(1);
    });
    expect(screen.getByTestId("tab-performance").textContent).toContain("performance:3M");
  });

  it("redirects /activity to the transactions tab", () => {
    renderAt("/activity");
    expect(screen.getByTestId("location").textContent).toBe(
      "/portfolio?tab=transactions",
    );
    expect(screen.getByTestId("tab-transactions")).toBeDefined();
  });

  it("redirects /performance to the performance tab", () => {
    renderAt("/performance");
    expect(screen.getByTestId("location").textContent).toBe(
      "/portfolio?tab=performance&timeframe=1M",
    );
    expect(screen.getByTestId("tab-performance")).toBeDefined();
  });

  it("keeps tab state in the URL so back/forward works", async () => {
    renderAt("/portfolio");
    expect(screen.getByTestId("location").textContent).toBe("/portfolio");

    await act(async () => {
      screen.getByRole("tab", { name: /Performance/ }).click();
    });
    expect(screen.getByTestId("location").textContent).toBe(
      "/portfolio?tab=performance&timeframe=1M",
    );
    expect(screen.getByTestId("tab-performance")).toBeDefined();

    await act(async () => {
      router.go(-1);
    });
    expect(screen.getByTestId("location").textContent).toBe("/portfolio");
    expect(screen.getByTestId("tab-state")).toBeDefined();

    await act(async () => {
      router.go(1);
    });
    expect(screen.getByTestId("location").textContent).toBe(
      "/portfolio?tab=performance&timeframe=1M",
    );
  });

  it("keeps the state subview in the URL independently of the tab", () => {
    renderAt("/portfolio?view=closed");
    expect(screen.getByTestId("tab-state")).toBeDefined();
    expect(screen.getByTestId("location").textContent).toBe(
      "/portfolio?view=closed",
    );
  });

  // Each tab owns its own fetching: switching tabs unmounts the previous tab
  // and mounts the next one; it never re-mounts (and so never wipes) a sibling.
  it("gives each tab independent data-fetching", async () => {
    renderAt("/portfolio?tab=transactions");
    expect(mountCounts.transactions).toBe(1);
    expect(mountCounts.state).toBe(0);
    expect(mountCounts.performance).toBe(0);

    await act(async () => {
      screen.getByRole("tab", { name: /Performance/ }).click();
    });
    expect(mountCounts.performance).toBe(1);
    // The transactions tab was not re-mounted or re-fetched by the switch.
    expect(mountCounts.transactions).toBe(1);
    expect(mountCounts.state).toBe(0);

    await act(async () => {
      screen.getByRole("tab", { name: /Transactions/ }).click();
    });
    expect(mountCounts.transactions).toBe(2);
    expect(mountCounts.performance).toBe(1);
    expect(mountCounts.state).toBe(0);
  });

  // The tab strip is a real ARIA tablist: selection state is exposed to
  // assistive technology and the strip is operable from the keyboard alone.
  describe("tab accessibility", () => {
    it("exposes a labelled tablist with aria-selected state", () => {
      renderAt("/portfolio?tab=performance");

      const tablist = screen.getByRole("tablist", {
        name: "Portfolio sections",
      });
      const tabs = within(tablist).getAllByRole("tab");
      expect(tabs).toHaveLength(3);

      const selected = tabs.filter(
        (tab) => tab.getAttribute("aria-selected") === "true",
      );
      expect(selected).toHaveLength(1);
      expect(selected[0].textContent).toContain("Performance");
    });

    it("uses a roving tabindex so only the selected tab is in the tab order", () => {
      renderAt("/portfolio");
      const tabs = within(
        screen.getByRole("tablist", { name: "Portfolio sections" }),
      ).getAllByRole("tab");

      const inOrder = tabs.filter((tab) => tab.getAttribute("tabindex") === "0");
      expect(inOrder).toHaveLength(1);
      expect(inOrder[0].getAttribute("aria-selected")).toBe("true");
    });

    it("moves between tabs with the arrow, Home and End keys", async () => {
      renderAt("/portfolio?tab=transactions");
      const tablist = screen.getByRole("tablist", {
        name: "Portfolio sections",
      });

      await act(async () => {
        fireEvent.keyDown(tablist, { key: "ArrowRight" });
      });
      expect(screen.getByTestId("location").textContent).toBe("/portfolio");

      await act(async () => {
        fireEvent.keyDown(tablist, { key: "End" });
      });
      expect(screen.getByTestId("location").textContent).toBe(
        "/portfolio?tab=performance&timeframe=1M",
      );

      await act(async () => {
        fireEvent.keyDown(tablist, { key: "Home" });
      });
      expect(screen.getByTestId("location").textContent).toBe(
        "/portfolio?tab=transactions",
      );
    });

    it("connects each tab to its panel", () => {
      renderAt("/portfolio?tab=performance");
      const panel = screen.getByRole("tabpanel");
      const tab = screen.getByRole("tab", { name: /Performance/ });

      expect(tab.getAttribute("aria-controls")).toBe(panel.getAttribute("id"));
      expect(panel.getAttribute("aria-labelledby")).toBe(tab.getAttribute("id"));
    });
  });
});
