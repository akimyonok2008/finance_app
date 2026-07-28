package benchmark

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

type resolvedRecipeVersion struct {
	version    BenchmarkRecipeVersion
	components []AssetAllocation
}

// CalculateReturn is the compatibility entry point. The authoritative
// calculation is CalculateIndex, using a normalized virtual portfolio.
func (s *BenchmarkConstructionService) CalculateReturn(
	ctx context.Context,
	recipeID string,
	start, end time.Time,
	req SeriesRequirement,
) (BenchmarkReturnResult, error) {
	return s.CalculateIndex(ctx, BenchmarkEvaluationRequest{
		RecipeID: recipeID, Start: start, End: end,
		BaseCurrency: s.baseCurrency, CurrencyTreatment: CurrencyTreatmentHistoricalSpot,
		SeriesRequirement: req,
	})
}

// CalculateIndex constructs the complete virtual benchmark index. A close is
// always valued before any rebalance at that close; changed units therefore
// become effective only for the following trading interval.
func (s *BenchmarkConstructionService) CalculateIndex(
	ctx context.Context,
	request BenchmarkEvaluationRequest,
) (BenchmarkReturnResult, error) {
	if s.versions == nil {
		return BenchmarkReturnResult{}, fmt.Errorf("%w: no version store configured", ErrRecipeVersionUnavailable)
	}
	if !request.End.After(request.Start) {
		return BenchmarkReturnResult{}, fmt.Errorf("%w: end must be after start", ErrIncompleteSeries)
	}
	if strings.TrimSpace(request.BaseCurrency) == "" ||
		request.CurrencyTreatment != CurrencyTreatmentHistoricalSpot {
		return BenchmarkReturnResult{}, ErrCurrencyTreatmentUnavailable
	}
	if s.series == nil {
		return BenchmarkReturnResult{}, fmt.Errorf("%w: no price provider configured", ErrIncompleteSeries)
	}

	timeline, err := s.resolveTimeline(request.RecipeID, request.Start, request.End)
	if err != nil {
		return BenchmarkReturnResult{}, err
	}
	policy := timeline[0].version.RebalancingPolicy
	if !supportedPolicy(policy) {
		return BenchmarkReturnResult{}, fmt.Errorf("%w: %s", ErrUnsupportedRebalancingPolicy, policy)
	}
	for _, item := range timeline[1:] {
		if item.version.RebalancingPolicy != policy {
			return BenchmarkReturnResult{}, fmt.Errorf("%w: policy changed within recipe timeline", ErrUnsupportedRebalancingPolicy)
		}
	}

	symbolSet := map[string]struct{}{}
	for _, item := range timeline {
		for _, component := range item.components {
			symbolSet[component.Symbol] = struct{}{}
		}
	}
	symbols := make([]string, 0, len(symbolSet))
	for symbol := range symbolSet {
		symbols = append(symbols, symbol)
	}
	sort.Strings(symbols)

	seriesBySymbol := make(map[string]BenchmarkPriceSeries, len(symbols))
	metadata, err := s.loadSeries(ctx, symbols, request, seriesBySymbol)
	if err != nil {
		return BenchmarkReturnResult{}, err
	}
	pricesBySymbol := make(map[string][]PricePoint, len(seriesBySymbol))
	for symbol, series := range seriesBySymbol {
		for _, point := range series.Points {
			date, parseErr := time.Parse(dateLayout, point.Date)
			if parseErr == nil && !date.Before(dayUTC(request.Start)) && !date.After(dayUTC(request.End)) {
				pricesBySymbol[symbol] = append(pricesBySymbol[symbol], point)
			}
		}
	}
	dates := commonDates(pricesBySymbol)
	if len(dates) < 2 {
		return BenchmarkReturnResult{}, fmt.Errorf("%w: not enough common dates", ErrIncompleteSeries)
	}

	converted, fxEvidence, fxProvider, err := s.convertedPrices(ctx, request, dates, seriesBySymbol)
	if err != nil {
		return BenchmarkReturnResult{}, err
	}
	metadata.BaseCurrency = strings.ToUpper(request.BaseCurrency)
	metadata.CurrencyTreatment = string(request.CurrencyTreatment)
	metadata.MethodologyVersion = MethodologyVirtualPortfolioV2
	metadata.FXPoints = fxEvidence
	metadata.FXProvider = fxProvider

	firstDate, _ := time.Parse(dateLayout, dates[0])
	state := BenchmarkPortfolioState{
		NAV: 100, Holdings: map[string]float64{},
		RecipeID: request.RecipeID, RecipeVersionID: timeline[0].version.VersionID,
		RebalancingPolicy: policy, LastValuationDate: firstDate,
	}
	if err := allocateAtNAV(&state, timeline[0].components, converted, dates[0]); err != nil {
		return BenchmarkReturnResult{}, err
	}
	metadata.ActivatedVersions = append(metadata.ActivatedVersions,
		activationEvidence(timeline[0], dates[0]))
	points := []IndexPoint{{Date: dates[0], Index: 100}}

	nextVersion := 1
	for index := 1; index < len(dates); index++ {
		dateKey := dates[index]
		date, _ := time.Parse(dateLayout, dateKey)
		nav, err := valueState(state, converted, dateKey)
		if err != nil {
			return BenchmarkReturnResult{}, err
		}
		state.NAV = nav
		state.LastValuationDate = date
		points = append(points, IndexPoint{Date: dateKey, Index: nav})

		components := timeline[nextVersion-1].components
		rebalance := false
		if policy == RebalanceFilingSnapshot && nextVersion < len(timeline) &&
			date.After(dayUTC(timeline[nextVersion].version.PubliclyKnownAt)) {
			components = timeline[nextVersion].components
			state.RecipeVersionID = timeline[nextVersion].version.VersionID
			metadata.ActivatedVersions = append(metadata.ActivatedVersions,
				activationEvidence(timeline[nextVersion], dateKey))
			nextVersion++
			rebalance = true
		} else {
			switch policy {
			case RebalanceDailyTargetWeight:
				rebalance = true
			case RebalancePeriodicMonthly:
				rebalance = isFinalCommonTradingDateOfMonth(index, dates, request.End)
			case RebalanceBuyAndHold, RebalanceFilingSnapshot:
				// Units remain unchanged.
			default:
				return BenchmarkReturnResult{}, fmt.Errorf("%w: %s", ErrUnsupportedRebalancingPolicy, policy)
			}
		}
		if rebalance {
			before := state.NAV
			if err := allocateAtNAV(&state, components, converted, dateKey); err != nil {
				return BenchmarkReturnResult{}, err
			}
			after, err := valueState(state, converted, dateKey)
			if err != nil {
				return BenchmarkReturnResult{}, err
			}
			if math.Abs(before-after) > 1e-9 {
				return BenchmarkReturnResult{}, fmt.Errorf("%w: rebalance changed NAV", ErrInvalidBenchmarkSeries)
			}
			state.NAV = after
			state.LastRebalanceDate = &date
			metadata.RebalanceDates = append(metadata.RebalanceDates, dateKey)
		}
	}

	effectiveStart, _ := time.Parse(dateLayout, dates[0])
	effectiveEnd, _ := time.Parse(dateLayout, dates[len(dates)-1])
	endNAV := points[len(points)-1].Index
	result := BenchmarkReturnResult{
		ReturnPercentage: round((endNAV/100-1)*100, 4),
		EffectiveStart:   effectiveStart, EffectiveEnd: effectiveEnd,
		RecipeVersion: timeline[0].version.Metadata(),
		DataMetadata:  metadata, StartNAV: 100, EndNAV: endNAV, Points: points,
	}
	// Evidence reports the policy actually dispatched by the engine.
	result.RecipeVersion.RebalancingPolicy = policy
	result.Fingerprint = computeVirtualFingerprint(result, timeline, seriesBySymbol)
	return result, nil
}

func (s *BenchmarkConstructionService) resolveTimeline(recipeID string, start, end time.Time) ([]resolvedRecipeVersion, error) {
	versions, err := s.versions.RelevantVersions(recipeID, start, end)
	if err != nil {
		return nil, err
	}
	if versions[0].RebalancingPolicy != RebalanceFilingSnapshot {
		versions = versions[:1]
	}
	out := make([]resolvedRecipeVersion, 0, len(versions))
	for _, version := range versions {
		components, err := s.flattenConcreteVersion(version, version.PubliclyKnownAt, 1, map[string]bool{})
		if err != nil {
			return nil, err
		}
		if err := validateWeights(components); err != nil {
			return nil, fmt.Errorf("recipe %s version %s: %w", recipeID, version.VersionID, err)
		}
		out = append(out, resolvedRecipeVersion{version: version, components: components})
	}
	return out, nil
}

func (s *BenchmarkConstructionService) flattenConcreteVersion(
	version BenchmarkRecipeVersion,
	asOf time.Time,
	parentWeight float64,
	visited map[string]bool,
) ([]AssetAllocation, error) {
	if visited[version.RecipeID] {
		return nil, fmt.Errorf("%w: %s", ErrCircularRecipe, version.RecipeID)
	}
	visited[version.RecipeID] = true
	defer delete(visited, version.RecipeID)
	var result []AssetAllocation
	for _, component := range version.Components {
		weight := parentWeight * component.Weight
		switch {
		case component.Symbol != "":
			result = append(result, AssetAllocation{Symbol: component.Symbol, Weight: weight})
		case component.RecipeRef != "":
			child, err := s.versions.SelectVersion(component.RecipeRef, asOf)
			if err != nil {
				return nil, err
			}
			nested, err := s.flattenConcreteVersion(child, asOf, weight, visited)
			if err != nil {
				return nil, err
			}
			result = append(result, nested...)
		default:
			return nil, fmt.Errorf("recipe %s contains an empty component", version.RecipeID)
		}
	}
	return mergeDuplicateSymbols(result), nil
}

func (s *BenchmarkConstructionService) loadSeries(
	ctx context.Context,
	symbols []string,
	request BenchmarkEvaluationRequest,
	out map[string]BenchmarkPriceSeries,
) (BenchmarkEvaluationMetadata, error) {
	agg := BenchmarkEvaluationMetadata{
		Quality: DataQualityVerified, IncludesDividends: true, IncludesSplits: true,
		AllSeriesAdjusted: true, AllSeriesTotalReturn: true, CorpActionsKnown: true,
		EvaluatedAt: s.now(),
	}
	providers := map[string]struct{}{}
	for _, symbol := range symbols {
		series, err := s.series.GetSeries(ctx, symbol, request.Start, request.End, request.SeriesRequirement)
		if err != nil {
			return agg, fmt.Errorf("series for %s: %w", symbol, err)
		}
		quality, err := validateSeries(series, request.SeriesRequirement)
		if err != nil {
			return agg, fmt.Errorf("%s: %w", symbol, err)
		}
		if strings.TrimSpace(series.Metadata.Currency) == "" {
			return agg, fmt.Errorf("%w: %s has no declared currency", ErrCurrencyTreatmentUnavailable, symbol)
		}
		series.Metadata.Quality = quality
		out[symbol] = series
		meta := series.Metadata
		agg.Quality = worseQuality(agg.Quality, quality)
		agg.Symbols = append(agg.Symbols, symbol)
		agg.PriceTypes = append(agg.PriceTypes, meta.PriceType)
		providers[meta.Provider] = struct{}{}
		agg.IsSynthetic = agg.IsSynthetic || meta.IsSynthetic
		agg.UsedStaleData = agg.UsedStaleData || meta.IsStale
		agg.IncludesDividends = agg.IncludesDividends && meta.IncludesDividends
		agg.IncludesSplits = agg.IncludesSplits && meta.IncludesSplits
		agg.AllSeriesAdjusted = agg.AllSeriesAdjusted && meta.IsAdjusted
		agg.AllSeriesTotalReturn = agg.AllSeriesTotalReturn && meta.IsTotalReturn
		agg.CorpActionsKnown = agg.CorpActionsKnown && meta.CorpActionsKnown
	}
	for provider := range providers {
		agg.Providers = append(agg.Providers, provider)
	}
	sort.Strings(agg.Providers)
	return agg, nil
}

func (s *BenchmarkConstructionService) convertedPrices(
	ctx context.Context,
	request BenchmarkEvaluationRequest,
	dates []string,
	series map[string]BenchmarkPriceSeries,
) (map[string]map[string]float64, []FXEvidencePoint, string, error) {
	out := map[string]map[string]float64{}
	evidence := []FXEvidencePoint{}
	providerSet := map[string]struct{}{}
	base := strings.ToUpper(request.BaseCurrency)
	for symbol, item := range series {
		out[symbol] = map[string]float64{}
		native := strings.ToUpper(item.Metadata.Currency)
		priceMap := toPriceMap(item.Points)
		for _, dateKey := range dates {
			price, ok := priceMap[dateKey]
			if !ok {
				return nil, nil, "", fmt.Errorf("%w: missing %s on %s", ErrIncompleteSeries, symbol, dateKey)
			}
			date, _ := time.Parse(dateLayout, dateKey)
			rate := HistoricalFXRate{Rate: 1, Date: date, Provider: "identity"}
			if native != base {
				if s.fx == nil {
					return nil, nil, "", fmt.Errorf("%w: %s/%s on %s", ErrHistoricalFXUnavailable, native, base, dateKey)
				}
				var err error
				rate, err = s.fx.Rate(ctx, native, base, date)
				if err != nil || rate.Rate <= 0 || math.IsNaN(rate.Rate) || math.IsInf(rate.Rate, 0) {
					return nil, nil, "", fmt.Errorf("%w: %s/%s on %s", ErrHistoricalFXUnavailable, native, base, dateKey)
				}
				if !dayUTC(rate.Date).Equal(date) {
					return nil, nil, "", fmt.Errorf("%w: FX date mismatch for %s/%s", ErrHistoricalFXUnavailable, native, base)
				}
			}
			out[symbol][dateKey] = price * rate.Rate
			evidence = append(evidence, FXEvidencePoint{
				From: native, To: base, Date: dateKey, Rate: rate.Rate, Provider: rate.Provider,
			})
			providerSet[rate.Provider] = struct{}{}
		}
	}
	sort.Slice(evidence, func(i, j int) bool {
		left := evidence[i].Date + "|" + evidence[i].From + "|" + evidence[i].To + "|" + evidence[i].Provider
		right := evidence[j].Date + "|" + evidence[j].From + "|" + evidence[j].To + "|" + evidence[j].Provider
		return left < right
	})
	providers := make([]string, 0, len(providerSet))
	for provider := range providerSet {
		providers = append(providers, provider)
	}
	sort.Strings(providers)
	return out, evidence, strings.Join(providers, ","), nil
}

func allocateAtNAV(
	state *BenchmarkPortfolioState,
	components []AssetAllocation,
	prices map[string]map[string]float64,
	date string,
) error {
	holdings := make(map[string]float64, len(components))
	for _, component := range components {
		price := prices[component.Symbol][date]
		if price <= 0 {
			return fmt.Errorf("%w: invalid converted price for %s", ErrInvalidBenchmarkSeries, component.Symbol)
		}
		holdings[component.Symbol] = state.NAV * component.Weight / price
	}
	state.Holdings = holdings
	state.Cash = 0
	return nil
}

func valueState(state BenchmarkPortfolioState, prices map[string]map[string]float64, date string) (float64, error) {
	value := state.Cash
	for symbol, units := range state.Holdings {
		price, ok := prices[symbol][date]
		if !ok || price <= 0 {
			return 0, fmt.Errorf("%w: missing converted price for %s on %s", ErrIncompleteSeries, symbol, date)
		}
		value += units * price
	}
	if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, ErrInvalidBenchmarkSeries
	}
	return value, nil
}

func supportedPolicy(policy RebalancingPolicy) bool {
	switch policy {
	case RebalanceDailyTargetWeight, RebalanceBuyAndHold, RebalancePeriodicMonthly, RebalanceFilingSnapshot:
		return true
	default:
		return false
	}
}

func isFinalCommonTradingDateOfMonth(index int, dates []string, requestedEnd time.Time) bool {
	current, _ := time.Parse(dateLayout, dates[index])
	if index+1 < len(dates) {
		next, _ := time.Parse(dateLayout, dates[index+1])
		return current.Month() != next.Month() || current.Year() != next.Year()
	}
	monthEnd := time.Date(current.Year(), current.Month()+1, 0, 0, 0, 0, 0, time.UTC)
	return !dayUTC(requestedEnd).Before(monthEnd)
}

func activationEvidence(item resolvedRecipeVersion, date string) ActivatedRecipeVersion {
	components := append([]AssetAllocation(nil), item.components...)
	sort.Slice(components, func(i, j int) bool { return components[i].Symbol < components[j].Symbol })
	return ActivatedRecipeVersion{
		RecipeID: item.version.RecipeID, VersionID: item.version.VersionID,
		ActivationDate: date, Policy: item.version.RebalancingPolicy,
		Components: components, PubliclyKnownAt: item.version.PubliclyKnownAt.UTC().Format(time.RFC3339),
	}
}

func computeVirtualFingerprint(
	result BenchmarkReturnResult,
	timeline []resolvedRecipeVersion,
	series map[string]BenchmarkPriceSeries,
) string {
	var builder strings.Builder
	builder.WriteString(MethodologyVirtualPortfolioV2 + "\n")
	builder.WriteString(result.EffectiveStart.Format(dateLayout) + "|" + result.EffectiveEnd.Format(dateLayout) + "\n")
	builder.WriteString(result.DataMetadata.BaseCurrency + "|" + result.DataMetadata.CurrencyTreatment + "\n")
	builder.WriteString("policy=" + string(result.RecipeVersion.RebalancingPolicy) + "\n")
	for _, activated := range result.DataMetadata.ActivatedVersions {
		builder.WriteString("version=" + activated.RecipeID + "|" + activated.VersionID + "|" +
			activated.ActivationDate + "|" + activated.PubliclyKnownAt + "\n")
		for _, component := range activated.Components {
			builder.WriteString(component.Symbol + "=" + strconv.FormatFloat(component.Weight, 'g', 17, 64) + "\n")
		}
	}
	for _, date := range result.DataMetadata.RebalanceDates {
		builder.WriteString("rebalance=" + date + "\n")
	}
	symbols := make([]string, 0, len(series))
	for symbol := range series {
		symbols = append(symbols, symbol)
	}
	sort.Strings(symbols)
	for _, symbol := range symbols {
		item := series[symbol]
		builder.WriteString("series=" + symbol + "|" + string(item.Metadata.PriceType) + "|" +
			item.Metadata.Provider + "|" + item.Metadata.Currency + "|" + seriesPointsHash(item.Points) + "\n")
	}
	for _, fx := range result.DataMetadata.FXPoints {
		builder.WriteString("fx=" + fx.From + "|" + fx.To + "|" + fx.Date + "|" +
			strconv.FormatFloat(fx.Rate, 'g', 17, 64) + "|" + fx.Provider + "\n")
	}
	builder.WriteString("quality=" + string(result.DataMetadata.Quality) + "\n")
	sum := sha256.Sum256([]byte(builder.String()))
	return hex.EncodeToString(sum[:])
}

func seriesPointsHash(points []PricePoint) string {
	var builder strings.Builder
	for _, point := range points {
		value := point.AdjustedClose
		if value == 0 {
			value = point.RawClose
		}
		builder.WriteString(point.Date + ":" + strconv.FormatFloat(value, 'g', 17, 64) + ";")
	}
	sum := sha256.Sum256([]byte(builder.String()))
	return hex.EncodeToString(sum[:])
}

func dayUTC(value time.Time) time.Time {
	value = value.UTC()
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}
