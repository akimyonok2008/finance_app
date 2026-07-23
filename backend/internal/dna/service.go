package dna

import (
	"context"
	"math"
	"sort"
	"strings"
)

// Service calculates Portfolio DNA. It holds only a Classifier and performs no
// I/O, so Calculate is deterministic and safe to call on any goroutine.
type Service struct {
	classifier Classifier
}

// NewService returns a Service backed by the curated default classifier.
func NewService() *Service {
	return &Service{classifier: newDefaultClassifier()}
}

// NewServiceWithClassifier returns a Service using a custom classifier, mainly
// for tests or future richer instrument metadata.
func NewServiceWithClassifier(c Classifier) *Service {
	if c == nil {
		c = newDefaultClassifier()
	}
	return &Service{classifier: c}
}

// classifiedPosition pairs a normalized position with its resolved factors so
// the scoring passes never re-classify.
type classifiedPosition struct {
	pos     NormalizedPosition
	factors InstrumentDNAFactors
}

// Calculate scores a portfolio from its positions. The context is accepted for
// interface symmetry; scoring itself does no I/O and never returns an error for
// well-formed input. An empty/zero-weight portfolio yields the empty DNA state.
func (s *Service) Calculate(_ context.Context, positions []PositionDNAInput) (PortfolioDNA, error) {
	normalized := normalize(positions)
	if len(normalized) == 0 {
		return emptyDNA(), nil
	}

	classified := make([]classifiedPosition, 0, len(normalized))
	weights := make([]float64, 0, len(normalized))
	for _, pos := range normalized {
		classified = append(classified, classifiedPosition{pos: pos, factors: s.classifier.Classify(pos)})
		weights = append(weights, pos.Weight)
	}

	rawGrowth := weightedAverage(classified, func(f InstrumentDNAFactors) float64 { return f.Growth })
	rawIncome := weightedAverage(classified, func(f InstrumentDNAFactors) float64 { return f.Income })
	rawCommodities := weightedAverage(classified, func(f InstrumentDNAFactors) float64 { return f.Commodities })
	rawInternational := weightedAverage(classified, func(f InstrumentDNAFactors) float64 { return f.International })
	rawDefensive := weightedAverage(classified, func(f InstrumentDNAFactors) float64 { return f.Defensive })
	rawVolatility := weightedAverage(classified, func(f InstrumentDNAFactors) float64 { return f.Volatility })

	con := concentrationDetail(weights)

	defensive := rawDefensive - 0.15*con.score - 0.10*math.Max(rawVolatility-50, 0)
	volatility := 0.85*rawVolatility + 0.15*con.score

	scores := PortfolioDNAScores{
		Growth:        clampInt(rawGrowth),
		Income:        clampInt(rawIncome),
		Commodities:   clampInt(rawCommodities),
		Defensive:     clampInt(defensive),
		International: clampInt(rawInternational),
		Concentration: clampInt(con.score),
		Volatility:    clampInt(volatility),
	}

	focusAreas := focusAreas(classified)
	style := investmentStyle(scores)
	summary := styleSummary(style, scores, focusAreas)

	return PortfolioDNA{
		InvestmentStyle: style,
		StyleSummary:    summary,
		FocusAreas:      focusAreas,
		HasData:         true,
		Scores:          scores,
		Explanations:    buildExplanations(classified, scores, con, len(normalized)),
	}, nil
}

// normalize drops non-positive weights, rescales the remainder so it sums to
// 1.0, and clamps tiny floating-point residue.
func normalize(positions []PositionDNAInput) []NormalizedPosition {
	total := 0.0
	for _, p := range positions {
		if p.Weight > 0 {
			total += p.Weight
		}
	}
	if total <= 0 {
		return nil
	}

	out := make([]NormalizedPosition, 0, len(positions))
	kept := 0.0
	for _, p := range positions {
		if p.Weight <= 0 {
			continue
		}
		w := p.Weight / total
		if w < 1e-6 { // clamp negligible dust
			continue
		}
		out = append(out, NormalizedPosition{
			Symbol:    normalizeSymbol(p.Symbol),
			Name:      strings.TrimSpace(p.Name),
			AssetType: p.AssetType,
			Sector:    p.Sector,
			Country:   p.Country,
			Currency:  p.Currency,
			Weight:    w,
		})
		kept += w
	}
	if len(out) == 0 {
		return nil
	}
	// Re-normalize after dropping dust so weights still sum to 1.0.
	if kept > 0 && math.Abs(kept-1.0) > 1e-9 {
		for i := range out {
			out[i].Weight /= kept
		}
	}
	return out
}

func weightedAverage(items []classifiedPosition, factor func(InstrumentDNAFactors) float64) float64 {
	sum := 0.0
	for _, item := range items {
		sum += item.pos.Weight * factor(item.factors)
	}
	return sum
}

// concentrationInfo carries the concentration score plus the structural inputs
// used to explain it.
type concentrationInfo struct {
	score              float64
	top1               float64
	top3               float64
	hhi                float64
	effectivePositions float64
	positionCount      int
}

// concentrationDetail computes the structural concentration score (0-100) from
// the position weights via top-1, top-3, HHI, and effective-position sub-scores.
func concentrationDetail(weights []float64) concentrationInfo {
	sorted := append([]float64(nil), weights...)
	sort.Sort(sort.Reverse(sort.Float64Slice(sorted)))

	top1 := 0.0
	if len(sorted) > 0 {
		top1 = sorted[0]
	}
	top3 := 0.0
	for i := 0; i < len(sorted) && i < 3; i++ {
		top3 += sorted[i]
	}
	hhi := 0.0
	for _, w := range weights {
		hhi += w * w
	}
	effective := 0.0
	if hhi > 0 {
		effective = 1 / hhi
	}

	top1Score := scale(top1, 0.10, 0.40)
	top3Score := scale(top3, 0.30, 0.75)
	hhiScore := scale(hhi, 0.08, 0.35)
	effectiveScore := 100 - scale(effective, 2, 12)

	score := 0.35*top1Score + 0.35*top3Score + 0.20*hhiScore + 0.10*effectiveScore

	return concentrationInfo{
		score:              clampF(score),
		top1:               top1,
		top3:               top3,
		hhi:                hhi,
		effectivePositions: effective,
		positionCount:      len(weights),
	}
}

// scale maps value linearly from [low, high] onto [0, 100], clamped.
func scale(value, low, high float64) float64 {
	if high == low {
		return 0
	}
	return clampF((value - low) / (high - low) * 100)
}

// investmentStyle selects a professional, non-judgmental label from thresholds
// over the seven scores. Rule order matters: earlier rules win.
func investmentStyle(s PortfolioDNAScores) string {
	switch {
	case s.Concentration >= 70 && s.Growth >= 65:
		return "High-Conviction Growth"
	case s.Concentration >= 70 && s.Commodities >= 45:
		return "High-Conviction Thematic"
	case s.Income >= 65 && s.Defensive >= 50:
		return "Income-Oriented"
	case s.Defensive >= 65 && s.Volatility <= 45:
		return "Defensive Allocator"
	case s.Commodities >= 60:
		return "Commodity-Oriented"
	case s.International >= 55:
		return "Global Allocator"
	case s.Growth >= 65:
		return "Growth-Oriented"
	case s.Concentration >= 70:
		return "Concentrated Opportunistic"
	case s.Concentration <= 40 && s.Volatility <= 55:
		return "Diversified Core"
	default:
		return "Balanced Allocator"
	}
}

// styleSummary produces a neutral, analytical, deterministic sentence describing
// the portfolio. It never implies advice or judgment.
func styleSummary(style string, s PortfolioDNAScores, focusAreas []string) string {
	riskLabel := "low volatility"
	switch {
	case s.Volatility >= 70:
		riskLabel = "high volatility"
	case s.Volatility >= 50:
		riskLabel = "elevated volatility"
	case s.Volatility >= 30:
		riskLabel = "moderate volatility"
	}

	concentrationLabel := "diversified"
	switch {
	case s.Concentration >= 70:
		concentrationLabel = "highly concentrated"
	case s.Concentration >= 50:
		concentrationLabel = "moderately concentrated"
	}

	lead := leadClause(s, focusAreas)
	return lead + " It is " + concentrationLabel + " and has " + riskLabel + "."
}

// leadClause builds the first, dimension-led sentence of the summary.
func leadClause(s PortfolioDNAScores, focusAreas []string) string {
	focus := ""
	if len(focusAreas) > 0 {
		focus = strings.ToLower(focusAreas[0])
	}
	switch {
	case s.Growth >= 60:
		if focus != "" {
			return "This portfolio is growth-oriented with meaningful " + focus + " exposure."
		}
		return "This portfolio is growth-oriented."
	case s.Income >= 60 && s.Defensive >= 50:
		return "This portfolio is income-oriented, with a strong defensive component."
	case s.Commodities >= 55:
		return "This portfolio is commodity-oriented, with performance likely to be influenced by precious metals, energy, or resource-related positions."
	case s.International >= 55:
		return "This portfolio is globally diversified, with meaningful exposure outside the United States."
	case s.Defensive >= 60:
		return "This portfolio is defensively positioned, favoring stability over aggressive return."
	default:
		return "This portfolio is balanced across several exposures."
	}
}

func clampF(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

func clampInt(value float64) int {
	v := int(math.Round(clampF(value)))
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}
