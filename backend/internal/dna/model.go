// Package dna implements a deterministic, privacy-safe Portfolio DNA scoring
// system. Given a portfolio's positions and their weights it produces seven
// explainable 0-100 scores (Growth, Income, Commodities, Defensive,
// International, Concentration, Volatility) plus an investment style, a neutral
// style summary, focus areas, and per-dimension explanations.
//
// The algorithm is fully deterministic and requires no runtime AI or external
// API calls. It is O(n log n) at worst (sorting for concentration); all other
// scoring is O(n).
package dna

// PositionDNAInput is the raw, caller-supplied description of a single holding.
// Weight is a positive magnitude (either a decimal fraction such as 0.25 or a
// percentage such as 25); the scorer normalizes weights internally so either
// convention works. Only public-safe fields are accepted — no monetary values,
// quantities, or identifiers.
type PositionDNAInput struct {
	Symbol    string
	Name      string
	AssetType string
	Sector    string
	Country   string
	Currency  string
	Weight    float64
}

// NormalizedPosition is a position whose weight has been normalized so that all
// weights across the portfolio sum to 1.0.
type NormalizedPosition struct {
	Symbol    string
	Name      string
	AssetType string
	Sector    string
	Country   string
	Currency  string
	Weight    float64 // decimal, 0.25 = 25%
}

// InstrumentDNAFactors are the per-instrument 0-100 exposure factors used to
// build weighted-average portfolio scores. Concentration is intentionally
// absent: it is a structural property of the whole portfolio, not an instrument.
type InstrumentDNAFactors struct {
	Growth        float64
	Income        float64
	Commodities   float64
	Defensive     float64
	International float64
	Volatility    float64
	FocusTags     []string
}

// PortfolioDNAScores holds the seven deterministic 0-100 DNA scores.
type PortfolioDNAScores struct {
	Growth        int `json:"growth"`
	Income        int `json:"income"`
	Commodities   int `json:"commodities"`
	Defensive     int `json:"defensive"`
	International int `json:"international"`
	Concentration int `json:"concentration"`
	Volatility    int `json:"volatility"`
}

// DNAExplanations holds short, human-readable strings describing what drove
// each dimension's score.
type DNAExplanations struct {
	Growth        []string `json:"growth"`
	Income        []string `json:"income"`
	Commodities   []string `json:"commodities"`
	Defensive     []string `json:"defensive"`
	International []string `json:"international"`
	Concentration []string `json:"concentration"`
	Volatility    []string `json:"volatility"`
}

// PortfolioDNA is the full deterministic scoring result. HasData is false when
// there were no valid positions to score, in which case the scores are zero and
// InvestmentStyle/StyleSummary carry the empty-state copy.
type PortfolioDNA struct {
	InvestmentStyle string             `json:"investment_style"`
	StyleSummary    string             `json:"style_summary"`
	FocusAreas      []string           `json:"focus_areas"`
	HasData         bool               `json:"has_data"`
	Scores          PortfolioDNAScores `json:"scores"`
	Explanations    DNAExplanations    `json:"explanations"`
}

func emptyDNA() PortfolioDNA {
	return PortfolioDNA{
		InvestmentStyle: "Not enough data",
		StyleSummary:    "Portfolio DNA will appear after positions are added.",
		FocusAreas:      []string{},
		HasData:         false,
		Scores:          PortfolioDNAScores{},
		Explanations:    emptyExplanations(),
	}
}

func emptyExplanations() DNAExplanations {
	return DNAExplanations{
		Growth:        []string{},
		Income:        []string{},
		Commodities:   []string{},
		Defensive:     []string{},
		International: []string{},
		Concentration: []string{},
		Volatility:    []string{},
	}
}
