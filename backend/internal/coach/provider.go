package coach

import "context"

// Provider turns deterministic, sanitized facts into narrative analysis. The
// concrete implementation is selected by configuration; MockProvider is the
// key-free default used for local development and tests.
//
// Implementations MUST return analysis only — never buy/sell/hold advice — and
// MUST NOT infer private values for other users from public weights.
type Provider interface {
	GeneratePortfolioCoachAnalysis(ctx context.Context, input CoachProviderInput) (CoachProviderOutput, error)
}

// MarketResearchProvider is a future extension point for real fundamental data
// (revenue, earnings, valuation, sector classification). It is intentionally
// unimplemented in this prototype: there is no fundamental data source yet, and
// the coach must not fabricate company financials.
//
// TODO(prototype-2+): implement against a licensed research/data vendor and feed
// its output into fundamental_context analysis.
type MarketResearchProvider interface {
	SymbolContext(ctx context.Context, symbol string) (map[string]string, error)
}
