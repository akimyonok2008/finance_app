package rules

import (
	"fmt"
	"strings"
)

// PositionFacts is the privacy-safe projection of one snapshot position that
// eligibility evaluation needs: no symbol, quantity, or price, just enough to
// evaluate a Filter. Weight is a decimal fraction of the snapshot's total
// starting value (0..1); the sum of Weight across a snapshot is 1.0.
type PositionFacts struct {
	Sector string
	Weight float64
}

// Evaluate reports whether facts satisfies filter. A zero-value filter always
// returns true. An unsupported Metric or Operator is an error, not a silent
// pass or fail: an eligibility rule that can't be evaluated must not be
// mistaken for one that was evaluated and failed.
func Evaluate(filter Filter, facts []PositionFacts) (bool, error) {
	if filter.IsZero() {
		return true, nil
	}
	switch filter.Metric {
	case MetricPortfolioWeight:
		return evaluatePortfolioWeight(filter, facts)
	default:
		return false, fmt.Errorf("rules: unsupported metric %q", filter.Metric)
	}
}

func evaluatePortfolioWeight(filter Filter, facts []PositionFacts) (bool, error) {
	wanted := make(map[string]bool, len(filter.Sectors))
	for _, s := range filter.Sectors {
		wanted[normalizeSector(s)] = true
	}
	var total float64
	for _, f := range facts {
		if wanted[normalizeSector(f.Sector)] {
			total += f.Weight
		}
	}
	return compare(total, filter.Operator, filter.Threshold)
}

func compare(value float64, op string, threshold float64) (bool, error) {
	switch op {
	case "", OpGTE:
		return value >= threshold, nil
	case OpGT:
		return value > threshold, nil
	case OpLTE:
		return value <= threshold, nil
	case OpLT:
		return value < threshold, nil
	case OpEQ:
		return value == threshold, nil
	default:
		return false, fmt.Errorf("rules: unsupported operator %q", op)
	}
}

func normalizeSector(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
