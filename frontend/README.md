# Finance App Frontend

Premium, dark-mode-first React interface for the gamified real-portfolio
tracker. The frontend talks directly to the Go API and currently provides
authentication, a central dashboard hub, manual portfolio management with
symbol autocomplete, a unified strategy leaderboard, Explore, public profiles,
and Portfolio Coach inside the Portfolio page. Explore is the single
discovery/social hub for public strategies, mutual-follower friends, and private
one-to-one messages.
It also includes standalone owner and public strategy profile screens.
Explore provides public-safe strategy discovery without exposing wealth.

## Implemented Screens

### Authentication

- Register and sign in with the real backend.
- Successful auth lands on `/dashboard`, the primary post-login product hub.
- JWT and user session persisted in `localStorage`.
- Optional Google sign-in calls the backend provider endpoint; the backend
  returns the same app JWT as password login.
- Protected application routes.
- Automatic session clearing and redirect to `/login` after a `401`.
- Optional mock-auth fallback through `VITE_ENABLE_MOCK_AUTH`.

### Dashboard

- Central overview for the authenticated product: private portfolio status,
  personal rank, strategy baseline state, achievements, and next best action.
- Portfolio Index chart plus compact owner snapshot cards for Portfolio,
  Leaderboard, Profile, and Explore.
- Compact market-data status card shows provider, latest refresh, and stale
  quote count when available.
- Owner-only private snapshot can show portfolio value, return, and index.
- Working refresh, logout, responsive layout, and new-user onboarding path.

The dashboard is intentionally a compact read-only hub. Portfolio management
lives on the Portfolio page; ranked performance and achievements live on the
Leaderboard.

### Portfolio

- `/portfolio` has internal tabs for Positions and Coach.
- `/portfolio?tab=coach` opens the private Portfolio Coach analysis surface.
- Portfolio summary cards.
- Symbol autocomplete calls `GET /instruments/search?q=...` and shows symbol,
  name, exchange, currency, and asset type. Users select a listed instrument so
  prices can update automatically.
- Add a position with just **symbol, asset type, and quantity** — the backend
  locks the baseline at today's market price, so every position starts at index
  `100`. There is no buy-price or currency input.
- Edit is **quantity-only**; the symbol and locked baseline are immutable (delete
  and re-add to re-baseline). Delete with confirmation.
- Desktop form and positions table (with a Baseline column), mobile add drawer
  and position cards, all showing base-currency value, gain/loss, and
  FX-normalized return measured from the locked baseline.
- Loading, empty, validation-error, and confirmation states.
- Quote freshness badges show mock/cached/stale/provider-unavailable status on
  position rows and cards. The frontend never calls Twelve Data directly.
- Query invalidation keeps portfolio, dashboard ranking, and achievements fresh.
- Four private analysis modes: Fundamental Analysis, Technical Analysis,
  Portfolio Review, and Compare with Top 10.
- Top 10 comparisons may use public symbols, asset types, and percentage
  weights.
- Public profiles can also be compared directly with "Compare with me", which
  shows overlap, weight gaps, concentration, deterministic learning points, and
  an education-only disclaimer.
- Quantities and all monetary values remain private.

### Leaderboard

- Privacy-safe ranked performance with `1W`, `1M`, `3M`, `6M`, `1Y`, and `ALL`
  timeframe controls — the tabs hit `GET /leaderboard?timeframe=…`, which the
  backend honours. Non-`ALL` windows show only strategies with enough snapshot
  history; the screen shows a clean not-enough-history state when needed.
  Eligibility is automatic — there is no "join" or baseline step; the
  locked-baseline portfolio is the ranked source of truth.
- A compact "Your Standing" card uses `GET /leaderboard/me?timeframe=...` to
  show rank, movement when available, best rank, participant count, percentile,
  next milestone, ranked index, and ranked return.
- Public rows link to the profile and show strategy tag + opted-in weight chips
  (enriched by the backend); private profiles stay anonymous. The current user's
  row is highlighted.
- Public-weight rows include compact Compare and Copy weights actions. Copying
  opens a review modal and creates a fresh local baseline only; no trades are
  executed.
- Ranked rows plus the achievement collection in one screen.
- Legacy `/arena`, `/sprint`, and `/achievements` routes redirect to
  `/leaderboard`.

### Profiles

- `/profile` lets the authenticated owner edit profile metadata, strategy tag,
  and public visibility settings while viewing a public preview.
- `/profiles/:handle` shows a privacy-filtered public strategy profile with
  performance, ranks, badges, symbols, percentage weights, exposures, and
  concentration.
- Profile calls use `GET /profiles/me`, `PATCH /profiles/me`, and
  `GET /profiles/{handle}`.
- Public profiles include a Follow control. Mutual friends get a Message CTA;
  non-mutual profiles explain that messaging unlocks after both users follow.
- Public profiles show symbols and percentage weights, not quantities, values,
  cost basis, or buy prices.
- Visible public-weight profiles expose Compare with me and Copy weights CTAs.
  Copying shows a read-only preview and clearly states that amounts, quantities,
  cost basis, and portfolio values are never copied.

### Explore

- `/explore` has internal tabs for Strategies, Friends, and Messages.
- `/explore?tab=strategies` shows featured public strategies, a **Similar to
  You** section (profiles overlapping your holdings/approach), top performers,
  and trending holdings.
- Timeframe chips (`1W`, `1M`, `3M`, `6M`, `1Y`, `ALL`) pass
  `timeframe=...` to `GET /profiles/explore`; pagination resets when the
  timeframe changes.
- Search supports public profiles and symbols, with an explicit symbol filter
  and top/return/rank/recent sorting.
- Profile cards include View profile, Compare with me, and Copy weights actions;
  trending symbols apply an Explore filter while preserving the selected
  timeframe.
- Explore uses `GET /profiles/explore` and renders only public-safe profile,
  ranked-performance, badge, symbol, asset-type, and percentage-weight fields.
- Explore shows symbols and weights only. It does not show quantities,
  portfolio values, cost basis, average buy prices, or absolute gain/loss.
- `/explore?tab=friends` shows mutual friends, following, and followers.
- Friend cards show only handle, display name, avatar key, and strategy tag.
- Mutual friends include Message and View profile actions.
- Query invalidation refreshes follow state, friends/followers/following, public
  profile data, and DM conversations after follow/unfollow.
- `/explore?tab=messages` shows authenticated one-to-one conversations and
  message history.
- Conversations can be created only with mutual followers.
- Sending a message is blocked by the backend if either user unfollows.
- The page polls conservatively every 30 seconds while open; no WebSockets.
- Messages never include portfolio values, quantities, cost basis, buy prices,
  emails, or raw user IDs.
- Legacy `/friends` and `/messages` routes redirect into the matching Explore
  tabs.

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
VITE_GOOGLE_AUTH_ENABLED=false
VITE_GOOGLE_CLIENT_ID=
```

The real JWT is stored under `finance_app_token`; the authenticated user is
stored under `finance_app_user`.

The Google button is hidden unless `VITE_GOOGLE_AUTH_ENABLED=true`. Create a
Google OAuth Web Client ID with `http://localhost:5173` as an Authorized
JavaScript origin. The current Google Identity Services callback flow does not
need an Authorized redirect URI. Frontend provider tokens are never trusted
directly; they are sent to the Go API for server-side verification. Mock auth
remains limited to the email/password fallback path behind
`VITE_ENABLE_MOCK_AUTH=true`.

## Routes

| Route | Status | Purpose |
| --- | --- | --- |
| `/login` | Implemented | Sign in |
| `/register` | Implemented | Create an account |
| `/dashboard` | Implemented | Central overview and product hub |
| `/portfolio` | Implemented | Manage holdings and open Portfolio Coach |
| `/leaderboard` | Implemented | Strategy baseline, ranked performance, privacy, and achievements |
| `/arena` | Redirected | Unified into `/leaderboard` |
| `/coach` | Redirected | Portfolio Coach lives at `/portfolio?tab=coach` |
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
    Dashboard/     overview hub and formatters
    arena/         competition and achievement experience
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
profile/me
exploreProfiles
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

## Prototype 3 Handoff Notes

- Vite is pinned to port `5173` with `strictPort`; Google OAuth local setup
  should use `http://localhost:5173` as the Authorized JavaScript origin.
  `127.0.0.1` is a different OAuth origin.
- The Google button uses Google Identity Services' official renderer. If the
  GIS script cannot load, the UI shows an unavailable state rather than a custom
  fake provider button.
- Portfolio symbol search and quote freshness are frontend shells over backend
  APIs. Twelve Data keys must stay in `backend/.env`; never add provider keys to
  Vite env files.
- Explore Friends and Messages are built as authenticated polling views, not a
  realtime chat product.
- Copy weights and Portfolio Coach are educational/productivity flows only: no
  trading, no quantities copied, and no private public-profile data displayed.

## Known Frontend Gaps

- The dashboard chart is derived from the current index because the backend
  does not yet expose per-position portfolio history.
- Personal ranking uses `GET /leaderboard/me`; `rank_delta` is shown when the
  backend returns it, but true rank movement history is not tracked yet.
- Ranked performance and achievements are unified on the Leaderboard.
- The current achievement API exposes unlock state but not numeric progress, so
  legacy locked badges display `0 / 1` and unlocked badges display `1 / 1`.
- True rank movement history is still future work, so compare/copy flows do not
  create movement-feed events yet.
- Google sign-in requires developer-console setup and matching backend +
  frontend client IDs.
- There is no automated frontend test suite yet; build, lint, and browser QA
  are the current verification steps.
- There is no route-level code splitting yet, so the production bundle emits a
  large-chunk warning.
- This configuration is intended for personal use, not public launch.
