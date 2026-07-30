import * as Sentry from "@sentry/react";

// Empty VITE_SENTRY_DSN disables reporting entirely — no default/shared DSN.
// init() no-ops safely when called with an empty dsn, so callers never need
// to branch on whether this ran.
export function initSentry() {
  const dsn = import.meta.env.VITE_SENTRY_DSN;
  if (!dsn) return;
  Sentry.init({
    dsn,
    environment: import.meta.env.MODE,
  });
}
