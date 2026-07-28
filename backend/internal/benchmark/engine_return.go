package benchmark

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ardakimyonok/finance_app/internal/money"
)

// intermediatePrecision is the explicit division precision used for
// intermediate (non-persisted) ratio math in the return-construction loop —
// comfortably above the >=18dp policy floor for NAV/index values so daily
// compounding across a long window does not accumulate visible rounding
// error before the final quantization at the persistence boundary.
const intermediatePrecision = int32(30)

// CalculateReturn evaluates a benchmark over [start, end] and returns the number
// together with full provenance: the recipe version used (selected by
// no-look-ahead policy), the aggregate data quality (weakest leg), and a
// deterministic fingerprint of the inputs.
//
// req controls how strict the data must be. For a verified award the caller
// passes RequirementForAwards(); any leg that cannot satisfy it (raw close,
// synthetic, stale, incomplete) makes the call fail with a typed error so the
// award fails closed rather than silently downgrading.
func (s *BenchmarkConstructionService) CalculateReturn(ctx context.Context, recipeID string, start, end time.Time, req SeriesRequirement) (BenchmarkReturnResult, error) {
	if s.versions == nil {
		return BenchmarkReturnResult{}, fmt.Errorf("%w: no version store configured", ErrRecipeVersionUnavailable)
	}
	if !end.After(start) {
		return BenchmarkReturnResult{}, fmt.Errorf("%w: end must be after start", ErrIncompleteSeries)
	}

	// Version resolution is keyed by start: the composition the public knew at
	// the start of the window governs the whole window.
	rootVersion, err := s.versions.SelectVersion(recipeID, start)
	if err != nil {
		return BenchmarkReturnResult{}, err
	}
	flattened, err := s.flattenVersion(recipeID, start, money.MustWeight("1"), map[string]bool{})
	if err != nil {
		return BenchmarkReturnResult{}, err
	}
	if err := validateWeights(flattened); err != nil {
		return BenchmarkReturnResult{}, fmt.Errorf("recipe %s: %w", recipeID, err)
	}
	if s.series == nil {
		return BenchmarkReturnResult{}, fmt.Errorf("%w: no price provider configured", ErrIncompleteSeries)
	}

	seriesBySymbol := make(map[string]BenchmarkPriceSeries, len(flattened))
	aggQuality := DataQualityVerified
	agg := BenchmarkEvaluationMetadata{
		Quality:              DataQualityVerified,
		IncludesDividends:    true,
		IncludesSplits:       true,
		AllSeriesAdjusted:    true,
		AllSeriesTotalReturn: true,
		CorpActionsKnown:     true,
		EvaluatedAt:          s.now(),
	}
	providerSet := map[string]struct{}{}

	for _, component := range flattened {
		ser, err := s.series.GetSeries(ctx, component.Symbol, start, end, req)
		if err != nil {
			return BenchmarkReturnResult{}, fmt.Errorf("series for %s: %w", component.Symbol, err)
		}
		quality, err := validateSeries(ser, req)
		if err != nil {
			return BenchmarkReturnResult{}, fmt.Errorf("%s: %w", component.Symbol, err)
		}
		ser.Metadata.Quality = quality
		seriesBySymbol[component.Symbol] = ser

		m := ser.Metadata
		aggQuality = worseQuality(aggQuality, quality)
		agg.Symbols = append(agg.Symbols, component.Symbol)
		agg.PriceTypes = append(agg.PriceTypes, m.PriceType)
		providerSet[m.Provider] = struct{}{}
		agg.IsSynthetic = agg.IsSynthetic || m.IsSynthetic
		agg.UsedStaleData = agg.UsedStaleData || m.IsStale
		agg.IncludesDividends = agg.IncludesDividends && m.IncludesDividends
		agg.IncludesSplits = agg.IncludesSplits && m.IncludesSplits
		agg.AllSeriesAdjusted = agg.AllSeriesAdjusted && m.IsAdjusted
		agg.AllSeriesTotalReturn = agg.AllSeriesTotalReturn && m.IsTotalReturn
		agg.CorpActionsKnown = agg.CorpActionsKnown && m.CorpActionsKnown
		if agg.CurrencyTreatment == "" {
			agg.CurrencyTreatment = m.CurrencyTreatment
		} else if agg.CurrencyTreatment != m.CurrencyTreatment {
			agg.CurrencyTreatment = "mixed"
		}
	}
	agg.Quality = aggQuality
	for p := range providerSet {
		agg.Providers = append(agg.Providers, p)
	}
	sort.Strings(agg.Providers)

	// Align on common trading dates and compound daily rebalanced returns.
	pricesBySymbol := make(map[string][]PricePoint, len(seriesBySymbol))
	for sym, ser := range seriesBySymbol {
		pricesBySymbol[sym] = ser.Points
	}
	alignedDates := commonDates(pricesBySymbol)
	if len(alignedDates) < 2 {
		return BenchmarkReturnResult{}, fmt.Errorf("%w: not enough common dates", ErrIncompleteSeries)
	}
	priceMaps := make(map[string]map[string]money.Price, len(pricesBySymbol))
	for sym, series := range pricesBySymbol {
		priceMaps[sym] = toPriceMap(series)
	}
	one := money.MustRatio("1")
	index := money.MustIndexValue("100")
	for i := 1; i < len(alignedDates); i++ {
		prevDate, currDate := alignedDates[i-1], alignedDates[i]
		weightedDailyReturn := money.ZeroRatio()
		for _, component := range flattened {
			pm := priceMaps[component.Symbol]
			prev, okPrev := pm[prevDate]
			curr, okCurr := pm[currDate]
			if !okPrev || !okCurr || prev.IsZero() {
				return BenchmarkReturnResult{}, fmt.Errorf("%w: missing aligned price for %s", ErrIncompleteSeries, component.Symbol)
			}
			// componentReturn = curr/prev - 1, computed exactly at explicit
			// precision (no float64 intermediate).
			ratio, err := curr.DivExact(prev, intermediatePrecision)
			if err != nil {
				return BenchmarkReturnResult{}, fmt.Errorf("%w: %s", ErrIncompleteSeries, component.Symbol)
			}
			componentReturn := ratio.Sub(one)
			weightedDailyReturn = weightedDailyReturn.Add(component.Weight.MulRatio(componentReturn))
		}
		index = index.MulRatio(one.Add(weightedDailyReturn))
	}

	// Quantize the NAV/index only at this persistence boundary (>=18dp per
	// policy); the return percentage derives exactly from the quantized index
	// against the 100 base, so it is never itself rounded a second time.
	index = money.QuantizeIndex(index)

	effStart, _ := time.Parse(dateLayout, alignedDates[0])
	effEnd, _ := time.Parse(dateLayout, alignedDates[len(alignedDates)-1])
	result := BenchmarkReturnResult{
		ReturnPercentage: index.Sub(money.MustIndexValue("100")),
		EffectiveStart:   effStart,
		EffectiveEnd:     effEnd,
		RecipeVersion:    rootVersion.Metadata(),
		DataMetadata:     agg,
	}
	result.Fingerprint = computeFingerprint(result, flattened, seriesBySymbol)
	return result, nil
}

// flattenVersion selects the version valid at start for recipeID and expands
// nested recipeRef legs into flat symbol legs, scaling weights by the parent and
// merging duplicate symbols. visited detects circular references.
func (s *BenchmarkConstructionService) flattenVersion(recipeID string, start time.Time, parentWeight money.Weight, visited map[string]bool) ([]AssetAllocation, error) {
	if visited[recipeID] {
		return nil, fmt.Errorf("%w: %s", ErrCircularRecipe, recipeID)
	}
	visited[recipeID] = true
	defer delete(visited, recipeID)

	version, err := s.versions.SelectVersion(recipeID, start)
	if err != nil {
		return nil, err
	}
	var result []AssetAllocation
	for _, component := range version.Components {
		effectiveWeight := parentWeight.Mul(component.Weight)
		switch {
		case component.Symbol != "":
			result = append(result, AssetAllocation{Symbol: component.Symbol, Weight: effectiveWeight})
		case component.RecipeRef != "":
			child, err := s.flattenVersion(component.RecipeRef, start, effectiveWeight, visited)
			if err != nil {
				return nil, err
			}
			result = append(result, child...)
		default:
			return nil, fmt.Errorf("recipe %s has a component with neither symbol nor recipeRef", recipeID)
		}
	}
	return mergeDuplicateSymbols(result), nil
}

// computeFingerprint is a stable SHA-256 over the recipe version, effective
// window, ordered symbol weights, per-series identity (symbol + price type +
// provider + a hash of the normalized points) and aggregate quality. Identical
// inputs always produce the same hash, so an award's evaluation is reproducible
// and tamper-evident.
func computeFingerprint(r BenchmarkReturnResult, flattened []AssetAllocation, series map[string]BenchmarkPriceSeries) string {
	var b strings.Builder
	b.WriteString("v1\n")
	b.WriteString(r.RecipeVersion.RecipeID + "|" + r.RecipeVersion.VersionID + "\n")
	b.WriteString(r.EffectiveStart.Format(dateLayout) + "|" + r.EffectiveEnd.Format(dateLayout) + "\n")

	legs := append([]AssetAllocation(nil), flattened...)
	sort.Slice(legs, func(i, j int) bool { return legs[i].Symbol < legs[j].Symbol })
	for _, leg := range legs {
		// leg.Weight.String() is the canonical decimal form (money.Decimal):
		// no exponent notation, no leading/trailing zero noise, so
		// mathematically equal weights always fingerprint identically.
		b.WriteString(leg.Symbol + "=" + leg.Weight.String() + "\n")
		ser := series[leg.Symbol]
		b.WriteString(string(ser.Metadata.PriceType) + "|" + ser.Metadata.Provider + "|" + seriesPointsHash(ser.Points) + "\n")
	}
	b.WriteString("quality=" + string(r.DataMetadata.Quality) + "\n")
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

func seriesPointsHash(points []PricePoint) string {
	var b strings.Builder
	for _, p := range points {
		value := p.AdjustedClose
		if value.IsZero() {
			value = p.RawClose
		}
		b.WriteString(p.Date + ":" + value.String() + ";")
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}
