/**
 * Old standalone paths that are now TABS of the single /portfolio product area.
 *
 * These are redirect-only: no page implementation lives behind them. Keeping the
 * map in one place is what lets the routing test assert the real behaviour that
 * App.tsx wires up.
 */
export const LEGACY_PORTFOLIO_REDIRECTS: Record<string, string> = {
  "/activity": "/portfolio?tab=transactions",
  "/performance": "/portfolio?tab=performance",
};
