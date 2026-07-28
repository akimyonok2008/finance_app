package performancehistory

import (
	"context"
	"errors"
	"time"
)

// BenchmarkReturn is the result of evaluating a benchmark over a requested
// window. EffectiveStart/EffectiveEnd are the ACTUAL aligned trading dates the
// benchmark could be measured between — they are almost never exactly the
// requested bounds (weekends, holidays, provider coverage), and reporting a
// benchmark difference without them would be a silent timeframe mismatch.
type BenchmarkReturn struct {
	RecipeID              string
	Name                  string
	ReturnPercentage      float64
	EffectiveStart        time.Time
	EffectiveEnd          time.Time
	Quality               string
	Synthetic             bool
	DataType              string
	TotalReturnComparable bool
	CurrencyTreatment     string
}

// BenchmarkReturner is the optional port to the benchmark construction engine.
// performancehistory deliberately does NOT import internal/benchmark: the ranked
// history service owns ranked snapshots only, and the benchmark engine stays a
// separately-owned service behind this narrow interface.
type BenchmarkReturner interface {
	ReturnOver(ctx context.Context, recipeID string, start, end time.Time) (BenchmarkReturn, error)
}

type BenchmarkUnavailableError struct {
	Reason string
}

func (e BenchmarkUnavailableError) Error() string { return e.Reason }

// DefaultBenchmarkRecipeID is the comparison benchmark shown on the Performance
// tab.
const DefaultBenchmarkRecipeID = "SPY"

// SetBenchmark wires the optional benchmark comparison. When it is not wired the
// Benchmark block reports available:false with a reason instead of a zero.
func (s *Service) SetBenchmark(b BenchmarkReturner) { s.benchmark = b }

// BenchmarkComparison is the Benchmark section of the Performance tab.
//
// The comparison is only emitted when BOTH legs are measured over the SAME
// boundary dates. The portfolio leg is re-derived from the ranked snapshots that
// fall inside the benchmark's effective window — not from the whole chart
// window — so "you beat SPY by X points" is a like-for-like statement.
type BenchmarkComparison struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
	RecipeID  string `json:"recipe_id,omitempty"`
	Name      string `json:"name,omitempty"`

	// AlignedFrom/AlignedTo are the shared boundary dates both returns use.
	AlignedFrom string `json:"aligned_from,omitempty"`
	AlignedTo   string `json:"aligned_to,omitempty"`

	BenchmarkReturnPercentage *float64 `json:"benchmark_return_percentage,omitempty"`
	// PortfolioReturnPercentage is the ranked return over the ALIGNED window. It
	// can differ from the chart's headline timeframe return, which covers the
	// full requested window; both are disclosed rather than conflated.
	PortfolioReturnPercentage  *float64 `json:"portfolio_return_percentage,omitempty"`
	DifferencePercentagePoints *float64 `json:"difference_percentage_points,omitempty"`

	DataQuality        string `json:"data_quality,omitempty"`
	DataType           string `json:"data_type"`
	DataQualityStatus  string `json:"data_quality_status"`
	CurrencyTreatment  string `json:"currency_treatment,omitempty"`
	IsSynthetic        bool   `json:"is_synthetic,omitempty"`
	VerificationStatus string `json:"verification_status"`
}

const (
	reasonBenchmarkUnavailable = "Benchmark comparison is unavailable: no benchmark price source is configured."
	reasonBenchmarkNoData      = "Benchmark comparison is unavailable: the benchmark has no usable price series for this window."
	reasonBenchmarkNoOverlap   = "Benchmark comparison is unavailable: no ranked snapshots fall inside the benchmark's trading window, so the two returns could not be measured over the same dates."
	reasonBenchmarkPriceOnly   = "Price-only benchmark data is not comparable with dividend-inclusive portfolio return."
	reasonBenchmarkCurrency    = "Benchmark comparison is unavailable: currency treatment is undefined."
)

// benchmarkComparison aligns the benchmark's own return with the portfolio's
// ranked return over identical boundary dates.
func (s *Service) benchmarkComparison(ctx context.Context, points []Snapshot) BenchmarkComparison {
	out := BenchmarkComparison{
		RecipeID: DefaultBenchmarkRecipeID, DataType: "unavailable",
		DataQualityStatus: "unavailable", VerificationStatus: "unavailable",
	}
	if s.benchmark == nil {
		out.RecipeID = ""
		out.Reason = reasonBenchmarkUnavailable
		return out
	}
	if len(points) < 2 {
		out.Reason = reasonBenchmarkNoData
		return out
	}

	result, err := s.benchmark.ReturnOver(
		ctx, DefaultBenchmarkRecipeID,
		points[0].CapturedAt.UTC(), points[len(points)-1].CapturedAt.UTC(),
	)
	if err != nil {
		var unavailable BenchmarkUnavailableError
		if errors.As(err, &unavailable) && unavailable.Reason != "" {
			out.Reason = unavailable.Reason
		} else {
			out.Reason = reasonBenchmarkNoData
		}
		return out
	}
	out.Name = result.Name
	out.DataQuality = result.Quality
	out.IsSynthetic = result.Synthetic
	out.DataType = result.DataType
	out.CurrencyTreatment = result.CurrencyTreatment
	// Older in-process implementations predate explicit provenance. Preserve
	// their contract while every production adapter now supplies these fields.
	if out.DataType == "" {
		out.DataType = "adjusted_close"
		result.TotalReturnComparable = true
	}
	if out.CurrencyTreatment == "" && result.DataType == "" {
		out.CurrencyTreatment = "legacy_same_currency"
	}
	if !result.TotalReturnComparable {
		out.DataQualityStatus = "insufficient_for_total_return_comparison"
		out.Reason = reasonBenchmarkPriceOnly
		return out
	}
	if out.CurrencyTreatment == "" {
		out.DataQualityStatus = "currency_treatment_undefined"
		out.Reason = reasonBenchmarkCurrency
		return out
	}

	// The benchmark's effective window is a pair of trading DAYS. Take the last
	// ranked snapshot at or before the end of each boundary day so both legs
	// describe the same calendar interval.
	from := result.EffectiveStart.UTC()
	to := result.EffectiveEnd.UTC()
	comparison, err := CompareWithBenchmarkCloses(
		points, from, to, result.ReturnPercentage,
	)
	if err != nil {
		out.Reason = reasonBenchmarkNoOverlap
		return out
	}

	benchmarkReturn := round4(result.ReturnPercentage)
	portfolio := round4(comparison.PortfolioReturnPercentage)
	difference := round4(comparison.EdgePercentagePoints)
	out.Available = true
	out.VerificationStatus = "preview"
	if result.Quality == "verified" && !result.Synthetic {
		out.VerificationStatus = "verified"
	}
	out.DataQualityStatus = "sufficient_for_comparison"
	out.AlignedFrom = from.Format("2006-01-02")
	out.AlignedTo = to.Format("2006-01-02")
	out.BenchmarkReturnPercentage = &benchmarkReturn
	out.PortfolioReturnPercentage = &portfolio
	out.DifferencePercentagePoints = &difference
	return out
}
