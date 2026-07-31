import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";

import { ArenaEmptyState } from "@/pages/arena/ArenaEmptyState";
import { CompetitionCard } from "@/pages/arena/CompetitionCard";
import type { ArenaCompetitionCard } from "@/types/arena";

const competition: ArenaCompetitionCard = {
  id: "premium-edition",
  name: "Global Quality Challenge",
  description: "Build a resilient portfolio from globally listed quality companies.",
  category: "regional",
  status: "registration_open",
  startsAt: "2026-08-01T12:00:00Z",
  endsAt: "2026-08-31T12:00:00Z",
  scoringScope: "full_portfolio",
  participantCount: 1284,
  joined: false,
  isLegacy: false,
};

function renderCard(overrides: Partial<ArenaCompetitionCard> = {}) {
  return render(
    <MemoryRouter>
      <CompetitionCard competition={{ ...competition, ...overrides }} />
    </MemoryRouter>,
  );
}

describe("CompetitionCard", () => {
  it("presents the key competition hierarchy and join action in one accessible link", () => {
    renderCard();
    const link = screen.getByRole("link", { name: /join competition: global quality challenge/i });
    expect(link.getAttribute("href")).toBe("/arena/competitions/premium-edition");
    expect(screen.getByText("Registration open")).not.toBeNull();
    expect(screen.getByText("1,284 joined")).not.toBeNull();
    expect(screen.getByText("Eligibility checked on entry")).not.toBeNull();
    expect(screen.getByText("01 Aug 2026")).not.toBeNull();
    expect(screen.getByText("31 Aug 2026")).not.toBeNull();
  });

  it("uses results and joined standing actions without changing routes", () => {
    const { rerender } = renderCard({ status: "completed" });
    expect(screen.getByRole("link", { name: /see results/i })).not.toBeNull();

    rerender(
      <MemoryRouter>
        <CompetitionCard competition={{ ...competition, joined: true, entryStatus: "active" }} />
      </MemoryRouter>,
    );
    expect(screen.getByRole("link", { name: /view standing/i })).not.toBeNull();
    expect(screen.getByText("Joined")).not.toBeNull();
    expect(screen.getByText("active")).not.toBeNull();
  });
});

describe("ArenaEmptyState", () => {
  it("uses plain copy without decorative empty-state boxes or lightning icons", () => {
    const { container } = render(<ArenaEmptyState />);
    expect(screen.getByText("No competitions available")).not.toBeNull();
    expect(container.querySelector("svg")).toBeNull();
  });
});
