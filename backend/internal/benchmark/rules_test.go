package benchmark

import "testing"

func testBadge(kind RuleKind, edge float64) Badge {
	return Badge{
		ID: "test", Name: "Test", Period: Period90D, RecipeID: "SPY",
		Rule: UnlockRule{Kind: kind, RequiredEdgePoints: edge, RequiresPositiveReturn: true},
	}
}

func evalCtx(kind RuleKind, edge, portfolio, benchmark float64) EvaluationContext {
	return EvaluationContext{
		Badge:              testBadge(kind, edge),
		StartDate:          "2026-01-01",
		EndDate:            "2026-04-01",
		PortfolioReturnPct: portfolio,
		BenchmarkReturnPct: benchmark,
	}
}

func TestPositiveAndBeat(t *testing.T) {
	engine := NewRulesEngine(DefaultEvaluators())

	res, err := engine.Evaluate(evalCtx(RulePositiveAndBeat, 0, 5, 3))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Unlocked || res.Evidence == nil {
		t.Errorf("expected unlock with evidence")
	}
	if res.Evidence.EdgePoints != 2 {
		t.Errorf("expected edge 2, got %v", res.Evidence.EdgePoints)
	}

	// Beats benchmark but negative portfolio → locked.
	res, _ = engine.Evaluate(evalCtx(RulePositiveAndBeat, 0, -1, -3))
	if res.Unlocked {
		t.Errorf("negative portfolio must not unlock positive_and_beat")
	}
}

func TestBeatByPoints(t *testing.T) {
	engine := NewRulesEngine(DefaultEvaluators())

	// Negative portfolio still unlocks if edge is enough (no positivity req).
	res, _ := engine.Evaluate(evalCtx(RuleBeatByPoints, 2, -1, -4))
	if !res.Unlocked {
		t.Errorf("expected unlock: edge 3 >= 2 regardless of sign")
	}

	res, _ = engine.Evaluate(evalCtx(RuleBeatByPoints, 3, 5, 3))
	if res.Unlocked {
		t.Errorf("edge 2 < 3 must not unlock")
	}
}

func TestBeatByPointsAndPositive(t *testing.T) {
	engine := NewRulesEngine(DefaultEvaluators())

	res, _ := engine.Evaluate(evalCtx(RuleBeatByPointsAndPositiv, 2, 7, 4.5))
	if !res.Unlocked {
		t.Errorf("expected unlock: positive and edge 2.5 >= 2")
	}

	// Enough edge but negative portfolio → locked.
	res, _ = engine.Evaluate(evalCtx(RuleBeatByPointsAndPositiv, 2, -1, -5))
	if res.Unlocked {
		t.Errorf("negative portfolio must not unlock")
	}
}

func TestNoEvaluatorErrors(t *testing.T) {
	engine := NewRulesEngine([]RuleEvaluator{PositiveAndBeatEvaluator{}})
	_, err := engine.Evaluate(evalCtx(RuleBeatByPoints, 2, 5, 1))
	if err == nil {
		t.Fatalf("expected error when no evaluator matches")
	}
}
