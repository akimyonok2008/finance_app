// Package rules implements competition eligibility filters: a small,
// serializable expression evaluated against a user's join-time portfolio
// snapshot to decide whether they may enter a competition. It is deliberately
// generic (a competition's Filter is opaque JSON to the rest of the
// competitions package) so new dimensions can be added without touching the
// join path in service.go.
package rules

// Filter describes one eligibility rule. The zero value (Metric == "") always
// evaluates to eligible: a competition with no filter admits everyone.
type Filter struct {
	// Metric selects what is being measured. Only MetricPortfolioWeight is
	// implemented; unknown metrics fail evaluation rather than silently
	// admitting everyone.
	Metric string `json:"metric"`
	// Sectors is the set of instrument.Sector values the metric aggregates
	// over. Matching is case-insensitive and whitespace-trimmed.
	Sectors []string `json:"sectors,omitempty"`
	// Operator compares the aggregated metric against Threshold. Defaults to
	// OpGTE when empty.
	Operator string `json:"operator,omitempty"`
	// Threshold is a decimal fraction (0.5 == 50%) for MetricPortfolioWeight.
	Threshold float64 `json:"threshold"`
}

// IsZero reports whether f carries no rule at all (Metric unset), the
// "everyone is eligible" case.
func (f Filter) IsZero() bool {
	return f.Metric == ""
}

// Supported metrics.
const (
	// MetricPortfolioWeight aggregates the fraction of a snapshot's total
	// starting value held in positions whose sector is in Filter.Sectors,
	// and compares it against Threshold.
	MetricPortfolioWeight = "portfolio_weight"
)

// Supported comparison operators.
const (
	OpGTE = ">="
	OpGT  = ">"
	OpLTE = "<="
	OpLT  = "<"
	OpEQ  = "=="
)
