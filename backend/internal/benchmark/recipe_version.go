package benchmark

import (
	"fmt"
	"sort"
	"time"

	"github.com/ardakimyonok/finance_app/internal/money"
)

// MinMappingCoverage is the minimum fraction of a source portfolio's market
// value that must map to supported tickers for a dynamic (13F-style) recipe
// version to be usable for a verified award. Below this, the version fails
// closed with ErrRecipeMappingCoverage.
const MinMappingCoverage = 0.90

// BenchmarkRecipeVersion is an immutable, dated composition of a recipe. Static
// recipes get a single _v1 version; dynamic recipes (Berkshire 13F) get one
// version per authoritative filing. Selection is by PubliclyKnownAt so a
// historical evaluation never uses holdings the public could not have known.
type BenchmarkRecipeVersion struct {
	RecipeID          string            `json:"recipe_id"`
	VersionID         string            `json:"version_id"`
	Name              string            `json:"name"`
	Description       string            `json:"description"`
	ReportPeriodEnd   *time.Time        `json:"report_period_end,omitempty"`
	FiledAt           *time.Time        `json:"filed_at,omitempty"`
	PubliclyKnownAt   time.Time         `json:"publicly_known_at"`
	EffectiveFrom     time.Time         `json:"effective_from"`
	EffectiveUntil    *time.Time        `json:"effective_until,omitempty"`
	Components        []AssetAllocation `json:"components"`
	SourceType        string            `json:"source_type"` // static_model | sec_13f_hr
	SourceURL         string            `json:"source_url,omitempty"`
	SourceAccession   string            `json:"source_accession,omitempty"`
	SourceDescription string            `json:"source_description,omitempty"`
	RebalancingPolicy RebalancingPolicy `json:"rebalancing_policy"`
	TotalReturnModel  bool              `json:"total_return_model"`
	InvestorProxy     bool              `json:"investor_proxy"`

	// Dynamic-recipe coverage accounting (13F mapping).
	SourceMarketValueTotal float64   `json:"source_market_value_total,omitempty"`
	MappedMarketValueTotal float64   `json:"mapped_market_value_total,omitempty"`
	MappingCoverage        *float64  `json:"mapping_coverage_percentage,omitempty"`
	ExcludedPositionsCount int       `json:"excluded_positions_count,omitempty"`
	CreatedAt              time.Time `json:"created_at"`
}

// Recipe projects a version back into the flat BenchmarkRecipe the engine's math
// consumes.
func (v BenchmarkRecipeVersion) Recipe() BenchmarkRecipe {
	return BenchmarkRecipe{ID: v.RecipeID, Name: v.Name, Description: v.Description, Components: v.Components}
}

// Metadata projects the version's provenance for evidence.
func (v BenchmarkRecipeVersion) Metadata() RecipeVersionMetadata {
	return RecipeVersionMetadata{
		RecipeID:          v.RecipeID,
		VersionID:         v.VersionID,
		SourceType:        v.SourceType,
		SourceURL:         v.SourceURL,
		SourceAccession:   v.SourceAccession,
		ReportPeriodEnd:   v.ReportPeriodEnd,
		PubliclyKnownAt:   timePtr(v.PubliclyKnownAt),
		RebalancingPolicy: v.RebalancingPolicy,
		TotalReturnModel:  v.TotalReturnModel,
		InvestorProxy:     v.InvestorProxy,
		MappingCoverage:   v.MappingCoverage,
	}
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	tt := t
	return &tt
}

// Validate checks a version's internal integrity. Weights must be positive and
// sum to 1; externally sourced (13F) versions require source metadata and must
// meet the mapping-coverage threshold.
func (v BenchmarkRecipeVersion) Validate() error {
	if v.VersionID == "" || v.RecipeID == "" {
		return fmt.Errorf("%w: version missing recipe_id/version_id", ErrInvalidBenchmarkSeries)
	}
	if len(v.Components) == 0 {
		return fmt.Errorf("recipe version %s has no components", v.VersionID)
	}
	total := money.ZeroWeight()
	tolerance := money.MustWeight("0.0001")
	one := money.MustWeight("1")
	for _, c := range v.Components {
		if c.Weight.Sign() <= 0 {
			return fmt.Errorf("recipe version %s: non-positive/non-finite weight", v.VersionID)
		}
		if (c.Symbol == "") == (c.RecipeRef == "") {
			return fmt.Errorf("recipe version %s: each component needs exactly one of symbol/recipeRef", v.VersionID)
		}
		total = total.Add(c.Weight)
	}
	deviation := total.Sub(one)
	if deviation.Sign() < 0 {
		deviation = money.ZeroWeight().Sub(deviation)
	}
	if deviation.Cmp(tolerance.Decimal) >= 0 {
		return fmt.Errorf("recipe version %s: weights must sum to 1, got %s", v.VersionID, total.String())
	}
	if v.EffectiveUntil != nil && !v.EffectiveUntil.After(v.EffectiveFrom) {
		return fmt.Errorf("recipe version %s: effective_until must be after effective_from", v.VersionID)
	}
	if v.SourceType == "sec_13f_hr" {
		if v.SourceAccession == "" || v.SourceURL == "" || v.ReportPeriodEnd == nil {
			return fmt.Errorf("recipe version %s: 13F version requires accession/url/report period", v.VersionID)
		}
		if v.MappingCoverage != nil && *v.MappingCoverage < MinMappingCoverage {
			return fmt.Errorf("%w: version %s coverage %.4f < %.2f", ErrRecipeMappingCoverage, v.VersionID, *v.MappingCoverage, MinMappingCoverage)
		}
	}
	return nil
}

// VersionedRecipeStore holds all recipe versions and resolves the one valid for
// a given evaluation start date.
type VersionedRecipeStore struct {
	versions map[string][]BenchmarkRecipeVersion // recipeID -> versions, ascending by PubliclyKnownAt
}

// NewVersionedRecipeStore builds a store, validating every version and rejecting
// overlapping effective ranges and duplicate version IDs.
func NewVersionedRecipeStore(versions []BenchmarkRecipeVersion) (*VersionedRecipeStore, error) {
	byRecipe := map[string][]BenchmarkRecipeVersion{}
	seen := map[string]struct{}{}
	for _, v := range versions {
		if err := v.Validate(); err != nil {
			return nil, err
		}
		key := v.RecipeID + "/" + v.VersionID
		if _, dup := seen[key]; dup {
			return nil, fmt.Errorf("duplicate recipe version id %s", key)
		}
		seen[key] = struct{}{}
		byRecipe[v.RecipeID] = append(byRecipe[v.RecipeID], v)
	}
	for id, vs := range byRecipe {
		sort.Slice(vs, func(i, j int) bool { return vs[i].PubliclyKnownAt.Before(vs[j].PubliclyKnownAt) })
		// Reject overlapping effective windows within a recipe.
		for i := 1; i < len(vs); i++ {
			prev := vs[i-1]
			if prev.EffectiveUntil == nil {
				// An open-ended earlier version must be closed before the next begins.
				if !vs[i].EffectiveFrom.Before(prev.EffectiveFrom) && prev.PubliclyKnownAt.Equal(vs[i].PubliclyKnownAt) {
					return nil, fmt.Errorf("recipe %s: overlapping versions %s/%s", id, prev.VersionID, vs[i].VersionID)
				}
				continue
			}
			if vs[i].EffectiveFrom.Before(*prev.EffectiveUntil) {
				return nil, fmt.Errorf("recipe %s: overlapping effective ranges %s/%s", id, prev.VersionID, vs[i].VersionID)
			}
		}
		byRecipe[id] = vs
	}
	return &VersionedRecipeStore{versions: byRecipe}, nil
}

// SelectVersion returns the newest version whose PubliclyKnownAt is at or before
// the evaluation start. This is the no-look-ahead policy: the whole [start,end]
// window uses the composition the public knew at start.
func (s *VersionedRecipeStore) SelectVersion(recipeID string, start time.Time) (BenchmarkRecipeVersion, error) {
	vs, ok := s.versions[recipeID]
	if !ok || len(vs) == 0 {
		return BenchmarkRecipeVersion{}, fmt.Errorf("%w: %s", ErrRecipeVersionUnavailable, recipeID)
	}
	var chosen *BenchmarkRecipeVersion
	for i := range vs {
		if !vs[i].PubliclyKnownAt.After(start) {
			chosen = &vs[i]
		}
	}
	if chosen == nil {
		return BenchmarkRecipeVersion{}, fmt.Errorf("%w: %s at %s", ErrRecipeVersionUnavailable, recipeID, start.Format(dateLayout))
	}
	return *chosen, nil
}

// Versions returns all versions of a recipe (for admin/audit).
func (s *VersionedRecipeStore) Versions(recipeID string) []BenchmarkRecipeVersion {
	return append([]BenchmarkRecipeVersion(nil), s.versions[recipeID]...)
}

// RelevantVersions returns the version knowable at the requested start plus
// every later version made public during the window. Callers decide whether the
// recipe policy activates those transitions; static policies intentionally use
// only the first version.
func (s *VersionedRecipeStore) RelevantVersions(recipeID string, start, end time.Time) ([]BenchmarkRecipeVersion, error) {
	first, err := s.SelectVersion(recipeID, start)
	if err != nil {
		return nil, err
	}
	out := []BenchmarkRecipeVersion{first}
	for _, version := range s.versions[recipeID] {
		if version.VersionID == first.VersionID {
			continue
		}
		if version.PubliclyKnownAt.After(start) && !version.PubliclyKnownAt.After(end) {
			out = append(out, version)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].PubliclyKnownAt.Equal(out[j].PubliclyKnownAt) {
			return out[i].VersionID < out[j].VersionID
		}
		return out[i].PubliclyKnownAt.Before(out[j].PubliclyKnownAt)
	})
	return out, nil
}

// epoch is the "always valid" public date for static, code-defined recipes,
// well before any evaluated window.
var epoch = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

// DefaultVersionStore builds the store from the static catalogue plus the
// authoritative Berkshire 13F versions. Static recipes become <ID>_v1 versions
// valid from the epoch; the dynamic BUFFETT_13F recipe is replaced by its dated
// filing versions.
func DefaultVersionStore(recipes map[string]BenchmarkRecipe) (*VersionedRecipeStore, error) {
	var versions []BenchmarkRecipeVersion
	for id, r := range recipes {
		if id == "BUFFETT_13F" || r.Dynamic {
			continue // dynamic recipes are versioned from filings below
		}
		versions = append(versions, BenchmarkRecipeVersion{
			RecipeID:          id,
			VersionID:         id + "_v1",
			Name:              r.Name,
			Description:       r.Description,
			PubliclyKnownAt:   epoch,
			EffectiveFrom:     epoch,
			Components:        r.Components,
			SourceType:        "static_model",
			SourceDescription: "Code-defined proxy allocation.",
			RebalancingPolicy: RebalanceDailyTargetWeight,
			TotalReturnModel:  true,
			InvestorProxy:     isInvestorProxy(id),
			CreatedAt:         epoch,
		})
	}
	versions = append(versions, berkshireVersions()...)
	return NewVersionedRecipeStore(versions)
}

// isInvestorProxy marks recipes that are inspired by a named investor rather
// than being a plain index — used to label badges as proxy portfolios.
func isInvestorProxy(recipeID string) bool {
	switch recipeID {
	case "ORACLE_BERKSHIRE", "MUNGER_QUALITY", "GRAHAM_DEFENSIVE_VALUE", "LYNCH_GROWTH",
		"SWENSEN_ENDOWMENT", "SOROS_MACRO", "QUANT_STYLE", "DRUCKENMILLER_MACRO_GROWTH", "ALL_WEATHER":
		return true
	default:
		return false
	}
}

func dateUTC(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// berkshireVersions returns the authoritative Berkshire Hathaway 13F equity
// baskets, one immutable version per SEC filing. Weights are market-value
// weights among the reliably-mapped large-cap positions, renormalized to sum to
// 1. Data is transcribed from the SEC EDGAR 13F-HR information tables
// (CIK 0001067983); see backend/README.md for the source policy.
func berkshireVersions() []BenchmarkRecipeVersion {
	cov2025 := 0.9267
	rpe2025 := dateUTC(2025, 3, 31)
	filed2025 := dateUTC(2025, 5, 15)
	cov2026 := 0.9681
	rpe2026 := dateUTC(2026, 3, 31)
	filed2026 := dateUTC(2026, 5, 15)

	return []BenchmarkRecipeVersion{
		{
			RecipeID:        "BUFFETT_13F",
			VersionID:       "BUFFETT_13F_2025Q1",
			Name:            "Berkshire 13F Equity Basket (2025 Q1)",
			Description:     "Berkshire Hathaway's disclosed public-equity basket from the 2025 Q1 13F-HR, mapped large-cap positions, market-value weighted.",
			ReportPeriodEnd: &rpe2025,
			FiledAt:         &filed2025,
			PubliclyKnownAt: filed2025,
			EffectiveFrom:   filed2025,
			EffectiveUntil:  &filed2026,
			Components: normalizeWeights([]AssetAllocation{
				{Symbol: "AAPL", Weight: money.MustWeight("0.277953")},
				{Symbol: "AXP", Weight: money.MustWeight("0.170140")},
				{Symbol: "KO", Weight: money.MustWeight("0.119492")},
				{Symbol: "BAC", Weight: money.MustWeight("0.109930")},
				{Symbol: "CVX", Weight: money.MustWeight("0.082763")},
				{Symbol: "OXY", Weight: money.MustWeight("0.054547")},
				{Symbol: "MCO", Weight: money.MustWeight("0.047919")},
				{Symbol: "KHC", Weight: money.MustWeight("0.041331")},
				{Symbol: "CB", Weight: money.MustWeight("0.034052")},
				{Symbol: "DVA", Weight: money.MustWeight("0.022422")},
				{Symbol: "VRSN", Weight: money.MustWeight("0.014073")},
				{Symbol: "KR", Weight: money.MustWeight("0.014117")},
				{Symbol: "SIRI", Weight: money.MustWeight("0.011263")},
			}),
			SourceType:             "sec_13f_hr",
			SourceURL:              "https://www.sec.gov/Archives/edgar/data/1067983/000095012325005701/form13fInfoTable.xml",
			SourceAccession:        "0000950123-25-005701",
			SourceDescription:      "Berkshire Hathaway Inc. 13F-HR for period 2025-03-31.",
			RebalancingPolicy:      RebalanceFilingSnapshot,
			TotalReturnModel:       true,
			InvestorProxy:          true,
			SourceMarketValueTotal: 258701144516,
			MappedMarketValueTotal: 239749268777,
			MappingCoverage:        &cov2025,
			ExcludedPositionsCount: 23,
			CreatedAt:              filed2025,
		},
		{
			RecipeID:        "BUFFETT_13F",
			VersionID:       "BUFFETT_13F_2026Q1",
			Name:            "Berkshire 13F Equity Basket (2026 Q1)",
			Description:     "Berkshire Hathaway's disclosed public-equity basket from the 2026 Q1 13F-HR, mapped large-cap positions, market-value weighted.",
			ReportPeriodEnd: &rpe2026,
			FiledAt:         &filed2026,
			PubliclyKnownAt: filed2026,
			EffectiveFrom:   filed2026,
			Components: normalizeWeights([]AssetAllocation{
				{Symbol: "AAPL", Weight: money.MustWeight("0.227110")},
				{Symbol: "AXP", Weight: money.MustWeight("0.180057")},
				{Symbol: "KO", Weight: money.MustWeight("0.119438")},
				{Symbol: "BAC", Weight: money.MustWeight("0.098311")},
				{Symbol: "CVX", Weight: money.MustWeight("0.068543")},
				{Symbol: "OXY", Weight: money.MustWeight("0.067616")},
				{Symbol: "GOOGL", Weight: money.MustWeight("0.061251")},
				{Symbol: "CB", Weight: money.MustWeight("0.043829")},
				{Symbol: "MCO", Weight: money.MustWeight("0.042256")},
				{Symbol: "KHC", Weight: money.MustWeight("0.028754")},
				{Symbol: "DVA", Weight: money.MustWeight("0.018164")},
				{Symbol: "KR", Weight: money.MustWeight("0.014205")},
				{Symbol: "SIRI", Weight: money.MustWeight("0.011310")},
				{Symbol: "DAL", Weight: money.MustWeight("0.010391")},
				{Symbol: "VRSN", Weight: money.MustWeight("0.008766")},
			}),
			SourceType:             "sec_13f_hr",
			SourceURL:              "https://www.sec.gov/Archives/edgar/data/1067983/000119312526226661/53405.xml",
			SourceAccession:        "0001193125-26-226661",
			SourceDescription:      "Berkshire Hathaway Inc. 13F-HR for period 2026-03-31.",
			RebalancingPolicy:      RebalanceFilingSnapshot,
			TotalReturnModel:       true,
			InvestorProxy:          true,
			SourceMarketValueTotal: 263095703570,
			MappedMarketValueTotal: 254692792933,
			MappingCoverage:        &cov2026,
			ExcludedPositionsCount: 14,
			CreatedAt:              filed2026,
		},
	}
}

// normalizeWeights rescales positive weights to sum to exactly 1.
func normalizeWeights(raw []AssetAllocation) []AssetAllocation {
	total := money.ZeroWeight()
	for _, c := range raw {
		if c.Weight.Sign() > 0 {
			total = total.Add(c.Weight)
		}
	}
	out := make([]AssetAllocation, 0, len(raw))
	if total.Sign() <= 0 {
		return out
	}
	for _, c := range raw {
		if c.Weight.Sign() <= 0 {
			continue
		}
		normalized, err := c.Weight.DivExact(total, intermediatePrecision)
		if err != nil {
			continue
		}
		out = append(out, AssetAllocation{Symbol: c.Symbol, RecipeRef: c.RecipeRef, Weight: normalized})
	}
	return out
}
