# Alarvest

Alarvest is a full-stack portfolio strategy tracker. Users manually enter
holdings, follow percentage performance, compare privacy-filtered public
strategies, earn benchmark badges, and connect with other users. It is a
tracking and education product: it does not connect to a broker or place
trades.

## CI status

[![CI](https://github.com/akimyonok2008/finance_app/actions/workflows/ci.yml/badge.svg)](https://github.com/akimyonok2008/finance_app/actions/workflows/ci.yml)

Pushes and pull requests run formatting, backend tests (including PostgreSQL
integration and race checks), vet, frontend lint, production build, and frontend
tests. CI uses mock providers and never contacts live financial-data services.

## Cash-funded portfolio tracking

Portfolio activity is user-reported and tracking-only. The app does not place
brokerage orders or initiate bank transfers.

- Deposits and withdrawals explicitly add or remove USD, TRY, EUR, or GBP cash.
- New buys require sufficient cash in the security's quote currency.
- Repeated buys merge into one open position using weighted-average cost basis.
- Partial and full sales use the same sale path; proceeds settle immediately to
  quote-currency cash.
- Deposits, withdrawals, buys, sales, and strategy reallocations are neutral to
  the chain-linked ranked index when recorded.
- Cash is tracked value. A cash-only portfolio remains active; only zero
  securities plus zero cash pauses.
- Positions whose history predates the ledger keep valid holdings valuation and
  ranked performance, while total portfolio P&L is marked unavailable rather
  than fabricating deposits.

Investment income is **detected and applied automatically** — users never enter
ordinary dividends or distributions, and there is no generic income-creation
endpoint (an unrestricted one could fabricate ranked returns). The only income
endpoints are read-only (`GET /portfolio/income-events`,
`GET /portfolio/income-events/{id}`) plus a constrained, account-specific
correction (`POST /portfolio/income-events/{id}/correction`). Fees remain
user-reported (`/portfolio/fees`); corporate actions are read-only
(`/portfolio/corporate-actions`).

- **Dividends, special dividends, ETF/fund distributions, bond coupons,
  capital-gains distributions, interest, staking** add cash and **raise** the
  ranked index once (a genuine return, not neutralized). A **reinvested
  dividend** is income plus a neutral buy, counted once. Gross, withholding, and
  net are stored **distinctly**; income stays in its declared (payment) currency.
- **Return of capital** is *not* ordinary income: it adds cash **and reduces the
  position's cost basis** (never below zero); the excess over basis is recorded
  separately. It is portfolio tracking, not tax advice.
- **Stock dividends** are quantity transformations: quantity increases, per-share
  basis decreases, total basis and the ranked index are preserved (no artificial
  jump) — no cash is credited.
- **Bond coupons** can be provider-driven; **cash interest** requires brokerage
  data or a configured account rule (never fabricated from a risk-free rate); and
  **staking rewards** require an account/wallet source (never inferred from
  merely holding a crypto asset).
- **Fees** (management/custody/other) reduce cash and **lower** the ranked index;
  cash can never go negative.
- **Splits / reverse splits / symbol changes** are neutral transformations —
  total basis and ranked index preserved. A **write-off** zeroes a position and
  records a genuine loss.
- The user portfolio is valued from live market quotes + explicit recorded cash
  income, so dividends are never double-counted through adjusted prices (adjusted
  series are benchmark-only).

**Corporate actions are applied automatically** — users never enter splits,
ticker changes, mergers, or spin-offs. A background pipeline ingests provider
events, matches them to stable instruments, and transforms affected portfolios
through the atomic coordinator; users see only a read-only "Automatic
adjustments" list with plain-language results. Splits update quantity **and**
basis automatically (value and ranked index preserved); symbol changes preserve
history; complete merger/spin-off terms may be processed automatically while
incomplete or conflicting events stay pending until reliable data arrives.
Delisting does **not** automatically mean zero value, and write-off is not a
normal user feature (it can be abused to erase losers). Corporate actions remain
private and are tracking adjustments, not brokerage executions.

Current limitations: user-reported income/fees and tracking only, immediate
settlement, no tax reporting/advice, no tax lots, margin, short selling,
options/futures, or brokerage/bank integration. Automatic merger, spin-off, and
delisting-resolution application is gated on complete authoritative provider data
(free-tier providers may delay complex events); account-specific cash-in-lieu and
tax-basis allocation are out of scope.

## Three-layer model

The product is organized around three explicit, separately-owned layers. They
stay separate services and DTOs on the backend, but the user sees ONE Portfolio
product area with three URL-driven tabs on the single `/portfolio` screen:

```text
Activity     "What happened?"      — the immutable ledger (Transactions tab)
Portfolio    "What do I own now?"  — materialized current state (Portfolio tab:
                                      Open positions / Closed positions / Cash)
Performance  "How did it perform?" — canonical ranked history (Performance tab)
```

Activity is the source of truth; Portfolio state is derived from it. Users
cannot bypass Activity to edit quantities, basis, or cash directly. The
Transactions tab records deposits, withdrawals, buys, and sales; a small
"Archive" toggle in its top-right corner reveals a searchable (category +
symbol) history of everything recorded — the full ledger is never rendered
by default. Recording a buy shows a live cost preview (price, total cost,
available cash, remaining cash) before submission, mirroring the sale
preview described below. The Dashboard only summarizes these layers — it
does not calculate financial metrics of its own — and every summary card
links back to `/portfolio`.

Selling is a single action ("Record sale"): the user never chooses partial vs.
full, and the backend determines automatically whether the position remains
open or the episode closes. A `POST /portfolio/sells/preview` call shows
gross/net proceeds, allocated basis, and estimated realized P&L (with quantity
shortcuts of 25/50/75/Max) before the user confirms; the preview never mutates
state and the final `POST /portfolio/sells` call re-validates everything under
the portfolio lock. A closed position's episode identity is the id of its
`positions` row, which is never reused: a later rebuy of the same symbol
always creates a new open position/episode, leaving the closed one's realized
totals untouched.

Corrections begin from the Activity layer, not direct state edits. User-
recorded deposits and withdrawals can be reconciled to their actual amount via
`POST /portfolio/activities/{id}/correction`, which posts a compensating
deposit/withdrawal rather than editing the original (immutable) record.
Buy/sell corrections are intentionally rejected — reconstructing everything
that may have happened to a position afterward (partial sells, closures,
rebuys) is out of scope for this pass; record an offsetting buy/sell instead.
Automatic/system-generated activities (income, corporate actions) keep their
own dedicated, more constrained correction paths.

## Current product

- Email/password authentication and optional Google Identity Services login
- One owner-only portfolio per user, with active positions, quantity edits,
  close history, archive snapshots, and base-currency summaries
- Persistent ranked-performance state designed to neutralize the effect of
  adding, resizing, closing, deleting, or copying positions
- Global `ALL` and canonical ranked-snapshot-backed `1W`, `1M`, `3M`, `6M`,
  and `1Y` leaderboards
- Twenty permanent benchmark achievements and a seven-dimension Portfolio DNA
  profile
- Opt-in public profiles, public allocation weights, Explore discovery,
  following, mutual-follower friendships, and one-to-one messages
- Compare and copy-weight templates that never execute trades
- In-memory or PostgreSQL storage, optional Redis leaderboard caching, mock
  prices by default, and optional Twelve Data quotes
- Responsive React interface for Dashboard, Portfolio, Leaderboard,
  Achievements, Explore, Friends/Messages, and Profiles

## Architecture

```text
frontend/  React 19, TypeScript, Vite, TanStack Query, Tailwind
backend/   Go 1.26 REST API, chi, JWT, pgx, optional Redis
```

The browser calls only the Go API. Provider keys remain on the backend.
PostgreSQL migrations run automatically when PostgreSQL storage is selected.

## Financial metric contract

Ranked performance and accounting P&L intentionally measure different things:

```text
ranked return             = ranked index - 100
current portfolio value   = open holdings market value + cash
current holdings P&L      = open holdings market value - remaining open basis
realized P&L              = net sale proceeds - allocated weighted-average basis
total portfolio P&L       = current value + withdrawals - deposits - explicit opening value
```

Income and fees are attribution metrics. Their cash effects are already inside
current portfolio value and are never added or subtracted again. Ranked return
is chain-linked and cash-flow-neutral, so it has no valid dollar equivalent.
For example, a ranked return of `+0.90%` and current holdings return of `-1.78%`
can both be correct after earlier profits were realized and the remaining
holdings declined. Owner-only APIs expose accounting values; public APIs expose
ranked percentages and never private dollar amounts.

## Run locally

Zero-infrastructure development uses in-memory storage and deterministic mock
market/FX data:

```bash
cd backend
go run ./cmd/api
```

```bash
cd frontend
npm install
npm run dev
```

Open `http://localhost:5173`. The API listens on `http://localhost:8080`.

To run the complete local stack:

```bash
docker compose up --build
```

Copy `backend/.env.example` and `frontend/.env.example` to their local `.env`
files when changing defaults. Durable development normally uses:

```env
STORAGE_PROVIDER=postgres
DATABASE_URL=postgres://postgres:postgres@localhost:5432/finance_app?sslmode=disable
REDIS_URL=redis://localhost:6379/0
```

Real Twelve Data quotes are opt-in:

```env
ENABLE_REAL_MARKET_DATA=true
PRICE_PROVIDER=twelvedata
TWELVE_DATA_API_KEY=replace-me
```

## Privacy boundary

Dashboard and Portfolio are owner-only and may contain quantities, prices,
values, cost basis, and absolute gain/loss. Public surfaces use percentage
performance and opt-in profile data. Public allocation views may expose symbols,
asset types, percentage weights, exposure aggregates, concentration, DNA, and
closed-position percentage returns; they do not expose quantities, monetary
values, buy prices, cost basis, email, or brokerage identifiers.

Leaderboard cards intentionally omit holdings. Composition is available only
through an opted-in public profile or Explore. A user must make both their
profile and weights public to appear in Explore.

## Performance semantics

The app deliberately keeps two differently scoped histories:

1. **Ranked performance** is the public competition and achievement truth. It
   stores a chain-linked checkpoint index and valuation segment. Portfolio
   mutations atomically checkpoint the live index and start a new segment, so
   changing capital or composition has zero immediate effect on ranked return.
   Empty portfolios pause at the preserved index.
2. **Portfolio summary/archive performance** is owner-private accounting-style
   analytics derived from position baselines and realized results. It supports
   cost-basis and composition history, but is not used by leaderboards,
   achievement evaluation, achievement progress, or public profile history.

Canonical ranked snapshots are recorded every four hours by default, once per
UTC day for permanent history, and immediately at epoch/pause/resume
transitions. Older intraday points are retained for 120 days by default and are
removed only when a complete daily point exists; transition and award-evidence
points are preserved. Configure these policies with the `RANKED_*` variables in
`backend/.env.example`.

`GET /performance/history` reads these CANONICAL ranked snapshots — the same
source the leaderboard and benchmark-achievement evidence read. It never reads
the private portfolio valuation archive (`GET /portfolio/archives`), which is
value/cost-basis history and is distorted by deposits and withdrawals. The
backend computes the timeframe return as `ending_index / starting_index - 1`
(not `ending_index - 100`) and the drawdown as `index_t / running_peak_t - 1`
from the ranked index; the frontend only formats those values. When no trusted
snapshot exists yet the response reports `available: false` with a reason and
omits the analytics rather than returning zeros.

The same response also carries two blocks that stay inside the ranked-history
service, never in React:

- `risk` — maximum drawdown, current drawdown (final index vs. its running
  peak), positive share of COMPLETE calendar weeks (the running week is
  excluded), and best/worst COMPLETE calendar month. Every field is `null` with
  a `*_reason` when there is not enough history; it is never 0.
- `benchmark` — a like-for-like comparison against `SPY` built by the SAME
  `internal/benchmark` construction engine the achievement pipeline uses,
  reached through a narrow `BenchmarkReturner` port so `performancehistory` does
  not import `internal/benchmark`. The engine reports the ACTUAL trading dates
  it could measure between, and the portfolio leg is then re-derived from the
  ranked snapshots inside exactly that window. If the two windows cannot be made
  identical the comparison is withheld with a reason — a timeframe mismatch is
  never papered over with a number.

`GET /performance/summary` additionally serves `economic_breakdown` (realized,
unrealized, net income, standalone fees, total economic P&L) and `contributions`
(top 3 contributors/detractors in percentage points). Both are computed from
Step 1's existing ledger figures — `StandaloneFeesBase` is the single definition
shared with `ReconcilePortfolioFinancials`, so the displayed breakdown and the
reconciliation check can never drift. Contribution is `weight x instrument
return`, never standalone instrument return, and declares
`basis: "since_inception"` because no per-instrument daily valuation history
exists; portfolio-level results (cash interest, management/custody fees) are
reported as `unattributed_percentage_points` rather than assigned to a symbol.
These two blocks are serialized ONLY by the performance DTO — the
portfolio-state DTO (`GET /portfolio/summary`) deliberately does not carry them,
so the three layers stay separately owned.

The Dashboard graph renders `GET /performance/history` ranked-index points
(1M window) with its own independent loading/error/empty states — a
history-fetch failure never blanks the rest of the Dashboard.

Redis is a derived acceleration cache for the active `ALL` leaderboard. A
committed pause preserves the ranked index and queues an idempotent `ZREM`; a
resume continues the same epoch/index and queues `ZADD`. The outbox projector
rereads current database-backed ranked state, so delayed pause/resume events
cannot overwrite newer state. Periodic explicit reconciliation upserts active
users and removes paused or deleted members. Redis failures never roll back a
portfolio mutation, and the live database calculation remains the fallback.

## Current limitations and blockers

- Manual tracking only: no broker connection or order execution. Cash ledger,
  deposits/withdrawals, dividends/distributions, fees, and corporate actions
  (splits, symbol changes, write-offs) are tracked; there are no tax lots or
  tax-basis accounting.
- Money and quantity fields are `float64` end-to-end in the Go backend, even
  though the underlying PostgreSQL columns are `NUMERIC`. This is a real
  precision-loss risk versus a fully decimal-safe implementation and was
  deliberately deferred as a separate, larger effort rather than bundled into
  this pass.
- Buy/sell activities cannot be corrected after the fact (only deposits and
  withdrawals can); record an offsetting buy/sell instead.
- Window leaderboards and achievements require epoch-safe trusted snapshots
  around the requested boundary; there is no interpolation.
- Benchmark returns must come from adjusted/total-return data to earn a verified
  award (raw close mishandles splits and dividends). The Twelve Data
  `/time_series` adapter supplies **raw closes**, labelled as such, so it powers
  previews but fails closed for verified awards until an adjusted/total-return
  feed (or raw prices plus dividend/split events) is wired in. Mixed-market
  calendars depend on common available dates.
- Benchmark recipes are immutably versioned; dynamic 13F recipes select the
  version publicly filed at or before the evaluation start (no look-ahead). The
  Berkshire baskets are the authoritative SEC 13F-HR filings for 2025 Q1 and
  2026 Q1 (CIK 0001067983), ~93–97% mapped coverage; older versions are
  preserved for reproducibility. They are proxy portfolios, not replicas of
  Berkshire's return.
- `BENCHMARK_AWARD_MODE` (`disabled`/`demo`/`verified_only`, default
  `verified_only`) governs awards independently of any API key. Mock/synthetic
  data is preview- or demo-only and can never create a verified permanent award;
  evidence records provider, price methodology, recipe version, data quality,
  and verification status, and is immutable once written.
- Ranked snapshot and evaluation workers currently scan the full user list in
  process; repository uniqueness and claim leases are multi-instance safe, but
  the scan is not yet paginated for large populations.
- Weekly competition APIs are implemented, but the active frontend redirects
  `/sprint` and `/arena` to Leaderboard. Completed sprint results are repriced
  rather than finalized and persisted.
- The FX provider is a static development provider for USD, TRY, EUR, and GBP.
- Google login is optional. Apple authentication code is not routed, and no
  Apple frontend flow exists.
- No token refresh/revocation, WebSockets, unread counts, notifications,
  moderation, message editing/deletion, or public unauthenticated social pages.
- This repository is a personal-use prototype, not production or regulatory
  readiness.

## Legacy, transitional, and planned

- **Legacy/unreachable:** Arena and sprint frontend components remain in the
  source tree, but their routes redirect to Leaderboard. Legacy achievement
  tables remain for migration compatibility; current awards use benchmark
  achievement storage.
- **Transitional:** users begin newly verified badge eligibility at their first
  trusted ranked snapshot after migration; legacy archive points are not
  backfilled. Existing awards are preserved and marked `archive_model_v0`.
- **Removed:** the Portfolio Coach implementation and UI are not part of the
  current application.
- **Not implemented:** brokerage execution, AI advice, Apple login, live FX,
  finalized competitions, and production-scale social infrastructure.

## Verification

```bash
cd backend
go test ./...
go vet ./...

cd ../frontend
npm install
npm run lint
npm run build
```

See [backend/README.md](backend/README.md) for API and calculation details and
[frontend/README.md](frontend/README.md) for routes and UI integration.
