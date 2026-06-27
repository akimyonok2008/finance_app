package coach

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// MockProvider produces deterministic, analysis-only narrative from the computed
// facts. It works with no external API key and is the default provider. It does
// not fabricate market data: where data is missing it says so, echoing the
// data_limitations carried in the input.
type MockProvider struct{}

// NewMockProvider constructs the key-free local provider.
func NewMockProvider() *MockProvider { return &MockProvider{} }

// GeneratePortfolioCoachAnalysis implements Provider deterministically.
func (MockProvider) GeneratePortfolioCoachAnalysis(_ context.Context, in CoachProviderInput) (CoachProviderOutput, error) {
	out := CoachProviderOutput{
		Title:               titleFor(in.Mode),
		RiskLevel:           in.User.RiskLevel,
		Observations:        baseObservations(in),
		LearningPoints:      baseLearningPoints(in),
		QuestionsToConsider: baseQuestions(in),
	}
	out.Summary = summaryFor(in)

	switch in.Mode {
	case ModeTechnicalSetup:
		out.TechnicalNotes = technicalNotes(in)
	case ModeFundamentalContext:
		out.FundamentalNotes = fundamentalNotes(in)
	case ModeCompareTop10:
		out.Observations = append(out.Observations, comparisonObservations(in)...)
	}

	// Surface any data limitations as plain, honest notes.
	for _, lim := range in.DataLimitations {
		out.TechnicalNotes = appendUnique(out.TechnicalNotes, "Data note: "+lim)
	}

	return out, nil
}

func titleFor(mode string) string {
	switch mode {
	case ModeCompareTop10:
		return "Compared with the Top 10"
	case ModeTechnicalSetup:
		return "Technical Analysis (Prototype-Limited)"
	case ModeFundamentalContext:
		return "Fundamental Analysis (Limited)"
	default:
		return "Portfolio Review"
	}
}

func summaryFor(in CoachProviderInput) string {
	u := in.User
	base := fmt.Sprintf(
		"Your portfolio index is %.2f (%+.2f%%) across %d position(s). Your largest holding, %s, is %.2f%% of the portfolio, which reads as %s concentration.",
		u.PortfolioIndex, u.GainLossPercentage, u.PositionCount, safeSymbol(u.LargestSymbol), u.LargestWeightPercentage, u.RiskLevel,
	)
	switch in.Mode {
	case ModeCompareTop10:
		if in.Top10.Available {
			return base + fmt.Sprintf(" Compared with the top performers, your return gap is %+.2f percentage points and you share %d symbol(s) with them.",
				in.Top10.ReturnGapPercentagePoints, in.Top10.SharedSymbolsCount)
		}
		return base + " A top-performer benchmark is not available yet, so this is a portfolio-only view."
	default:
		return base
	}
}

func baseObservations(in CoachProviderInput) []CoachObservation {
	u := in.User
	obs := []CoachObservation{
		{
			Label:  "Concentration",
			Status: concentrationStatus(u.LargestWeightPercentage, u.PositionCount),
			Text: fmt.Sprintf("Your largest holding weight is %.2f%% across %d position(s). Higher concentration can amplify both gains and losses.",
				u.LargestWeightPercentage, u.PositionCount),
		},
		{
			Label:  "Asset mix",
			Status: "neutral",
			Text:   "Asset-type exposure: " + exposureText(u.AssetTypeExposure) + ".",
		},
		{
			Label:  "Currency exposure",
			Status: currencyStatus(u.CurrencyExposure),
			Text:   "Currency exposure: " + exposureText(u.CurrencyExposure) + ". Non-base currencies add FX sensitivity.",
		},
	}

	if top := topContributor(u.Holdings); top != nil {
		obs = append(obs, CoachObservation{
			Label:  "Performance attribution",
			Status: "neutral",
			Text: fmt.Sprintf("%s contributes the most to your return at %+.2f percentage points; this suggests your performance is concentrated in a few names.",
				top.Symbol, top.ContributionPercentagePoints),
		})
	}
	return obs
}

func comparisonObservations(in CoachProviderInput) []CoachObservation {
	if !in.Top10.Available {
		return []CoachObservation{{
			Label:  "Benchmark",
			Status: "neutral",
			Text:   "A top-10 benchmark is not available yet; more participants with positions are needed.",
		}}
	}
	t := in.Top10
	gapStatus := "neutral"
	if t.ReturnGapPercentagePoints < 0 {
		gapStatus = "watch"
	} else if t.ReturnGapPercentagePoints > 0 {
		gapStatus = "positive"
	}
	return []CoachObservation{
		{
			Label:  "Return gap",
			Status: gapStatus,
			Text: fmt.Sprintf("Compared with the top performers (sample of %d), your return gap is %+.2f percentage points.",
				t.SampleSize, t.ReturnGapPercentagePoints),
		},
		{
			Label:  "Concentration vs top 10",
			Status: weightCompareStatus(t.UserLargestWeightPercentage, t.MedianLargestWeightPercentage),
			Text: fmt.Sprintf("Your largest weight is %.2f%% versus a top-10 median of %.2f%%. This is a structural difference, not a recommendation.",
				t.UserLargestWeightPercentage, t.MedianLargestWeightPercentage),
		},
		{
			Label:  "Overlap",
			Status: "neutral",
			Text: fmt.Sprintf("You share %d symbol(s) with the top performers. Overlap does not mean those portfolios should be copied.",
				t.SharedSymbolsCount),
		},
	}
}

func technicalNotes(in CoachProviderInput) []string {
	notes := []string{
		"Prototype price data does not include history, so true trend, moving averages, RSI, MACD, and support/resistance cannot be computed.",
	}
	for _, h := range topHoldings(in.User.Holdings, 3) {
		notes = append(notes, fmt.Sprintf("%s: current return %+.2f%%, weight %.2f%%. This may indicate momentum is concentrated in your larger positions.",
			h.Symbol, h.GainLossPercentage, h.WeightPercentage))
	}
	return notes
}

func fundamentalNotes(in CoachProviderInput) []string {
	notes := []string{
		"No fundamental data source is connected, so company financials are not available; the following is broad context only.",
		"Asset-type tilt: " + exposureText(in.User.AssetTypeExposure) + ".",
	}
	if in.User.AssetTypeExposure["crypto"] > 0 {
		notes = append(notes, "Crypto exposure tends to carry higher volatility and macro sensitivity, which is a risk to watch.")
	}
	return notes
}

func baseLearningPoints(in CoachProviderInput) []string {
	points := []string{
		"High concentration can increase both upside and downside.",
		"Currency exposure introduces FX risk that is separate from asset performance.",
	}
	if in.Mode == ModeCompareTop10 {
		points = append(points, "Top-performer overlap does not mean those portfolios should be copied.")
	}
	return points
}

func baseQuestions(in CoachProviderInput) []string {
	q := []string{
		fmt.Sprintf("Is your largest holding weight of %.2f%% intentional?", in.User.LargestWeightPercentage),
		"Are you comfortable with the currency exposure in your portfolio?",
	}
	return q
}

// --- small helpers -----------------------------------------------------------

func concentrationStatus(largestWeight float64, positionCount int) string {
	if positionCount <= 1 || largestWeight >= 50 {
		return "risk"
	}
	if largestWeight >= 35 {
		return "watch"
	}
	return "neutral"
}

func currencyStatus(exposure map[string]float64) string {
	// More than one currency means some FX sensitivity worth noting.
	if len(exposure) > 1 {
		return "watch"
	}
	return "neutral"
}

func weightCompareStatus(userWeight, medianWeight float64) string {
	if userWeight > medianWeight+10 {
		return "watch"
	}
	return "neutral"
}

func exposureText(exposure map[string]float64) string {
	if len(exposure) == 0 {
		return "none"
	}
	keys := make([]string, 0, len(exposure))
	for k := range exposure {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s %.2f%%", k, exposure[k]))
	}
	return strings.Join(parts, ", ")
}

func topContributor(holdings []HoldingFact) *HoldingFact {
	if len(holdings) == 0 {
		return nil
	}
	best := holdings[0]
	for _, h := range holdings[1:] {
		if h.ContributionPercentagePoints > best.ContributionPercentagePoints {
			best = h
		}
	}
	return &best
}

func topHoldings(holdings []HoldingFact, n int) []HoldingFact {
	sorted := make([]HoldingFact, len(holdings))
	copy(sorted, holdings)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].WeightPercentage > sorted[j].WeightPercentage
	})
	if len(sorted) > n {
		sorted = sorted[:n]
	}
	return sorted
}

func safeSymbol(s string) string {
	if s == "" {
		return "n/a"
	}
	return s
}

func appendUnique(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}
