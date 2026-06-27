# Finance App Frontend

Premium, dark-mode-first React interface for the gamified real-portfolio
tracker. The frontend talks directly to the Go API and currently provides
authentication, a read-only game dashboard, manual portfolio management, a
unified strategy leaderboard, Explore, public profiles, and Portfolio Coach.
It also includes standalone owner and public strategy profile screens.
Explore provides public-safe strategy discovery without exposing wealth.

## Implemented Screens

### Authentication

- Register and sign in with the real backend.
- JWT and user session persisted in `localStorage`.
- Protected application routes.
- Automatic session clearing and redirect to `/login` after a `401`.
- Optional mock-auth fallback through `VITE_ENABLE_MOCK_AUTH`.

### Dashboard

- Portfolio index, value, cost basis, gain/loss, and percentage performance.
- Deterministic prototype index chart until historical snapshots are available.
- Current competition status and privacy-safe standings preview.
- Personal global rank and full privacy-safe leaderboard detail view.
- Achievement preview.
- Working Dashboard and Portfolio navigation, refresh, logout, skeletons,
  partial errors, and new-user onboarding.
- Responsive bento layout for desktop and mobile.

The dashboard is intentionally read-only. Portfolio management lives on the
Portfolio page; ranked performance and achievements live on the Leaderboard.

### Portfolio

- Portfolio summary cards.
- Add a position with just **symbol, asset type, and quantity** — the backend
  locks the baseline at today's market price, so every position starts at index
  `100`. There is no buy-price or currency input.
- Edit is **quantity-only**; the symbol and locked baseline are immutable (delete
  and re-add to re-baseline). Delete with confirmation.
- Desktop form and positions table (with a Baseline column), mobile add drawer
  and position cards, all showing base-currency value, gain/loss, and
  FX-normalized return measured from the locked baseline.
- Loading, empty, validation-error, and confirmation states.
- Query invalidation keeps portfolio, dashboard ranking, and achievements fresh.

### Leaderboard

- Privacy-safe ranked performance with `1W`, `1M`, `3M`, `6M`, `1Y`, and `ALL`
  timeframe controls — the tabs hit `GET /leaderboard?timeframe=…`, which the
  backend honours (windows fall back to since-baseline until history accrues).
  Eligibility is automatic — there is no "join" or baseline step; the
  locked-baseline portfolio is the ranked source of truth.
- Public rows link to the profile and show strategy tag + opted-in weight chips
  (enriched by the backend); private profiles stay anonymous. The current user's
  row is highlighted.
- Ranked rows plus the achievement collection in one screen.
- Legacy `/arena`, `/sprint`, and `/achievements` routes redirect to
  `/leaderboard`.

### Portfolio Coach

- Four private analysis modes: Fundamental Analysis, Technical Analysis,
  Portfolio Review, and Compare with Top 10.
- Top 10 comparisons may use public symbols, asset types, and percentage
  weights.
- Quantities and all monetary values remain private.

### Profiles

- `/profile` lets the authenticated owner edit profile metadata, strategy tag,
  and public visibility settings while viewing a public preview.
- `/profiles/:handle` shows a privacy-filtered public strategy profile with
  performance, ranks, badges, symbols, percentage weights, exposures, and
  concentration.
- Profile calls use `GET /profiles/me`, `PATCH /profiles/me`, and
  `GET /profiles/{handle}`.
- Public profiles show symbols and percentage weights, not quantities, values,
  cost basis, or buy prices.

### Explore Strategies

- `/explore` shows featured public strategies, a **Similar to You** section
  (profiles overlapping your holdings/approach), top performers, and trending
  holdings.
- Search supports public profiles and symbols, with an explicit symbol filter
  and top/return/rank/recent sorting.
- Profile cards link to `/profiles/:handle`; trending symbols apply an Explore
  filter rather than opening a discussion or symbol page.
- Explore uses `GET /profiles/explore` and renders only public-safe profile,
  ranked-performance, badge, symbol, asset-type, and percentage-weight fields.
- Explore shows symbols and weights only. It does not show quantities,
  portfolio values, cost basis, average buy prices, or absolute gain/loss.

## Privacy Surfaces

- Dashboard and Portfolio are owner-only and may display the authenticated
  user's positions and monetary totals.
- Leaderboard rows display ranked performance and may display opted-in symbols,
  asset types, and percentage weights.
- Coach comparison profiles may display public composition using symbols,
  asset types, and percentage weights.
- Public screens never display quantities, average buy prices, position prices,
  portfolio value, cost basis, absolute gain/loss, user IDs, emails, or
  brokerage identifiers.

## Stack

- React 19 + TypeScript + Vite
- React Router
- Tailwind CSS v3
- Hand-rolled shadcn-style UI primitives with Radix and Vaul
- TanStack Query v5
- React Hook Form + Zod
- Recharts
- Framer Motion
- Lucide React
- Sonner

## Run

The Go API must be available at `http://localhost:8080` by default.

```bash
# Terminal 1
cd backend
go run ./cmd/api

# Terminal 2
cd frontend
npm install
npm run dev
```

Open `http://localhost:5173`.

## Configuration

Copy `.env.example` or provide equivalent values:

```env
VITE_API_BASE_URL=http://localhost:8080
VITE_AUTH_LOGIN_PATH=/auth/login
VITE_AUTH_REGISTER_PATH=/auth/register
VITE_ENABLE_MOCK_AUTH=false
```

The real JWT is stored under `finance_app_token`; the authenticated user is
stored under `finance_app_user`.

## Routes

| Route | Status | Purpose |
| --- | --- | --- |
| `/login` | Implemented | Sign in |
| `/register` | Implemented | Create an account |
| `/dashboard` | Implemented | Performance and game overview |
| `/portfolio` | Implemented | Manage holdings |
| `/leaderboard` | Implemented | Strategy baseline, ranked performance, privacy, and achievements |
| `/arena` | Redirected | Unified into `/leaderboard` |
| `/coach` | Implemented | Private analysis and public Top 10 composition comparison |
| `/explore` | Implemented | Discover public strategies, performers, and trending holdings |
| `/profile` | Implemented | Edit owner profile settings and view public preview |
| `/profiles/:handle` | Implemented | View a privacy-filtered public strategy profile |
| `/profile/me` | Redirected | Owner profile lives at `/profile` |
| `/sprint` | Redirected | Unified into `/leaderboard` |
| `/achievements` | Redirected | Unified into `/leaderboard` |

Unknown and unimplemented routes currently redirect to `/dashboard`.

## Project Structure

```text
src/
  api/             authenticated API client and domain API helpers
  auth/            provider, storage, protected route, and auth context
  components/
    portfolio/     position forms, tables, cards, dialogs, and summaries
    explore/       public-safe strategy discovery cards and filters
    profile/       reusable privacy-safe profile display and settings form
    ui/            reusable UI primitives
  hooks/           TanStack Query hooks and centralized query keys
  pages/
    auth/          login and registration
    Dashboard/     bento dashboard and formatters
    arena/         competition and achievement experience
    coach/         portfolio analysis and Top 10 comparison
    leaderboard/   strategy baseline and privacy-safe ranked standings
    Explore/       strategy discovery page
    Profile/       owner and public profile screens
    PortfolioPage.tsx
  types/           API and form types
  utils/           formatting and class-name helpers
```

## Server-State Rules

Portfolio mutations invalidate:

```text
positions
portfolioSummary
leaderboard
leaderboardMe
achievements
```

This is required because a position change can affect portfolio values,
personal rank, global standings, and badge eligibility.

## Demo Market Data

Supported mock symbols:

```text
AAPL, MSFT, NVDA, SPY, BTC-USD, ETH-USD, THYAO.IS, GARAN.IS, ASELS.IS
```

Supported currencies: `USD`, `TRY`, `EUR`, `GBP`.

## Verification

```bash
npm run lint
npm run build
```

Both commands currently pass. The production build warns that the main
JavaScript chunk exceeds 500 kB; route-based code splitting is planned as more
screens are added.

## Known Frontend Gaps

- The dashboard chart is derived from the current index because the backend
  does not yet expose per-position portfolio history.
- Personal ranking uses `GET /leaderboard/me` (exact rank + participant count);
  `rank_delta` is not shown because rank history is not tracked yet.
- Ranked performance and achievements are unified on the Leaderboard.
- The current achievement API exposes unlock state but not numeric progress, so
  legacy locked badges display `0 / 1` and unlocked badges display `1 / 1`.
- The frontend sends timeframe filters, but timeframe-specific rankings and a
  personalized eligibility row require a future leaderboard backend contract.
- Google sign-in is a visual prototype only and is disabled unless mock auth is
  enabled.
- There is no automated frontend test suite yet; build, lint, and browser QA
  are the current verification steps.
