package dna

import (
	"context"
	"encoding/json"
	"testing"
)

func calc(t *testing.T, positions []PositionDNAInput) PortfolioDNA {
	t.Helper()
	result, err := NewService().Calculate(context.Background(), positions)
	if err != nil {
		t.Fatalf("Calculate returned error: %v", err)
	}
	return result
}

func TestEmptyPortfolio(t *testing.T) {
	result := calc(t, nil)
	if result.HasData {
		t.Fatalf("expected HasData false for empty portfolio")
	}
	if result.InvestmentStyle != "Not enough data" {
		t.Fatalf("unexpected style: %q", result.InvestmentStyle)
	}
	if result.Scores != (PortfolioDNAScores{}) {
		t.Fatalf("expected zero scores, got %+v", result.Scores)
	}
	if result.FocusAreas == nil {
		t.Fatalf("FocusAreas must be non-nil")
	}
}

func TestZeroAndNegativeWeightsIgnored(t *testing.T) {
	result := calc(t, []PositionDNAInput{
		{Symbol: "QQQ", AssetType: "etf", Weight: 0},
		{Symbol: "SPY", AssetType: "etf", Weight: -5},
	})
	if result.HasData {
		t.Fatalf("expected empty state when all weights are non-positive")
	}
}

func TestAllSGOV(t *testing.T) {
	result := calc(t, []PositionDNAInput{{Symbol: "SGOV", AssetType: "etf", Weight: 1}})
	s := result.Scores
	if s.Income < 85 {
		t.Errorf("expected high income, got %d", s.Income)
	}
	if s.Defensive < 70 {
		t.Errorf("expected high defensive, got %d", s.Defensive)
	}
	if s.Volatility > 25 {
		t.Errorf("expected very low volatility, got %d", s.Volatility)
	}
	if s.Growth > 10 {
		t.Errorf("expected low growth, got %d", s.Growth)
	}
	if s.Commodities != 0 {
		t.Errorf("expected zero commodities, got %d", s.Commodities)
	}
}

func TestAllQQQ(t *testing.T) {
	result := calc(t, []PositionDNAInput{{Symbol: "QQQ", AssetType: "etf", Weight: 100}})
	s := result.Scores
	if s.Growth < 80 {
		t.Errorf("expected high growth, got %d", s.Growth)
	}
	if s.Income > 15 {
		t.Errorf("expected low income, got %d", s.Income)
	}
	if s.Commodities != 0 {
		t.Errorf("expected low commodities, got %d", s.Commodities)
	}
	if s.Volatility < 50 {
		t.Errorf("expected elevated volatility, got %d", s.Volatility)
	}
}

func TestSilverUraniumThematic(t *testing.T) {
	result := calc(t, []PositionDNAInput{
		{Symbol: "SIVR", AssetType: "etf", Weight: 0.5},
		{Symbol: "URA", AssetType: "etf", Weight: 0.5},
	})
	s := result.Scores
	if s.Commodities < 80 {
		t.Errorf("expected high commodities, got %d", s.Commodities)
	}
	if s.Volatility < 70 {
		t.Errorf("expected high volatility, got %d", s.Volatility)
	}
	if s.Defensive > 40 {
		t.Errorf("expected low defensive, got %d", s.Defensive)
	}
	if result.InvestmentStyle != "High-Conviction Thematic" && result.InvestmentStyle != "Commodity-Oriented" {
		t.Errorf("expected thematic/commodity style, got %q", result.InvestmentStyle)
	}
}

func TestTenEqualWeightETFsLowConcentration(t *testing.T) {
	symbols := []string{"SPY", "VTI", "VXUS", "VEA", "VWO", "EFA", "QQQ", "IWM", "IJR", "SCHD"}
	positions := make([]PositionDNAInput, 0, len(symbols))
	for _, sym := range symbols {
		positions = append(positions, PositionDNAInput{Symbol: sym, AssetType: "etf", Weight: 10})
	}
	result := calc(t, positions)
	if result.Scores.Concentration > 30 {
		t.Errorf("expected low concentration, got %d", result.Scores.Concentration)
	}
}

func TestConcentratedSinglePosition(t *testing.T) {
	result := calc(t, []PositionDNAInput{
		{Symbol: "NVDA", AssetType: "stock", Weight: 0.7},
		{Symbol: "QQQ", AssetType: "etf", Weight: 0.3},
	})
	s := result.Scores
	if s.Concentration < 60 {
		t.Errorf("expected high concentration, got %d", s.Concentration)
	}
	if s.Growth < 65 {
		t.Errorf("expected high growth, got %d", s.Growth)
	}
	if s.Volatility < 60 {
		t.Errorf("expected high volatility, got %d", s.Volatility)
	}
	if result.InvestmentStyle != "High-Conviction Growth" {
		t.Errorf("expected High-Conviction Growth, got %q", result.InvestmentStyle)
	}
}

func TestGlobalAllocatorStyle(t *testing.T) {
	result := calc(t, []PositionDNAInput{
		{Symbol: "VXUS", AssetType: "etf", Weight: 40},
		{Symbol: "EEM", AssetType: "etf", Weight: 30},
		{Symbol: "SPY", AssetType: "etf", Weight: 30},
	})
	if result.Scores.International < 55 {
		t.Errorf("expected high international, got %d", result.Scores.International)
	}
	if result.InvestmentStyle != "Global Allocator" {
		t.Errorf("expected Global Allocator, got %q", result.InvestmentStyle)
	}
}

func TestDefensiveAdjustedDownByConcentrationAndVolatility(t *testing.T) {
	// SCHD is defensive (65). Compare diversified vs. concentrated + volatile.
	diversified := calc(t, []PositionDNAInput{
		{Symbol: "SCHD", AssetType: "etf", Weight: 25},
		{Symbol: "BND", AssetType: "etf", Weight: 25},
		{Symbol: "SGOV", AssetType: "etf", Weight: 25},
		{Symbol: "TIP", AssetType: "etf", Weight: 25},
	})
	concentratedVolatile := calc(t, []PositionDNAInput{
		{Symbol: "SCHD", AssetType: "etf", Weight: 60},
		{Symbol: "NVDA", AssetType: "stock", Weight: 40},
	})
	if concentratedVolatile.Scores.Defensive >= diversified.Scores.Defensive {
		t.Errorf("expected concentrated+volatile defensive (%d) < diversified defensive (%d)",
			concentratedVolatile.Scores.Defensive, diversified.Scores.Defensive)
	}
}

func TestVolatilityIncludesConcentration(t *testing.T) {
	// Same moderate-volatility instrument; concentration should lift volatility.
	spread := calc(t, []PositionDNAInput{
		{Symbol: "AAPL", AssetType: "stock", Weight: 20},
		{Symbol: "MSFT", AssetType: "stock", Weight: 20},
		{Symbol: "GOOGL", AssetType: "stock", Weight: 20},
		{Symbol: "AMZN", AssetType: "stock", Weight: 20},
		{Symbol: "META", AssetType: "stock", Weight: 20},
	})
	single := calc(t, []PositionDNAInput{{Symbol: "AAPL", AssetType: "stock", Weight: 100}})
	if single.Scores.Volatility <= spread.Scores.Volatility {
		t.Errorf("expected concentrated volatility (%d) > spread volatility (%d)",
			single.Scores.Volatility, spread.Scores.Volatility)
	}
}

func TestFocusAreasAggregation(t *testing.T) {
	result := calc(t, []PositionDNAInput{
		{Symbol: "QQQ", AssetType: "etf", Weight: 40},
		{Symbol: "SIVR", AssetType: "etf", Weight: 30},
		{Symbol: "URA", AssetType: "etf", Weight: 30},
	})
	if len(result.FocusAreas) == 0 {
		t.Fatalf("expected focus areas")
	}
	if len(result.FocusAreas) > maxFocusAreas {
		t.Fatalf("too many focus areas: %v", result.FocusAreas)
	}
	if !contains(result.FocusAreas, "Technology") {
		t.Errorf("expected Technology in focus areas, got %v", result.FocusAreas)
	}
}

func TestUnknownSymbolFallback(t *testing.T) {
	// Unknown non-US listed single stock should read as international.
	result := calc(t, []PositionDNAInput{
		{Symbol: "ZZZZ.L", AssetType: "stock", Currency: "GBP", Weight: 1},
	})
	if result.Scores.International < 90 {
		t.Errorf("expected international fallback, got %d", result.Scores.International)
	}

	// Unknown US stock uses the single-stock default.
	us := calc(t, []PositionDNAInput{{Symbol: "ZZZZ", AssetType: "stock", Currency: "USD", Weight: 1}})
	if us.Scores.International != 0 {
		t.Errorf("expected no international exposure for US stock, got %d", us.Scores.International)
	}
	if us.Scores.Growth == 0 {
		t.Errorf("expected non-zero growth from single-stock default")
	}
}

func TestSectorFallbackClassification(t *testing.T) {
	result := calc(t, []PositionDNAInput{
		{Symbol: "XYZ", AssetType: "etf", Sector: "Technology", Weight: 1},
	})
	if result.Scores.Growth < 70 {
		t.Errorf("expected tech-sector growth, got %d", result.Scores.Growth)
	}
}

func TestScoresWithinBounds(t *testing.T) {
	result := calc(t, []PositionDNAInput{
		{Symbol: "NVDA", AssetType: "stock", Weight: 0.9},
		{Symbol: "SIVR", AssetType: "etf", Weight: 0.1},
	})
	s := result.Scores
	for name, v := range map[string]int{
		"growth": s.Growth, "income": s.Income, "commodities": s.Commodities,
		"defensive": s.Defensive, "international": s.International,
		"concentration": s.Concentration, "volatility": s.Volatility,
	} {
		if v < 0 || v > 100 {
			t.Errorf("%s out of bounds: %d", name, v)
		}
	}
}

func TestExplanationsPresent(t *testing.T) {
	result := calc(t, []PositionDNAInput{
		{Symbol: "QQQ", AssetType: "etf", Weight: 30},
		{Symbol: "NVDA", AssetType: "stock", Weight: 30},
		{Symbol: "SGOV", AssetType: "etf", Weight: 40},
	})
	if len(result.Explanations.Growth) == 0 {
		t.Errorf("expected growth explanation")
	}
	if len(result.Explanations.Concentration) == 0 {
		t.Errorf("expected concentration explanation")
	}
}

func TestSinglePositionAddsDiversificationNote(t *testing.T) {
	result := calc(t, []PositionDNAInput{{Symbol: "AAPL", AssetType: "stock", Weight: 1}})
	found := false
	for _, line := range result.Explanations.Growth {
		if contains([]string{line}, line) && len(line) > 0 && containsSub(line, "diversified") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected diversification note for single position, got %v", result.Explanations.Growth)
	}
}

func TestDeterministic(t *testing.T) {
	positions := []PositionDNAInput{
		{Symbol: "QQQ", AssetType: "etf", Weight: 30},
		{Symbol: "NVDA", AssetType: "stock", Weight: 30},
		{Symbol: "SGOV", AssetType: "etf", Weight: 40},
	}
	a := calc(t, positions)
	b := calc(t, positions)
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if string(ja) != string(jb) {
		t.Errorf("Calculate is not deterministic:\n%s\n%s", ja, jb)
	}
}

func TestWeightNormalizationConventionAgnostic(t *testing.T) {
	// Percentages and decimals describing the same split must score identically.
	pct := calc(t, []PositionDNAInput{
		{Symbol: "QQQ", AssetType: "etf", Weight: 60},
		{Symbol: "SGOV", AssetType: "etf", Weight: 40},
	})
	dec := calc(t, []PositionDNAInput{
		{Symbol: "QQQ", AssetType: "etf", Weight: 0.6},
		{Symbol: "SGOV", AssetType: "etf", Weight: 0.4},
	})
	if pct.Scores != dec.Scores {
		t.Errorf("normalization not convention-agnostic: %+v vs %+v", pct.Scores, dec.Scores)
	}
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func containsSub(s, sub string) bool {
	return len(s) >= len(sub) && (indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
