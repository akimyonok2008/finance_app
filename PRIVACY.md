# Product Data Visibility Policy

Effective: July 28, 2026

Alarvest protects **monetary account details**, while allowing users to share
percentage-based strategy composition. A symbol and its allocation percentage
are treated as public strategy information when the user explicitly enables
composition sharing. They are not treated as private monetary data.

This policy describes product visibility boundaries. It does not replace a
jurisdiction-specific legal privacy notice covering data collection, storage,
processors, retention, or data-subject rights.

## Visibility controls

Profiles and composition sharing are off by default.

- `Public profile` (`is_public`) allows other users to view the profile,
  identity fields, ranked performance, and non-monetary strategy analytics.
- `Show public weights` (`show_public_weights`) is a separate opt-in. It has no
  public effect while the profile is private. When both controls are enabled,
  Alarvest may publish symbol-level strategy composition and reuse it in
  profile, Explore, comparison, copying, trending, and leaderboard features.
- Turning either control off prevents subsequent API responses from disclosing
  the affected profile or composition. Information already seen or copied by
  another person cannot be recalled from that person.

Competitive rankings may show a participant's display name, avatar, rank,
ranked index, and percentage return even when their profile is private. A
private profile does not expose its handle, profile page, or composition.

## Visibility matrix

| Data | Owner workspace | Public profile | Public profile with weights enabled |
| --- | --- | --- | --- |
| Display name, avatar, bio, strategy tag | Yes | Yes | Yes |
| Ranked index, percentage return, rank, ranked history, badges | Yes | Yes | Yes |
| Aggregate exposure, concentration, and strategy/DNA signals | Yes | Yes | Yes |
| Active symbols, asset types, and allocation percentages | Yes | No | Yes |
| Closed-position symbols and percentage returns | Yes | No | Yes |
| Symbol-level contributors and detractors | Yes | No | Yes |
| Percentage strategy weights used by Compare or Copy | Yes | No | Yes |
| Quantities, balances, monetary values, cost basis, transaction prices, or absolute gain/loss | Yes | Never | Never |

Benchmark symbols identify public reference benchmarks; they do not assert that
the user owns those instruments.

## Data that must never appear on public or competitive surfaces

- position quantities or units;
- cash or account balances;
- portfolio, position, or transaction monetary values;
- purchase, sale, execution, or average prices;
- cost basis, proceeds, fees, or absolute gain/loss;
- email addresses, authentication data, provider subjects, brokerage
  identifiers, or internal user/portfolio/position IDs.

Public API models and UI components must use dedicated projections that omit
these fields. Adding a field to an owner-facing portfolio response does not
authorize adding it to a profile, Explore, leaderboard, achievement, social, or
strategy-sharing response.

## Product-language rule

Product copy should say that **amounts and account values remain private**. It
must not promise that rankings or holdings are universally private, because an
eligible ranking is competitive information and opted-in composition includes
holding symbols.
