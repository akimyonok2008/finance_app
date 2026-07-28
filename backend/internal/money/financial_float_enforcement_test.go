package money_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestCoreDomainsDoNotAddAuthoritativeFloatFields is intentionally targeted:
// presentation/statistical projections and provider/cache boundary values are
// documented exceptions, while financial-looking fields in domain state fail.
func TestCoreDomainsDoNotAddAuthoritativeFloatFields(t *testing.T) {
	_, here, _, _ := runtime.Caller(0)
	internal := filepath.Dir(filepath.Dir(here))
	packages := []string{"portfolio", "performance", "performancehistory", "benchmark", "income", "leaderboard", "competitions"}
	financialName := func(name string) bool {
		n := strings.ToLower(name)
		for _, word := range []string{"amount", "quantity", "price", "value", "basis", "pnl", "index", "return", "fee", "weight", "score"} {
			if strings.Contains(n, word) {
				return true
			}
		}
		return false
	}
	presentationType := func(name string) bool {
		for _, suffix := range []string{"View", "Response", "Preview", "Summary", "Metrics", "Analysis", "Point", "Result", "Entry", "Milestone", "Risk", "Comparison"} {
			if strings.Contains(name, suffix) {
				return true
			}
		}
		return false
	}
	specificExceptions := map[string]string{
		"portfolio.Position.ClosePrice":                            "legacy read/presentation close-price projection",
		"portfolio.Position.RealizedGainLossBase":                  "legacy presentation projection; authoritative activity ledger is decimal",
		"portfolio.Position.RealizedGainLossPercentage":            "presentation percentage",
		"portfolio.StrategyWeightInput.WeightPercentage":           "non-authoritative strategy preference",
		"portfolio.mutationPlan.buyCashUsed":                       "legacy response-only preview accumulator",
		"portfolio.mutationPlan.buyFunding":                        "legacy response-only preview accumulator",
		"portfolio.EconomicAttribution.RealizedPnLBase":            "presentation attribution projection",
		"portfolio.EconomicAttribution.UnrealizedPnLBase":          "presentation attribution projection",
		"portfolio.EconomicAttribution.StandaloneFeesBase":         "presentation attribution projection",
		"portfolio.InstrumentEconomics.UnrealizedPnLBase":          "presentation attribution input after exact ledger calculation",
		"portfolio.InstrumentEconomics.RealizedPnLBase":            "presentation attribution input after exact ledger calculation",
		"portfolio.InstrumentEconomics.FeesBase":                   "presentation attribution input after exact ledger calculation",
		"portfolio.InstrumentContribution.WeightPercentage":        "presentation percentage",
		"portfolio.PerformanceAttribution.UnrealizedPnLBase":       "legacy archive presentation projection",
		"portfolio.PerformanceAttribution.RealizedPnLBase":         "legacy archive presentation projection",
		"portfolio.PerformanceAttribution.FeesBase":                "legacy archive presentation projection",
		"portfolio.PortfolioValuation.OpenHoldingsMarketValueBase": "legacy archive projection; live authoritative valuation uses money.Amount",
		"portfolio.PortfolioValuation.CashValueBase":               "legacy archive projection; live authoritative valuation uses money.Amount",
		"portfolio.PortfolioValuation.CurrentPortfolioValueBase":   "legacy archive projection; live authoritative valuation uses money.Amount",
		"portfolio.PortfolioArchiveSnapshot.PortfolioIndex":        "legacy archive response/store predating ranked snapshots",
		"portfolio.PortfolioArchiveSnapshot.TotalCostBasis":        "legacy archive response/store",
		"portfolio.PortfolioArchiveSnapshot.CurrentValue":          "legacy archive response/store",
		"portfolio.PortfolioArchiveSnapshot.TotalCashValueBase":    "legacy archive response/store",
		"performancehistory.Window.HistoryCoverage":                "statistical coverage ratio",
		"performancehistory.Window.ActiveCoverage":                 "statistical coverage ratio",
		"performancehistory.Window.TrustedCoverage":                "statistical coverage ratio",
		"performancehistory.BenchmarkReturn.ReturnPercentage":      "external benchmark-provider presentation boundary",
		"performancehistory.PeriodReturn.ReturnPercentage":         "risk/statistical presentation series",
		"performancehistory.indexPoint.index":                      "chart/risk library boundary after exact snapshot conversion",
		"benchmark.AchievementEvidence.PortfolioReturnPct":         "achievement presentation evidence",
		"benchmark.AchievementEvidence.BenchmarkReturnPct":         "achievement presentation evidence",
		"benchmark.AchievementEvidence.StartRankedIndex":           "achievement presentation evidence",
		"benchmark.AchievementEvidence.EndRankedIndex":             "achievement presentation evidence",
		"benchmark.EvaluationContext.PortfolioReturnPct":           "rule-engine presentation input",
		"benchmark.EvaluationContext.BenchmarkReturnPct":           "rule-engine presentation input",
		"benchmark.BenchmarkRecipeVersion.SourceMarketValueTotal":  "provider mapping diagnostic",
		"benchmark.BenchmarkRecipeVersion.MappedMarketValueTotal":  "provider mapping diagnostic",
		"leaderboard.CachedLeaderboardScore.Score":                 "Redis IEEE-754 read boundary",
		"leaderboard.PublicWeight.WeightPercentage":                "public presentation percentage",
		"leaderboard.UserRanking.RankedReturnPercentage":           "profile presentation projection after exact ordering",
		"leaderboard.UserRanking.RankedIndex":                      "profile presentation projection after exact ordering",
		"leaderboard.Standing.RankedReturnPercentage":              "HTTP presentation projection after exact ordering",
		"leaderboard.Standing.RankedIndex":                         "HTTP presentation projection after exact ordering",
		"leaderboard.Milestone.ReturnGapPercentage":                "HTTP presentation percentage",
	}

	for _, pkg := range packages {
		dir := filepath.Join(internal, pkg)
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range files {
			if strings.HasSuffix(file, "_test.go") {
				continue
			}
			parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
			if err != nil {
				t.Fatalf("parse %s: %v", file, err)
			}
			for _, decl := range parsed.Decls {
				gen, ok := decl.(*ast.GenDecl)
				if !ok {
					continue
				}
				for _, spec := range gen.Specs {
					typeSpec, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					structType, ok := typeSpec.Type.(*ast.StructType)
					if !ok {
						continue
					}
					for _, field := range structType.Fields.List {
						ident, ok := field.Type.(*ast.Ident)
						if !ok || (ident.Name != "float64" && ident.Name != "float32") {
							continue
						}
						for _, fieldName := range field.Names {
							if !financialName(fieldName.Name) {
								continue
							}
							key := pkg + "." + typeSpec.Name.Name + "." + fieldName.Name
							if _, ok := specificExceptions[key]; ok {
								continue
							}
							if presentationType(typeSpec.Name.Name) && field.Tag != nil && strings.Contains(field.Tag.Value, "json:") {
								continue
							}
							t.Errorf("%s uses %s for financial-looking field; use internal/money or add a documented exception", key, ident.Name)
						}
					}
				}
			}
		}
	}
}
