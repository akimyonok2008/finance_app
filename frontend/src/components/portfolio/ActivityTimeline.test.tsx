import { fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";

import {
  ActivityTimeline,
} from "@/components/portfolio/ActivityTimeline";
import { groupActivities } from "@/components/portfolio/activityGrouping";
import type { PortfolioActivity } from "@/types/portfolio";

function activity(
  id: string,
  type: PortfolioActivity["activity_type"],
  overrides: Partial<PortfolioActivity> = {},
): PortfolioActivity {
  return {
    id,
    activity_type: type,
    currency: "USD",
    gross_amount: 0,
    occurred_at: "2026-07-20T12:00:00Z",
    portfolio_version: 1,
    origin: "user_recorded",
    status: "completed",
    ...overrides,
  };
}

function renderTimeline(items: PortfolioActivity[]) {
  return render(
    <MemoryRouter>
      <ActivityTimeline activities={items} />
    </MemoryRouter>,
  );
}

describe("ActivityTimeline", () => {
  it("groups an automatically funded buy and explains cash, funding, and fee", () => {
    renderTimeline([
      activity("fund", "deposit", { group_id: "buy-1", gross_amount: 1305, origin: "system_generated" }),
      activity("buy", "buy", { group_id: "buy-1", symbol: "AAPL", quantity: 10, unit_price: 180.25, gross_amount: 1802.5, position_episode_id: "episode-1" }),
      activity("fee", "buy_fee", { group_id: "buy-1", gross_amount: 2.5 }),
    ]);

    expect(screen.getByText(/Bought 10 AAPL at/)).toBeTruthy();
    expect(screen.getByText(/Used existing cash/).textContent).toContain("$500.00");
    expect(screen.getByText(/Automatically funded/).textContent).toContain("$1,305.00");
    expect(screen.getByText(/Transaction fee/).textContent).toContain("-$2.50");
    expect(screen.getAllByTestId("activity-timeline")).toHaveLength(1);
  });

  it("groups a sale with neutral proceeds, signed realized P&L, and a negative fee", () => {
    renderTimeline([
      activity("sell", "sell", {
        group_id: "sale-1", symbol: "AAPL", quantity: 5, unit_price: 210,
        gross_amount: 1050, net_amount: 1048.5, realized_gain_loss_base: 543.5,
      }),
      activity("fee", "sell_fee", { group_id: "sale-1", gross_amount: 1.5 }),
    ]);

    expect(screen.getByText(/Sold 5 AAPL at/)).toBeTruthy();
    expect(screen.getByText(/Net proceeds/).getAttribute("data-amount-tone")).toBe("neutral");
    expect(screen.getByText(/Realized P&L/).getAttribute("data-amount-tone")).toBe("positive");
    expect(screen.getByText(/Transaction fee/).getAttribute("data-amount-tone")).toBe("negative");
  });

  it("groups reinvested dividend income and shares added", () => {
    renderTimeline([
      activity("income", "reinvested_dividend", {
        group_id: "income-1", symbol: "AAPL", gross_amount: 42.5,
        origin: "provider_generated",
      }),
      activity("buy", "buy", {
        group_id: "income-1", symbol: "AAPL", quantity: 0.21, gross_amount: 42.5,
        origin: "system_generated",
      }),
    ]);

    expect(screen.getByText("AAPL dividend reinvested")).toBeTruthy();
    expect(screen.getByText(/Income/).textContent).toContain("$42.50");
    expect(screen.getByText(/Shares added/).textContent).toContain("0.21");
  });

  it("keeps ungrouped legacy rows independent even with the same symbol and timestamp", () => {
    const rows = [
      activity("legacy-1", "deposit", { symbol: "AAPL", gross_amount: 100 }),
      activity("legacy-2", "deposit", { symbol: "AAPL", gross_amount: 100 }),
    ];
    expect(groupActivities(rows)).toHaveLength(2);
    renderTimeline(rows);
    expect(screen.getAllByText("Deposit")).toHaveLength(2);
  });

  it("uses positive, negative, and neutral amount semantics", () => {
    renderTimeline([
      activity("deposit", "deposit", { gross_amount: 100 }),
      activity("withdraw", "withdrawal", { gross_amount: 20 }),
      activity("split", "stock_split", { gross_amount: 0 }),
    ]);
    expect(screen.getByText(/Cash flow \$100/).getAttribute("data-amount-tone")).toBe("positive");
    expect(screen.getByText(/Cash flow -\$20/).getAttribute("data-amount-tone")).toBe("negative");
    expect(screen.getByText(/Amount \$0/).getAttribute("data-amount-tone")).toBe("neutral");
  });

  it("expands immutable legs and preserves the episode deep link", () => {
    renderTimeline([
      activity("buy", "buy", {
        group_id: "g", symbol: "A&B", quantity: 1, unit_price: 10,
        gross_amount: 10, position_episode_id: "episode/one",
      }),
      activity("fee", "buy_fee", { group_id: "g", gross_amount: 1 }),
    ]);
    const toggle = screen.getByRole("button", { name: "Show 2 activity legs" });
    expect(toggle.getAttribute("aria-expanded")).toBe("false");
    toggle.focus();
    expect(document.activeElement).toBe(toggle);
    fireEvent.click(toggle);
    expect(toggle.getAttribute("aria-expanded")).toBe("true");
    expect(within(screen.getByRole("list", { name: "Immutable activity legs" })).getByText("buy fee")).toBeTruthy();

    const link = screen.getByRole("link", { name: /View the A&B position episode/ });
    expect(link.getAttribute("href")).toBe("/portfolio?tab=state&episode=episode%2Fone");
  });

  it("uses a single-column mobile layout with a responsive desktop enhancement", () => {
    renderTimeline([activity("legacy", "deposit", { gross_amount: 100 })]);
    const card = screen.getByText("Deposit").closest("[class*='grid-cols-1']");
    expect(card).toBeTruthy();
    expect(card?.className).toContain("sm:grid-cols-[1fr_auto]");
  });
});
