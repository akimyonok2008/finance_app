# Alarvest

Alarvest is a full-stack portfolio strategy tracker. Users manually enter
holdings, follow percentage performance, compare privacy-filtered public
strategies, earn benchmark badges, and connect with other users. It is a
tracking and education product: it does not connect to a broker or place
trades.

## Current product

- Email/password authentication and optional Google Identity Services login
- One owner-only portfolio per user, with active positions, quantity edits,
  close history, archive snapshots, and base-currency summaries
- Persistent ranked-performance state designed to neutralize the effect of
  adding, resizing, closing, deleting, or copying positions
- Global `ALL` and snapshot-backed `1W`, `1M`, `3M`, `6M`, and `1Y`
  leaderboards
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

Two performance models currently coexist:

1. **Ranked performance** stores a checkpoint index and a valuation segment.
   Portfolio mutations checkpoint the live index and start a new segment, so
   changing capital or composition is intended to have zero immediate effect on
   ranked return. Empty portfolios are paused and excluded while preserving
   their index.
2. **Portfolio summary/archive performance** compares the current basket with
   its stored position baselines and realized close results. This index is used
   in owner summaries, archives, public history, and achievement evaluation. It
   is not account-level time-weighted return and can change when the portfolio
   is resized.

The Dashboard graph is currently a deterministic seven-point presentation
derived from the current summary index. The Portfolio Archive tab is the UI
surface that displays stored archive points.

## Current limitations and blockers

- Manual tracking only: no broker connection, order execution, cash ledger,
  deposits, dividends, fees, tax lots, splits, or corporate-action accounting.
- Portfolio writes and ranked checkpoints are separate repository operations,
  not one database transaction. A rare checkpoint failure after a successful
  position write can leave them inconsistent.
- Redis `ALL` leaderboard data is eventually consistent. A paused user can
  remain in a previously populated cache until it is refreshed or cleared.
- Window leaderboards require a post-tracking snapshot at or before the cutoff;
  there is no interpolation.
- Achievement history uses portfolio archives, and Twelve Data history uses
  close prices rather than adjusted/total-return data. The Berkshire recipe is
  a hard-coded filing snapshot. Permanent awards can be produced from mock
  benchmark data in a mock-configured environment.
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
- **Transitional:** public profile history and achievement evaluation still use
  the archive index while leaderboard headlines use persistent ranked
  performance.
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
