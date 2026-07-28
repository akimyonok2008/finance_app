package benchmark

import (
	"errors"
	"math"
	"strconv"
	"testing"

	"github.com/ardakimyonok/finance_app/internal/money"
)

func seriesReturnPct(pts []PricePoint) float64 {
	ratio, err := pts[len(pts)-1].AdjustedClose.DivExact(pts[0].AdjustedClose, intermediatePrecision)
	if err != nil {
		panic(err)
	}
	pct := ratio.Sub(money.MustRatio("1")).Mul(money.MustRatio("100"))
	f, err := strconv.ParseFloat(pct.String(), 64)
	if err != nil {
		panic(err)
	}
	return f
}

// TestSplitDoesNotCreateFalseReturn: a 2-for-1 split halves the raw price but is
// economically neutral. Raw close shows -50%; the adjusted series must show 0%.
func TestSplitDoesNotCreateFalseReturn(t *testing.T) {
	raw := []PricePoint{
		{Date: "2026-01-02", AdjustedClose: money.MustPrice("100")},
		{Date: "2026-01-05", AdjustedClose: money.MustPrice("50")}, // post 2-for-1 split
	}
	if got := seriesReturnPct(raw); math.Abs(got-(-50)) > 1e-9 {
		t.Fatalf("raw return should be -50%%, got %.4f", got)
	}
	adj, err := BuildTotalReturnSeries("AAPL", raw,
		[]CorporateAction{{Date: "2026-01-05", SplitRatio: money.MustRatio("2")}},
		BenchmarkDataMetadata{Provider: "test"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if got := seriesReturnPct(adj.Points); math.Abs(got) > 1e-9 {
		t.Fatalf("split-adjusted economic return must be 0%%, got %.4f", got)
	}
}

// TestDividendChangesTotalReturn: start 100, end 98, a $5 dividend. Raw price
// return is -2%; total return is positive (reinvestment-adjusted ~+3.16%). The
// two must not be treated as equal.
func TestDividendChangesTotalReturn(t *testing.T) {
	raw := []PricePoint{
		{Date: "2026-01-02", AdjustedClose: money.MustPrice("100")},
		{Date: "2026-02-02", AdjustedClose: money.MustPrice("98")},
	}
	rawReturn := seriesReturnPct(raw)
	if math.Abs(rawReturn-(-2)) > 1e-9 {
		t.Fatalf("raw return should be -2%%, got %.4f", rawReturn)
	}
	adj, err := BuildTotalReturnSeries("SCHD", raw,
		[]CorporateAction{{Date: "2026-02-02", CashDividend: money.MustPrice("5")}},
		BenchmarkDataMetadata{Provider: "test"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	totalReturn := seriesReturnPct(adj.Points)
	if totalReturn <= 0 {
		t.Fatalf("total return with a dividend must be positive, got %.4f", totalReturn)
	}
	if math.Abs(totalReturn-rawReturn) < 1 {
		t.Fatalf("raw (%.4f) and total (%.4f) return must differ materially", rawReturn, totalReturn)
	}
	// Reinvestment-adjusted: 98 / (100*(1-5/100)) - 1 = 98/95 - 1 ≈ 3.16%.
	if math.Abs(totalReturn-3.1578) > 0.01 {
		t.Fatalf("expected total return ~+3.16%%, got %.4f", totalReturn)
	}
	if !adj.Metadata.IsTotalReturn || !adj.Metadata.CorpActionsKnown {
		t.Fatalf("constructed series must be labelled total-return with known corp actions")
	}
}

func adjustedMeta() BenchmarkDataMetadata {
	return BenchmarkDataMetadata{
		Provider: "feed", ProviderMode: "real", PriceType: PriceTypeAdjustedClose,
		IsAdjusted: true, IsTotalReturn: true, IncludesDividends: true, IncludesSplits: true,
		CorpActionsKnown: true, Quality: DataQualityVerified,
	}
}

func rawMeta() BenchmarkDataMetadata {
	return BenchmarkDataMetadata{
		Provider: "feed", ProviderMode: "real", PriceType: PriceTypeRawClose,
		CorpActionsKnown: false, Quality: DataQualityAcceptable,
	}
}

func twoPoints() []PricePoint {
	return []PricePoint{
		{Date: "2026-01-02", AdjustedClose: money.MustPrice("100")},
		{Date: "2026-01-05", AdjustedClose: money.MustPrice("101")},
	}
}

func TestValidateSeries_AdjustedAcceptedForAwards(t *testing.T) {
	q, err := validateSeries(BenchmarkPriceSeries{Symbol: "SPY", Points: twoPoints(), Metadata: adjustedMeta()}, RequirementForAwards())
	if err != nil {
		t.Fatalf("adjusted series should satisfy award requirement: %v", err)
	}
	if q != DataQualityVerified {
		t.Fatalf("expected verified quality, got %s", q)
	}
}

func TestValidateSeries_RawRejectedWhenAdjustedRequired(t *testing.T) {
	_, err := validateSeries(BenchmarkPriceSeries{Symbol: "SPY", Points: twoPoints(), Metadata: rawMeta()}, RequirementForAwards())
	if !errors.Is(err, ErrAdjustedDataUnavailable) {
		t.Fatalf("expected ErrAdjustedDataUnavailable, got %v", err)
	}
}

func TestValidateSeries_RawAcceptedForPreview(t *testing.T) {
	q, err := validateSeries(BenchmarkPriceSeries{Symbol: "SPY", Points: twoPoints(), Metadata: rawMeta()}, RequirementForPreview())
	if err != nil {
		t.Fatalf("raw series should be usable for preview: %v", err)
	}
	if q != DataQualityAcceptable {
		t.Fatalf("expected acceptable quality, got %s", q)
	}
}

func TestValidateSeries_SyntheticRejectedForAwards(t *testing.T) {
	meta := adjustedMeta()
	meta.IsSynthetic = true
	meta.Quality = DataQualitySynthetic
	_, err := validateSeries(BenchmarkPriceSeries{Symbol: "SPY", Points: twoPoints(), Metadata: meta}, RequirementForAwards())
	if !errors.Is(err, ErrSyntheticDataNotAllowed) {
		t.Fatalf("expected ErrSyntheticDataNotAllowed, got %v", err)
	}
}

func TestValidateSeries_StaleRejectedForAwards(t *testing.T) {
	meta := adjustedMeta()
	meta.IsStale = true
	_, err := validateSeries(BenchmarkPriceSeries{Symbol: "SPY", Points: twoPoints(), Metadata: meta}, RequirementForAwards())
	if !errors.Is(err, ErrStaleBenchmarkData) {
		t.Fatalf("expected ErrStaleBenchmarkData, got %v", err)
	}
}

func TestValidateSeries_RejectsBadValues(t *testing.T) {
	// NaN/Inf cases are no longer representable: money.Price parsing rejects
	// non-finite values at construction, so validateSeries can never observe
	// one — that invariant is now enforced structurally rather than at
	// validation time.
	cases := map[string][]PricePoint{
		"too few":   {{Date: "2026-01-02", AdjustedClose: money.MustPrice("100")}},
		"negative":  {{Date: "2026-01-02", AdjustedClose: money.MustPrice("100")}, {Date: "2026-01-05", AdjustedClose: money.MustPrice("-1")}},
		"zero":      {{Date: "2026-01-02", AdjustedClose: money.MustPrice("100")}, {Date: "2026-01-05", AdjustedClose: money.MustPrice("0")}},
		"unsorted":  {{Date: "2026-01-05", AdjustedClose: money.MustPrice("100")}, {Date: "2026-01-02", AdjustedClose: money.MustPrice("101")}},
		"duplicate": {{Date: "2026-01-02", AdjustedClose: money.MustPrice("100")}, {Date: "2026-01-02", AdjustedClose: money.MustPrice("101")}},
		"bad date":  {{Date: "nope", AdjustedClose: money.MustPrice("100")}, {Date: "2026-01-05", AdjustedClose: money.MustPrice("101")}},
	}
	for name, pts := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := validateSeries(BenchmarkPriceSeries{Symbol: "X", Points: pts, Metadata: adjustedMeta()}, RequirementForPreview())
			if err == nil {
				t.Fatalf("expected rejection for %s", name)
			}
		})
	}
}

func TestWorseQuality(t *testing.T) {
	if worseQuality(DataQualityVerified, DataQualitySynthetic) != DataQualitySynthetic {
		t.Fatal("synthetic must dominate verified")
	}
	if worseQuality(DataQualityAcceptable, DataQualityVerified) != DataQualityAcceptable {
		t.Fatal("acceptable is weaker than verified")
	}
	if worseQuality(DataQualityStale, DataQualityInvalid) != DataQualityInvalid {
		t.Fatal("invalid is weakest")
	}
}
