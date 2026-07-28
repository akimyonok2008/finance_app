# Alarvest Frontend

Responsive React interface for the Alarvest portfolio strategy tracker. It uses
the Go API for authentication, portfolio state, rankings, achievements,
profiles, discovery, friendships, and messages.

The public experience is designed for percentage-based strategy comparison.
Stock/ETF symbols, asset types, allocation percentages, percentage returns, and
ranks are suitable for profiles, Explore, and leaderboard surfaces. Account
balances and all other absolute monetary information remain private.

## Portfolio activity UI

The private `/portfolio` page has two top-level tabs — **Transactions** ("What
happened") and **Portfolio** ("What you own") — instead of separate routes.
Wording deliberately says "Record" because the app neither places orders nor
moves money. Mutations wait for the backend transaction to commit and then
invalidate positions, summary, cash, activities, ranked standing, leaderboard,
achievements, and profile queries.

The Transactions tab records deposits, withdrawals, buys, and sales. Buying
shows a live cost preview (`AddPositionForm`, debounced `useQuotes` lookup +
`useCashBalances`) with price, total cost, available cash, and remaining cash
after the purchase — the submit button disables itself if cash would go
negative. There is a single **Record sale** action — the user never chooses
partial vs. full. `ClosePositionDialog` offers 25/50/75/Max quantity shortcuts
and calls `useSalePreview` (`POST /portfolio/sells/preview`) live as the
quantity changes, showing gross proceeds, fee, net proceeds, allocated basis,
and estimated realized P&L before the user confirms; when the sale would use
all remaining shares the preview says the position will close automatically.
The backend — not the client — determines whether the position stays open or
the episode closes.

The full transaction ledger is not rendered by default. A small **Archive**
toggle in the top-right corner of the Transactions tab reveals a searchable
history (category filter + optional symbol) — `useActivityList` only fetches
once the archive is opened, via its `enabled` parameter.

The Portfolio tab shows the summary cards, then **Open positions / Closed
positions / Cash** as a nested pill selector. Selling from a position row
switches to the Transactions tab with that position pre-selected
(`?tab=transactions&sell=<id>`).

**Investment income is not user-entered.** There are no dividend / ETF-
distribution / interest / reinvested-dividend forms — routine income is detected
and credited automatically by the backend pipeline. The read-only **Income**
section (`AutomaticIncome`, `useIncomeEvents` → `GET /portfolio/income-events`)
lists what was credited with gross / withholding / net shown distinctly and an
“Estimated” tag when the gross is a provider expectation; it is hidden when
empty. Users may only submit a constrained, account-specific *correction* to a
detected event (`useCorrectIncomeEvent` →
`POST /portfolio/income-events/{id}/correction`) — never arbitrary income. A
small **Record fee** panel (management/custody/other) remains, and can only
reduce cash and ranked return. All income details are owner-private and never
exposed publicly.

**Corporate actions are not user-entered.** There are no split/symbol-change/
merger/spin-off/delisting/write-off forms. Routine corporate actions are applied
automatically by the backend pipeline; the read-only **Automatic adjustments**
section lists what was applied or is processing (statuses: Applied automatically,
Processing, Awaiting confirmed market data, Corrected) in plain language, hidden
when empty. These flows reuse the same query invalidation as cash/position
mutations (now including `corporateActions`).

## Stack

- React 19, TypeScript 6, Vite 8
- React Router 7 and TanStack Query 5
- Tailwind CSS 3, Radix primitives, Vaul, Framer Motion
- Recharts, React Hook Form, Zod, Lucide, and Sonner

There is no frontend automated test suite or `test` script. Validation currently
consists of linting and a production build.

## Run

Start the Go API on port 8080, then:

```bash
npm install
npm run dev
```

Open `http://localhost:5173`.

Configuration:

```env
VITE_API_BASE_URL=http://localhost:8080
VITE_AUTH_LOGIN_PATH=/auth/login
VITE_AUTH_REGISTER_PATH=/auth/register
VITE_ENABLE_MOCK_AUTH=false
VITE_GOOGLE_AUTH_ENABLED=false
VITE_GOOGLE_CLIENT_ID=
```

Google requires the same web client ID on the backend. Mock auth is a local
demo fallback: it can generate a frontend-only token after an email-login
failure, and that token is not accepted by the real API.

The JWT and user snapshot are stored in `localStorage`. The app does not use a
refresh token. Any API `401` clears local auth and redirects to `/login`.
Password changes replace the stored JWT with the backend's newly versioned
token so the current browser continues while older sessions are revoked.

## Active routes

| Route | Access | Behavior |
| --- | --- | --- |
| `/` | Public | Landing page; authenticated users go to Dashboard |
| `/login`, `/register` | Public | Verified email/password and optional Google auth |
| `/verification-pending`, `/verify-email` | Public | Resend guidance and one-time email verification |
| `/forgot-password`, `/reset-password` | Public | Enumeration-safe recovery and one-time password reset |
| `/dashboard` | Protected | Compact portfolio summary and product navigation |
| `/portfolio` | Protected | Transactions, Portfolio (Open/Closed/Cash), and Performance tabs — see below |
| `/leaderboard` | Protected | Global/timeframe rankings and personal standing |
| `/achievements` | Protected | Benchmark badge overview and catalogues |
| `/profile` | Protected | Owner profile preview and settings |
| `/profiles/:handle` | Protected | Public percentage-based strategy profile |
| `/explore` | Protected | Strategies, Friends, and Messages workspace |

Compatibility redirects:

- `/arena` and `/sprint` → `/leaderboard` (Arena and Sprint are not currently
  product features; they only exist as legacy/unreachable code and would only
  be built out if a future decision is made to add them)
- `/profile/me` → `/profile`
- `/friends` → `/explore?tab=friends`
- `/messages` and `/messages/:conversationId` → the matching Explore Messages
  state
- `/activity` → `/portfolio?tab=transactions`,
  `/performance` → `/portfolio?tab=performance` (kept for old bookmarks; both
  are tabs on `/portfolio` now, and no page implementation lives behind them)
- unknown routes → `/dashboard`

## Screen behavior

### Dashboard

The Dashboard is a read-only hub. Its performance card shows the ranked index
and ranked return without a dollar amount. Separate compact metrics show current
portfolio value, open holdings cost basis, current holdings P&L, and ledger-based
total portfolio P&L. When ledger history is incomplete, total P&L is shown as
unavailable rather than `$0.00`. The graph renders real `GET /performance/history`
ranked-index points (1M window) with its own loading/error/empty state, distinct
from the summary card's loading/error state — a history-fetch failure never
blanks the rest of the card. Every card links back to `/portfolio`; the
Dashboard does not calculate financial metrics itself.

### Portfolio

`/portfolio` is the single Portfolio product area with three URL-driven tabs:
**Transactions** (`?tab=transactions`), **Portfolio** (default, `?tab=state`),
and **Performance** (`?tab=performance`). Tab state lives in the search params,
so links, bookmarks, and browser back/forward all work. `PortfolioPage` is only
the shell (header + tab nav + tab content); each tab is its own component under
`src/components/portfolio/` and owns its own data fetching, so switching tabs
never wipes a sibling's loaded data. Within the Portfolio tab:

- Open positions: Add accepts symbol, asset type, and quantity — the backend
  captures the current quote as a baseline, there is no buy-price input. Sell
  routes to the Transactions tab (see above). Desktop tables and mobile cards
  show owner-only position values and freshness metadata.
- Closed positions: lifecycle summaries (opened/closed dates, realized P&L)
  for episodes that have gone to zero quantity.
- Cash: per-currency balances and base-value weights.
- Allocation: each holding's share of total holdings value, derived from the
  position values `GET /portfolio/summary` already returns. Cash is excluded
  and labelled as such (it has its own subview).

A compact **Ranked Return** card sits above the subview switcher and links to
`/portfolio?tab=performance`.

**Episode deep-links.** Activities that carry a `position_episode_id` link to
`/portfolio?tab=state&episode=<id>`. Episode identity IS the `positions` row id
(migration 0018), so the tab resolves whether the episode is open or closed from
the data and switches subview itself — the link never has to guess. The target
card is focused and marked `aria-current`, with a text status announcing it.

Portfolio mutations invalidate summaries, rankings, profiles, Explore, and
achievement queries.

### Performance tab

Three overview cards — **Ranked Return** (selected timeframe), **Economic P&L**
(ledger reconciliation), and **Maximum Drawdown** — plus one chart with a
Return / Portfolio Value / Drawdown mode switch and `1W/1M/3M/6M/1Y/ALL`
timeframes, then four analytic sections:

- **Economic breakdown** — Realized P&L, Unrealized P&L, Net Income, Standalone
  Fees, and Total Economic P&L from `GET /performance/summary`'s
  `economic_breakdown`. This is the exact decomposition the backend's
  reconciliation verifies; nothing is summed in the browser. Each row deep-links
  to the activities that produced it (`?tab=transactions&category=trades|income|fees`).
  "Standalone" fees exclude trade fees already netted into cost basis and
  realized P&L, so they are never double-counted.
- **Risk & consistency** — Maximum Drawdown, Current Drawdown, Positive Weeks
  (complete calendar weeks only, current week excluded) and Best/Worst Month
  (complete calendar months only), all from `performancehistory`'s `risk` block.
  Insufficient history renders "Not enough history", never 0%.
- **Benchmark & competition** — Benchmark Difference against `SPY` and the
  caller's global rank/percentile. The difference is only shown when the backend
  confirms both returns were measured between the SAME boundary dates, which it
  discloses; otherwise the reason is shown. Rank comes from
  `GET /leaderboard/me` at the SAME timeframe as the chart.
- **Contributors & detractors** — top 3 each, ranked by contribution in
  percentage points (weight x instrument return), never by standalone return.
  Its basis is `since_inception`, and the UI says so whenever a shorter
  timeframe is selected, because no per-instrument daily valuation history
  exists to support period attribution.

Return and Drawdown come from `GET /performance/history`, the canonical ranked
snapshot history. Portfolio Value comes from `GET /portfolio/archives`, the
private valuation history — it includes deposits and withdrawals and is
therefore never used for return or drawdown. Every number is computed by the
backend; nothing is recalculated in the browser. Missing analytics render as
"—" with a reason ("Performance history will appear after the first trusted
snapshot."), never as zero.

**Accessibility.** The Portfolio tab strip is a real ARIA `tablist` with
`role="tab"`, `aria-selected`, a roving tabindex and Arrow/Home/End keyboard
navigation, each tab wired to its `tabpanel`. Chart mode and timeframe controls
are labelled groups; loading and error states are `role="status"` / `role="alert"`
live regions; and every gain/loss value carries an explicit `+`/`-` sign so
colour is never the only signal.

### Leaderboard

`1W`, `1M`, `3M`, `6M`, `1Y`, and `ALL` controls call the matching backend
timeframe. Window boards can be empty until enough ranked snapshots exist.
Rows display identity, rank, ranked index/return, and profile navigation.
Current cards keep composition in the linked profile to stay visually compact.
That is a presentation choice, not a privacy requirement: holding symbols and
percentage weights are acceptable leaderboard data because they reveal the
strategy without revealing the amount invested.

The personal standing card displays the backend values, but rank movement is
usually absent because historical rank delta is not currently recorded.

### Achievements

The dedicated screen presents twenty benchmark badges across Overview,
Legendary, Strategy, and All views. Badges have distinct icons and color
identities. Detail dialogs expose recipes, rules, evidence, and real evaluation
fields when the API supplies them.

`POST /achievements/evaluate` provides progress and can permanently award
badges. Progress is a backend presentation heuristic, not a probability. It is
based on the same canonical ranked-index snapshots as timeframe leaderboards.
The detail dialog shows history, active, and trusted-data coverage, effective
boundaries, current/required edge, snapshot cadence, and versioned evidence.
For unlocked badges it also surfaces benchmark data provenance: the award
verification (Verified / Demo / Legacy / Unverified), price methodology
(total-return vs. price-return benchmark), recipe version, data quality, and —
for the Berkshire 13F basket — the SEC filing accession and mapped-holdings
coverage. Investor-inspired badges note they are proxy portfolios, not replicas
of the investor's actual holdings or performance. Each benchmark integrity
failure has its own explanation — unadjusted/price-return data, stale data,
unverified/demo data, and unavailable recipe versions are distinguished, and a
portfolio that beats the benchmark while lacking verified total-return data is
told so explicitly. Synthetic mock benchmark data is preview- or demo-only and
never produces a verified award. Existing archive-model awards remain visible
with a subtle legacy-evidence marker.

### Profiles

The owner can edit display name, bio, strategy tag, and the current public
profile/public weights presentation controls. Public pages may display headline
ranked return/index, ranked snapshot history, ranks, unlocked badges, DNA,
exposure aggregates, concentration, symbols, weights, and closed-symbol
percentage returns. Symbols and percentages are public strategy information;
the UI must still keep absolute monetary fields private.

### Authentication and account lifecycle

Password registration routes to a verification-pending screen rather than
logging in. `/verify-email` consumes the link token, stores the returned
session, and opens the dashboard. Login recognizes
`email_verification_required` and returns the user to the pending screen.

Forgot-password always shows the same confirmation. `/reset-password` accepts
the one-time URL token, enforces the eight-character minimum and matching
confirmation, and explains that older sessions were revoked.

Account Settings uses `has_password` from the safe user projection:

- Password users confirm their current password. A successful change stores the
  replacement JWT returned by the API.
- Provider-only users never see a current-password prompt. They reauthenticate
  with the linked Google identity before creating their first password.
- Deletion first obtains a short-lived reauthentication token—by password or
  linked Google subject—then submits that token to the destructive endpoint.

Successful deletion clears local authentication and returns to sign-in. A 401
from any ordinary authenticated request also clears storage and redirects,
covering password reset, explicit revocation, deletion, or another browser
changing the password.

Headline performance and chart history now share persistent ranked semantics.
The benchmark section is currently a placeholder. Compare and Copy are
deterministic template tools and do not trade or copy monetary amounts.

### Explore, Friends, and Messages

Explore is the active combined workspace:

- Strategies uses a larger left column for Featured and Similar and a smaller
  right column for trending public holdings.
- Search results are independent of Featured and Similar. Text search matches
  public people/handles and visible symbols; the symbol filter targets a
  holding exactly.
- The old Top Rated/Top Performers Explore block is not present.
- Friends shows mutual, following, and follower lists with profile/message
  actions.
- Messages lists one-to-one mutual-follower conversations, polls every 30
  seconds, and uses distinct sent/received states.

The UI has no WebSockets, unread counts, notifications, attachments, reactions,
blocking, moderation, or group chat.

## Privacy and API integration

Alarvest protects monetary amounts, not strategy visibility. Owner routes may
display quantities, transaction prices, balances, values, cost basis, and
absolute gain/loss. Public UI must never display those fields.

Public strategy and leaderboard surfaces may display:

- display name, handle, avatar, and strategy tag
- ranked percentage return/index, rank, achievements, and ranked history
- DNA, exposure, concentration, and performance-driver aggregates
- stock/ETF symbols, asset types, and percentage weights
- closed symbols and percentage returns

Showing a symbol or percentage does not disclose the user's account size.
Quantities, balances, monetary values, buy/sell prices, cost basis, absolute
gain/loss, email, and brokerage identifiers remain private. Current leaderboard
rows link to profile composition rather than rendering it inline, and Explore
currently includes profiles with both public visibility and public weights
enabled; these are product-presentation controls, not restrictions required to
protect monetary privacy.

Symbol-level composition requires both a public profile and the separate
`show_public_weights` opt-in. Enabling it also permits reuse in Compare, Copy,
similar-strategy matching, trending holdings, and leaderboard enrichment.
Disabling it stops subsequent product responses, but cannot recall information
another user already saw or copied. Competitive rankings may still show
display name, avatar, rank, ranked index, and percentage return for an eligible
participant whose profile is private. See [`../PRIVACY.md`](../PRIVACY.md) for
the authoritative visibility matrix.

React components only format backend-owned financial metrics. They do not
multiply ranked return by basis, reconstruct total P&L, or add realized results,
income, or fees to current value. Positive/negative colors are selected
independently for ranked return, holdings P&L, and total portfolio P&L.

## Server-state conventions

TanStack Query keys are centralized in `src/hooks/queryKeys.ts`. The shared API
client attaches the bearer token and owns global `401` handling. Domain hooks
invalidate related portfolio, leaderboard, profile, Explore, social, message,
and achievement data after mutations.

The frontend never calls Twelve Data directly and contains no market-data
provider secret.

## Current, legacy, and not implemented

- **Current:** all routes listed above, responsive portfolio workflows,
  persistent-ranked leaderboard presentation, benchmark achievements, public
  profile discovery, friendships, and messages.
- **Legacy/unreachable:** Arena/sprint components and APIs remain in the source
  tree, but active routes redirect to Leaderboard. Arena and Sprint are not
  currently product features — they would only be built out and shipped if a
  future decision is made to add them.
- **Removed:** Coach API, hooks, types, components, and portfolio analysis modes.
- **Not implemented:** Apple sign-in UI, live competition UI, broker trading,
  AI advice, refresh tokens, device-management UI, MFA/passkeys, and production
  social features.

## Build checks

```bash
npm install
npm run lint
npm run build
npm test
```

`npm test` runs Vitest (jsdom + Testing Library). `src/pages/PortfolioPage.test.tsx`
covers the `/portfolio` tab routing contract, the `/activity` and `/performance`
compatibility redirects, browser back/forward, and per-tab data-fetching
independence.
