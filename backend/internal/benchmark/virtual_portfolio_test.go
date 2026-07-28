package benchmark

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/money"
)

type tableSeriesProvider struct {
	points   map[string][]PricePoint
	currency map[string]string
}

func (p tableSeriesProvider) GetAdjustedCloseSeries(_ context.Context, symbol string, _, _ time.Time) ([]PricePoint, error) {
	points, ok := p.points[symbol]
	if !ok {
		return nil, fmt.Errorf("missing %s", symbol)
	}
	return append([]PricePoint(nil), points...), nil
}

func (p tableSeriesProvider) GetSeries(ctx context.Context, symbol string, start, end time.Time, _ SeriesRequirement) (BenchmarkPriceSeries, error) {
	points, err := p.GetAdjustedCloseSeries(ctx, symbol, start, end)
	if err != nil {
		return BenchmarkPriceSeries{}, err
	}
	return BenchmarkPriceSeries{
		Symbol: symbol, Points: points,
		Metadata: BenchmarkDataMetadata{
			Provider: "unit-feed", ProviderMode: "real", PriceType: PriceTypeTotalReturn,
			IncludesDividends: true, IncludesSplits: true, IsAdjusted: true,
			IsTotalReturn: true, CorpActionsKnown: true, Quality: DataQualityVerified,
			Currency: p.currency[symbol],
		},
	}, nil
}

type tableFXProvider struct {
	rates map[string]float64
}

func (p tableFXProvider) Rate(_ context.Context, from, to string, date time.Time) (HistoricalFXRate, error) {
	key := from + "/" + to + "/" + date.Format(dateLayout)
	rate, ok := p.rates[key]
	if !ok {
		return HistoricalFXRate{}, ErrHistoricalFXUnavailable
	}
	return HistoricalFXRate{Rate: rate, Date: date, Provider: "historical-fx-test"}, nil
}

func virtualEngine(t *testing.T, policy RebalancingPolicy, points map[string][]PricePoint, components []AssetAllocation) *BenchmarkConstructionService {
	t.Helper()
	version := BenchmarkRecipeVersion{
		RecipeID: "TEST", VersionID: "TEST_v1", Name: "Test",
		PubliclyKnownAt: epoch, EffectiveFrom: epoch, SourceType: "static_model",
		RebalancingPolicy: policy, TotalReturnModel: true, Components: components,
	}
	store, err := NewVersionedRecipeStore([]BenchmarkRecipeVersion{version})
	require.NoError(t, err)
	currencies := map[string]string{}
	for symbol := range points {
		currencies[symbol] = "USD"
	}
	provider := tableSeriesProvider{points: points, currency: currencies}
	engine := NewBenchmarkConstructionService(provider, map[string]BenchmarkRecipe{}, nil)
	engine.SetVersionStore(store)
	engine.SetClock(func() time.Time { return mustTime("2026-01-10") })
	return engine
}

func pp(date string, value float64) PricePoint {
	return PricePoint{Date: date, AdjustedClose: money.PriceFromFloat64(value)}
}

func evaluateVirtual(t *testing.T, engine *BenchmarkConstructionService, start, end string) BenchmarkReturnResult {
	t.Helper()
	result, err := engine.CalculateIndex(context.Background(), BenchmarkEvaluationRequest{
		RecipeID: "TEST", Start: mustTime(start), End: mustTime(end),
		BaseCurrency: "USD", CurrencyTreatment: CurrencyTreatmentHistoricalSpot,
		SeriesRequirement: RequirementForAwards(),
	})
	require.NoError(t, err)
	return result
}

func TestVirtualPortfolio_SingleAssetMatchesEndpointForEveryPolicy(t *testing.T) {
	points := map[string][]PricePoint{"SPY": {
		pp("2026-01-02", 100), pp("2026-01-05", 110), pp("2026-01-06", 121),
	}}
	for _, policy := range []RebalancingPolicy{
		RebalanceBuyAndHold, RebalanceDailyTargetWeight,
		RebalancePeriodicMonthly, RebalanceFilingSnapshot,
	} {
		t.Run(string(policy), func(t *testing.T) {
			engine := virtualEngine(t, policy, points, []AssetAllocation{{Symbol: "SPY", Weight: money.MustWeight("1")}})
			result := evaluateVirtual(t, engine, "2026-01-02", "2026-01-06")
			assert.InDelta(t, 21, result.ReturnPercentage.Float64(), 0.0001)
			assert.Equal(t, "100", result.StartNAV.String())
			assert.InDelta(t, 121, result.EndNAV.Float64(), 1e-9)
		})
	}
}

func TestVirtualPortfolio_BuyAndHoldDiffersFromDailyTargetWeightRegression(t *testing.T) {
	points := map[string][]PricePoint{
		"A": {pp("2026-01-02", 100), pp("2026-01-05", 200), pp("2026-01-06", 100)},
		"B": {pp("2026-01-02", 100), pp("2026-01-05", 50), pp("2026-01-06", 100)},
	}
	components := []AssetAllocation{{Symbol: "A", Weight: money.MustWeight("0.5")}, {Symbol: "B", Weight: money.MustWeight("0.5")}}
	hold := evaluateVirtual(t, virtualEngine(t, RebalanceBuyAndHold, points, components), "2026-01-02", "2026-01-06")
	daily := evaluateVirtual(t, virtualEngine(t, RebalanceDailyTargetWeight, points, components), "2026-01-02", "2026-01-06")

	assert.InDelta(t, 0, hold.ReturnPercentage.Float64(), 0.0001)
	assert.InDelta(t, 56.25, daily.ReturnPercentage.Float64(), 0.0001)
	assert.Empty(t, hold.DataMetadata.RebalanceDates)
	assert.Equal(t, []string{"2026-01-05", "2026-01-06"}, daily.DataMetadata.RebalanceDates)
	assert.NotEqual(t, hold.Fingerprint, daily.Fingerprint)
}

func TestVirtualPortfolio_InitialUnitsAndRebalancePreserveNAV(t *testing.T) {
	state := BenchmarkPortfolioState{NAV: money.MustIndexValue("100"), Cash: money.ZeroAmount(), Holdings: map[string]money.Quantity{}}
	prices := map[string]map[string]money.Price{
		"A": {"2026-01-02": money.MustPrice("20")}, "B": {"2026-01-02": money.MustPrice("10")},
	}
	components := []AssetAllocation{{Symbol: "A", Weight: money.MustWeight("0.6")}, {Symbol: "B", Weight: money.MustWeight("0.4")}}
	require.NoError(t, allocateAtNAV(&state, components, prices, "2026-01-02"))
	assert.InDelta(t, 3, state.Holdings["A"].Float64(), 1e-12)
	assert.InDelta(t, 4, state.Holdings["B"].Float64(), 1e-12)
	nav, err := valueState(state, prices, "2026-01-02")
	require.NoError(t, err)
	assert.InDelta(t, 100, nav.Float64(), 1e-12)
}

func TestVirtualPortfolio_MonthlyUsesFinalCommonTradingDate(t *testing.T) {
	points := map[string][]PricePoint{
		"A": {pp("2026-01-02", 100), pp("2026-01-30", 120), pp("2026-02-02", 130)},
		"B": {pp("2026-01-02", 100), pp("2026-01-30", 80), pp("2026-02-02", 70)},
	}
	engine := virtualEngine(t, RebalancePeriodicMonthly, points,
		[]AssetAllocation{{Symbol: "A", Weight: money.MustWeight("0.5")}, {Symbol: "B", Weight: money.MustWeight("0.5")}})
	result := evaluateVirtual(t, engine, "2026-01-02", "2026-02-02")
	assert.Equal(t, []string{"2026-01-30"}, result.DataMetadata.RebalanceDates)
	assert.InDelta(t, 100, result.Points[1].Index.Float64(), 1e-9)
}

func TestVirtualPortfolio_FilingUsesConservativeNextTradingDate(t *testing.T) {
	versionA := BenchmarkRecipeVersion{
		RecipeID: "TEST", VersionID: "A", PubliclyKnownAt: mustTime("2026-01-01"),
		EffectiveFrom: mustTime("2026-01-01"), SourceType: "sec_13f_hr",
		SourceURL: "https://example.test/a", SourceAccession: "a",
		ReportPeriodEnd: timePointer(mustTime("2025-09-30")), MappingCoverage: floatPointer(1),
		RebalancingPolicy: RebalanceFilingSnapshot,
		Components:        []AssetAllocation{{Symbol: "A", Weight: money.MustWeight("1")}},
	}
	versionB := versionA
	versionB.VersionID = "B"
	versionB.PubliclyKnownAt = mustTime("2026-01-04")
	versionB.EffectiveFrom = versionB.PubliclyKnownAt
	versionB.SourceAccession = "b"
	versionB.ReportPeriodEnd = timePointer(mustTime("2025-12-31"))
	versionB.Components = []AssetAllocation{{Symbol: "B", Weight: money.MustWeight("1")}}
	store, err := NewVersionedRecipeStore([]BenchmarkRecipeVersion{versionA, versionB})
	require.NoError(t, err)
	provider := tableSeriesProvider{
		points: map[string][]PricePoint{
			"A": {pp("2026-01-02", 100), pp("2026-01-04", 110), pp("2026-01-05", 120), pp("2026-01-06", 120)},
			"B": {pp("2026-01-02", 100), pp("2026-01-04", 50), pp("2026-01-05", 50), pp("2026-01-06", 100)},
		},
		currency: map[string]string{"A": "USD", "B": "USD"},
	}
	engine := NewBenchmarkConstructionService(provider, nil, nil)
	engine.SetVersionStore(store)
	result := evaluateVirtual(t, engine, "2026-01-02", "2026-01-06")

	require.Len(t, result.DataMetadata.ActivatedVersions, 2)
	assert.Equal(t, "2026-01-05", result.DataMetadata.ActivatedVersions[1].ActivationDate)
	assert.Equal(t, []string{"2026-01-05"}, result.DataMetadata.RebalanceDates)
	assert.InDelta(t, 240, result.EndNAV.Float64(), 1e-9) // A reaches 120, then B doubles.
}

func TestVirtualPortfolio_HistoricalFXChangesResultAndFingerprint(t *testing.T) {
	provider := tableSeriesProvider{
		points:   map[string][]PricePoint{"EU": {pp("2026-01-02", 100), pp("2026-01-05", 100)}},
		currency: map[string]string{"EU": "EUR"},
	}
	engine := virtualEngine(t, RebalanceBuyAndHold, provider.points,
		[]AssetAllocation{{Symbol: "EU", Weight: money.MustWeight("1")}})
	engine.series = provider
	engine.SetHistoricalFXProvider(tableFXProvider{rates: map[string]float64{
		"EUR/USD/2026-01-02": 1, "EUR/USD/2026-01-05": 1.1,
	}})
	first := evaluateVirtual(t, engine, "2026-01-02", "2026-01-05")
	assert.InDelta(t, 10, first.ReturnPercentage.Float64(), 0.0001)

	engine.SetHistoricalFXProvider(tableFXProvider{rates: map[string]float64{
		"EUR/USD/2026-01-02": 1, "EUR/USD/2026-01-05": 1.2,
	}})
	second := evaluateVirtual(t, engine, "2026-01-02", "2026-01-05")
	assert.InDelta(t, 20, second.ReturnPercentage.Float64(), 0.0001)
	assert.NotEqual(t, first.Fingerprint, second.Fingerprint)
}

func TestVirtualPortfolio_MissingHistoricalFXFailsClosed(t *testing.T) {
	provider := tableSeriesProvider{
		points:   map[string][]PricePoint{"EU": {pp("2026-01-02", 100), pp("2026-01-05", 101)}},
		currency: map[string]string{"EU": "EUR"},
	}
	engine := virtualEngine(t, RebalanceBuyAndHold, provider.points,
		[]AssetAllocation{{Symbol: "EU", Weight: money.MustWeight("1")}})
	engine.series = provider
	_, err := engine.CalculateReturn(context.Background(), "TEST",
		mustTime("2026-01-02"), mustTime("2026-01-05"), RequirementForAwards())
	assert.ErrorIs(t, err, ErrHistoricalFXUnavailable)
}

func TestVirtualPortfolio_UnsupportedPolicyFailsExplicitly(t *testing.T) {
	engine := virtualEngine(t, RebalancingPolicy("surprise"), map[string][]PricePoint{
		"A": {pp("2026-01-02", 100), pp("2026-01-05", 101)},
	}, []AssetAllocation{{Symbol: "A", Weight: money.MustWeight("1")}})
	_, err := engine.CalculateReturn(context.Background(), "TEST",
		mustTime("2026-01-02"), mustTime("2026-01-05"), RequirementForAwards())
	assert.ErrorIs(t, err, ErrUnsupportedRebalancingPolicy)
}

func TestVirtualPortfolio_DeterministicPointsReturnAndFingerprint(t *testing.T) {
	engine := virtualEngine(t, RebalanceBuyAndHold, map[string][]PricePoint{
		"A": {pp("2026-01-02", 100), pp("2026-01-05", 105)},
	}, []AssetAllocation{{Symbol: "A", Weight: money.MustWeight("1")}})
	first := evaluateVirtual(t, engine, "2026-01-02", "2026-01-05")
	second := evaluateVirtual(t, engine, "2026-01-02", "2026-01-05")
	assert.Equal(t, first.Points, second.Points)
	assert.Equal(t, first.ReturnPercentage, second.ReturnPercentage)
	assert.Equal(t, first.Fingerprint, second.Fingerprint)
}

func timePointer(value time.Time) *time.Time { return &value }
func floatPointer(value float64) *float64    { return &value }
