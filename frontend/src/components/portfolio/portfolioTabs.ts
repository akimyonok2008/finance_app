/**
 * URL-driven tab and subview identifiers for the unified Portfolio area.
 *
 * These live in their own module (not next to a component) so the tab
 * components stay pure component modules.
 */

/** `/portfolio?tab=…`. Omitted or unknown → "state". */
export const PORTFOLIO_TABS = ["transactions", "state", "performance"] as const;
export type PortfolioTab = (typeof PORTFOLIO_TABS)[number];

/** `/portfolio?view=…` within the state tab. Omitted or unknown → "open". */
export const STATE_VIEWS = ["open", "closed", "cash"] as const;
export type StateView = (typeof STATE_VIEWS)[number];

export const STATE_VIEW_LABELS: Record<StateView, string> = {
  open: "Open positions",
  closed: "Closed positions",
  cash: "Cash",
};
