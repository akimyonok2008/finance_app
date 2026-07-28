package benchmark

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/ardakimyonok/finance_app/internal/money"
)

// dateLayout is the canonical YYYY-MM-DD key used across the engine.
const dateLayout = "2006-01-02"

// round rounds value to the given number of decimal places.
func round(value float64, decimals int) float64 {
	factor := math.Pow(10, float64(decimals))
	return math.Round(value*factor) / factor
}

// toDateKey formats a time as a YYYY-MM-DD key (UTC).
func toDateKey(t time.Time) string {
	return t.UTC().Format(dateLayout)
}

// SubtractPeriod returns the start date for a lookback period ending at end.
func SubtractPeriod(end time.Time, period PeriodCode) (time.Time, error) {
	switch period {
	case Period30D:
		return end.AddDate(0, 0, -30), nil
	case Period90D:
		return end.AddDate(0, 0, -90), nil
	case Period6M:
		return end.AddDate(0, -6, 0), nil
	case Period1Y:
		return end.AddDate(-1, 0, 0), nil
	default:
		return time.Time{}, fmt.Errorf("unsupported period: %s", period)
	}
}

// CalculateIndexReturnPct returns the total percentage return of an index
// series (last/first - 1) * 100, rounded to 4 decimals. It sorts defensively by
// date and requires at least two points.
func CalculateIndexReturnPct(points []IndexPoint) (money.Ratio, error) {
	if len(points) < 2 {
		return money.Ratio{}, fmt.Errorf("need at least two index points, got %d", len(points))
	}
	sorted := append([]IndexPoint(nil), points...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Date < sorted[j].Date })

	first := sorted[0]
	last := sorted[len(sorted)-1]
	if first.Index.IsZero() {
		return money.Ratio{}, fmt.Errorf("first index value is zero")
	}
	ratio, err := last.Index.DivExact(first.Index, intermediatePrecision)
	if err != nil {
		return money.Ratio{}, err
	}
	return ratio.Sub(money.MustRatio("1")).Mul(money.MustRatio("100")), nil
}

// validateWeights asserts a recipe's components form a valid, fully-invested
// allocation: weights sum to 1, each is positive, and each leg names exactly one
// of symbol or recipeRef.
func validateWeights(components []AssetAllocation) error {
	total := money.ZeroWeight()
	for _, c := range components {
		total = total.Add(c.Weight)
	}
	tolerance := money.MustWeight("0.0001")
	one := money.MustWeight("1")
	deviation := total.Sub(one)
	if deviation.Sign() < 0 {
		deviation = money.ZeroWeight().Sub(deviation)
	}
	if deviation.Cmp(tolerance.Decimal) >= 0 {
		return fmt.Errorf("recipe weights must sum to 1, got %s", total.String())
	}
	for _, c := range components {
		if c.Weight.Sign() <= 0 {
			return fmt.Errorf("component weight must be positive, got %s", c.Weight.String())
		}
		hasSymbol := c.Symbol != ""
		hasRecipe := c.RecipeRef != ""
		if hasSymbol == hasRecipe {
			return fmt.Errorf("each component must have either symbol or recipeRef, not both/neither")
		}
	}
	return nil
}
