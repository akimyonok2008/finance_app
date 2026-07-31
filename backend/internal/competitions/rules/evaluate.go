package rules

import (
	"strconv"
	"time"

	"github.com/ardakimyonok/finance_app/internal/money"
)

// PositionFacts is one position's rule-relevant classification and exact
// base-currency value. Classification comes from the stable instrument
// identity layer (asset type, venue MIC, countries, currency, instrument ID)
// — never from symbol-name heuristics. ValueBase is internal input only; the
// engine never emits absolute values in evidence.
type PositionFacts struct {
	InstrumentID   string
	AssetType      string
	Sector         string
	VenueMIC       string
	IssuerCountry  string
	ListingCountry string
	Currency       string
	ValueBase      money.Amount
}

// PortfolioFacts is the aggregate composition a rule evaluation runs against.
// TotalValueBase = position values + cash. Weight metrics that describe
// portfolio COMPOSITION (portfolio_weight, largest_position_weight) are
// measured against investedValueBase = TotalValueBase - CashValueBase, i.e.
// the user's CURRENTLY INVESTED holdings — idle cash sitting in the account
// never dilutes them, so "30% crypto" means 30% of what's actually invested.
// cash_weight is the one metric that is, by definition, about cash's own
// share of the whole account, so it alone still divides by TotalValueBase.
// UniverseMembership carries the resolved named universes (universe ->
// instrument ID set), supplied by the caller: resolution is I/O, evaluation
// is pure.
type PortfolioFacts struct {
	Positions          []PositionFacts
	CashValueBase      money.Amount
	TotalValueBase     money.Amount
	PortfolioAgeDays   int
	UniverseMembership map[string]map[string]bool
}

// Evidence is the structured, privacy-safe outcome of one rule: required and
// actual values only ever contain percentages, counts or day counts — never
// absolute monetary values.
type Evidence struct {
	Code     string `json:"code"`
	Label    string `json:"label"`
	Metric   string `json:"metric"`
	Operator string `json:"operator"`
	Required string `json:"required"`
	Actual   string `json:"actual"`
	Passed   bool   `json:"passed"`
	Reason   string `json:"reason,omitempty"`
}

// Result is a full eligibility evaluation.
type Result struct {
	Eligible    bool       `json:"eligible"`
	EvaluatedAt time.Time  `json:"evaluated_at"`
	Rules       []Evidence `json:"rules"`
}

// Reason codes attached to failed evidence.
const (
	ReasonEmptyPortfolio  = "empty_portfolio"
	ReasonUnknownUniverse = "unknown_universe"
	ReasonBelowRequired   = "below_required"
	ReasonAboveRequired   = "above_required"
	ReasonOutsideRange    = "outside_range"
	ReasonNotEqual        = "not_equal"
)

// weightDivPlaces is the exact-division precision for weight fractions,
// matching the money domain's high-precision intermediate convention.
const weightDivPlaces = 18

// Evaluate runs the eligibility document against facts: every rule in All
// must pass, and — when Any is present — at least one rule in Any must pass.
// Evidence is produced for EVERY rule (including the passing ones), so the
// persisted join record shows the complete picture the decision was based on.
func Evaluate(e Eligibility, facts PortfolioFacts, now time.Time) Result {
	result := Result{Eligible: true, EvaluatedAt: now.UTC()}
	for _, r := range e.All {
		ev := evaluateRule(r, facts)
		result.Rules = append(result.Rules, ev)
		if !ev.Passed {
			result.Eligible = false
		}
	}
	if len(e.Any) > 0 {
		anyPassed := false
		for _, r := range e.Any {
			ev := evaluateRule(r, facts)
			result.Rules = append(result.Rules, ev)
			if ev.Passed {
				anyPassed = true
			}
		}
		if !anyPassed {
			result.Eligible = false
		}
	}
	return result
}

// Matches reports whether one position satisfies the filter. Populated
// dimensions AND together; values within a dimension OR together. Universe
// membership uses the resolved instrument-ID sets in facts; an unresolved
// universe matches nothing (fail closed).
func Matches(f *Filter, p PositionFacts, universes map[string]map[string]bool) bool {
	if f.IsZero() {
		return true
	}
	if !valueIn(p.AssetType, f.AssetTypes) || !valueIn(p.Sector, f.Sectors) ||
		!valueIn(p.VenueMIC, f.VenueMICs) || !valueIn(p.IssuerCountry, f.IssuerCountries) ||
		!valueIn(p.ListingCountry, f.ListingCountries) || !valueIn(p.Currency, f.Currencies) ||
		!valueIn(p.InstrumentID, f.InstrumentIDs) {
		return false
	}
	if f.Universe != "" {
		members, known := universes[f.Universe]
		if !known || !members[p.InstrumentID] {
			return false
		}
	}
	return true
}

func valueIn(v string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, a := range allowed {
		if a == v {
			return true
		}
	}
	return false
}

func evaluateRule(r Rule, facts PortfolioFacts) Evidence {
	ev := Evidence{
		Code: r.Code, Label: r.Label, Metric: r.Metric, Operator: r.Operator,
	}

	actual, displayAsPercent, reason := metricValue(r, facts)
	ev.Required = displayValue(money.MustRatio(r.Value), displayAsPercent)
	if r.Operator == OpBetween {
		ev.Required += " - " + displayValue(money.MustRatio(r.ValueHigh), displayAsPercent)
	}
	ev.Actual = displayValue(actual, displayAsPercent)
	if reason != "" {
		ev.Passed = false
		ev.Reason = reason
		return ev
	}

	ev.Passed = compare(actual, r)
	if !ev.Passed {
		ev.Reason = failureReason(r.Operator)
	}
	return ev
}

// metricValue computes the exact actual value for a rule's metric.
// displayAsPercent reports whether required/actual should be rendered as
// percentages (weight metrics) rather than plain numbers (counts, days).
// A non-empty reason means the metric could not be computed and the rule
// fails closed with that reason.
func metricValue(r Rule, facts PortfolioFacts) (actual money.Ratio, displayAsPercent bool, reason string) {
	switch r.Metric {
	case MetricPortfolioWeight:
		return filteredWeight(r.Filter, facts)
	case MetricCashWeight:
		if facts.TotalValueBase.Sign() <= 0 {
			return money.ZeroRatio(), true, ReasonEmptyPortfolio
		}
		w, err := facts.CashValueBase.DivExact(facts.TotalValueBase, weightDivPlaces)
		if err != nil {
			return money.ZeroRatio(), true, ReasonEmptyPortfolio
		}
		return w, true, ""
	case MetricLargestPositionWeight:
		invested := investedValueBase(facts)
		if invested.Sign() <= 0 {
			return money.ZeroRatio(), true, ReasonEmptyPortfolio
		}
		largest := money.ZeroAmount()
		for _, p := range facts.Positions {
			if p.ValueBase.Cmp(largest) > 0 {
				largest = p.ValueBase
			}
		}
		w, err := largest.DivExact(invested, weightDivPlaces)
		if err != nil {
			return money.ZeroRatio(), true, ReasonEmptyPortfolio
		}
		return w, true, ""
	case MetricPositionCount:
		count := 0
		for _, p := range facts.Positions {
			if Matches(r.Filter, p, facts.UniverseMembership) {
				count++
			}
		}
		return ratioFromInt(count), false, ""
	case MetricAssetClassCount:
		classes := map[string]bool{}
		for _, p := range facts.Positions {
			if p.AssetType != "" {
				classes[p.AssetType] = true
			}
		}
		return ratioFromInt(len(classes)), false, ""
	case MetricPortfolioAgeDays:
		return ratioFromInt(facts.PortfolioAgeDays), false, ""
	default:
		// Unreachable for parsed documents; fail closed regardless.
		return money.ZeroRatio(), false, "unknown_metric"
	}
}

func filteredWeight(f *Filter, facts PortfolioFacts) (money.Ratio, bool, string) {
	invested := investedValueBase(facts)
	if invested.Sign() <= 0 {
		return money.ZeroRatio(), true, ReasonEmptyPortfolio
	}
	if f != nil && f.Universe != "" {
		if _, known := facts.UniverseMembership[f.Universe]; !known {
			return money.ZeroRatio(), true, ReasonUnknownUniverse
		}
	}
	matching := money.ZeroAmount()
	for _, p := range facts.Positions {
		if Matches(f, p, facts.UniverseMembership) {
			matching = matching.Add(p.ValueBase)
		}
	}
	w, err := matching.DivExact(invested, weightDivPlaces)
	if err != nil {
		return money.ZeroRatio(), true, ReasonEmptyPortfolio
	}
	return w, true, ""
}

// investedValueBase is the value of what the user actually holds right now
// (positions only) — the denominator for composition weight metrics, so idle
// cash sitting in the account never dilutes them.
func investedValueBase(facts PortfolioFacts) money.Amount {
	return facts.TotalValueBase.Sub(facts.CashValueBase)
}

func compare(actual money.Ratio, r Rule) bool {
	value := money.MustRatio(r.Value)
	switch r.Operator {
	case OpGTE:
		return actual.Cmp(value) >= 0
	case OpGT:
		return actual.Cmp(value) > 0
	case OpLTE:
		return actual.Cmp(value) <= 0
	case OpLT:
		return actual.Cmp(value) < 0
	case OpEQ:
		return actual.Cmp(value) == 0
	case OpBetween:
		high := money.MustRatio(r.ValueHigh)
		return actual.Cmp(value) >= 0 && actual.Cmp(high) <= 0
	}
	return false
}

func failureReason(operator string) string {
	switch operator {
	case OpGTE, OpGT:
		return ReasonBelowRequired
	case OpLTE, OpLT:
		return ReasonAboveRequired
	case OpBetween:
		return ReasonOutsideRange
	default:
		return ReasonNotEqual
	}
}

// displayValue renders an exact value for evidence: weight fractions as
// percentages with two decimals ("0.30" -> "30.00"), counts/days as plain
// decimal strings.
func displayValue(v money.Ratio, asPercent bool) string {
	if asPercent {
		return money.QuantizeRatio(v.Mul(money.MustRatio("100")), 2).StringFixed(2)
	}
	return v.String()
}

func ratioFromInt(n int) money.Ratio {
	if n < 0 {
		n = 0
	}
	return money.MustRatio(strconv.Itoa(n))
}
