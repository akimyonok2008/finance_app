# Alarvest Frontend

Responsive React interface for the Alarvest portfolio strategy tracker. It uses
the Go API for authentication, portfolio state, rankings, achievements,
profiles, discovery, friendships, and messages.

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

The JWT and user snapshot are stored in `localStorage`. The app does not refresh
or revalidate the token at bootstrap. Any API `401` clears local auth and
redirects to `/login`.

## Active routes

| Route | Access | Behavior |
| --- | --- | --- |
| `/` | Public | Landing page; authenticated users go to Dashboard |
| `/login`, `/register` | Public | Email/password and optional Google auth |
| `/dashboard` | Protected | Compact portfolio summary and product navigation |
| `/portfolio` | Protected | Active, Closed, and Archive tabs |
| `/leaderboard` | Protected | Global/timeframe rankings and personal standing |
| `/achievements` | Protected | Benchmark badge overview and catalogues |
| `/profile` | Protected | Owner profile preview and settings |
| `/profiles/:handle` | Protected | Privacy-filtered public profile |
| `/explore` | Protected | Strategies, Friends, and Messages workspace |

Compatibility redirects:

- `/arena` and `/sprint` → `/leaderboard`
- `/profile/me` → `/profile`
- `/friends` → `/explore?tab=friends`
- `/messages` and `/messages/:conversationId` → the matching Explore Messages
  state
- unknown routes → `/dashboard`

## Screen behavior

### Dashboard

The Dashboard is a read-only hub. Its compact performance card places the
portfolio index graph beside owner-only value, cost, gain, and return. The graph
is currently a deterministic seven-point curve derived from the latest summary
index; it is not backend archive history. Navigation cards lead to Portfolio,
Leaderboard, Profile, and Explore.

### Portfolio

The Portfolio page has Active, Closed, and Archive views.

- Add accepts symbol, asset type, and quantity. The backend captures the
  current quote as a baseline; there is no buy-price input.
- Edit changes quantity only.
- Delete corrects an active entry.
- Close records current provider price and realized performance without placing
  a trade.
- Desktop tables and mobile cards show owner-only position values and freshness
  metadata.
- Archive charts and summaries use stored backend snapshots for `1W`, `1M`,
  `3M`, `6M`, and `1Y`.

Portfolio mutations invalidate summaries, archives, rankings, profiles, Explore,
and achievement queries.

### Leaderboard

`1W`, `1M`, `3M`, `6M`, `1Y`, and `ALL` controls call the matching backend
timeframe. Window boards can be empty until enough ranked snapshots exist.
Rows display identity, rank, ranked index/return, and profile navigation.
Holding symbols and weights were intentionally removed from leaderboard cards;
users inspect opted-in composition on a profile.

The personal standing card displays the backend values, but rank movement is
usually absent because historical rank delta is not currently recorded.

### Achievements

The dedicated screen presents twenty benchmark badges across Overview,
Legendary, Strategy, and All views. Badges have distinct icons and color
identities. Detail dialogs expose recipes, rules, evidence, and real evaluation
fields when the API supplies them.

`POST /achievements/evaluate` provides progress and can permanently award
badges. Progress is a backend presentation heuristic, not a probability. It is
based on portfolio archives and benchmark history rather than persistent ranked
performance.

### Profiles

The owner can edit display name, bio, strategy tag, and the separate public
profile/public weights controls. Public pages may display headline return/index,
archive history, ranks, unlocked badges, DNA, exposure aggregates,
concentration, and—when opted in—symbols, weights, and closed-symbol percentage
returns.

Headline performance can use persistent ranked state while chart history uses
archive index data, so the two can diverge. The benchmark section is currently
a placeholder. Compare and Copy are deterministic template tools and do not
trade or copy monetary amounts.

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

Owner routes may display quantities, prices, values, cost basis, and absolute
gain/loss. Public UI must not display those fields. Public strategy surfaces may
display:

- display name, handle, avatar, and strategy tag
- percentage return/index, rank, achievements, and archive index history
- DNA, exposure, concentration, and performance-driver aggregates
- symbols, asset types, and percentage weights only when sharing permits it
- closed symbols and percentage returns only when sharing permits it

Leaderboard rows deliberately do not render composition. Explore includes only
profiles with both public visibility and public weights enabled.

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
  tree, but active routes redirect to Leaderboard.
- **Removed:** Coach API, hooks, types, components, and portfolio analysis modes.
- **Not implemented:** Apple sign-in UI, live competition UI, broker trading,
  AI advice, token refresh, and production social features.

## Build checks

```bash
npm install
npm run lint
npm run build
```
