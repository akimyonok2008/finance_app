// Package fx provides currency conversion behind an interface so portfolio,
// leaderboard, and competition math can be normalized to a single base currency
// (USD for the prototype). The mock rates are deterministic and prototype-only.
package fx

// BaseCurrency is the currency all portfolio/competition math is normalized to.
const BaseCurrency = "USD"
