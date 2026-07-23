package benchmark

import "fmt"

// RuleEvaluator decides whether a badge unlocks for a given context. Each
// evaluator handles exactly one RuleKind.
type RuleEvaluator interface {
	CanEvaluate(rule UnlockRule) bool
	Evaluate(ctx EvaluationContext) EvaluationResult
}

// PositiveAndBeatEvaluator: portfolio must be positive and beat the benchmark by
// any margin.
type PositiveAndBeatEvaluator struct{}

func (PositiveAndBeatEvaluator) CanEvaluate(rule UnlockRule) bool {
	return rule.Kind == RulePositiveAndBeat
}

func (PositiveAndBeatEvaluator) Evaluate(ctx EvaluationContext) EvaluationResult {
	edge := ctx.PortfolioReturnPct - ctx.BenchmarkReturnPct
	unlocked := ctx.PortfolioReturnPct > 0 && edge > 0
	return buildResult(ctx, unlocked, edge)
}

// BeatByPointsEvaluator: portfolio must beat the benchmark by at least the
// required edge, regardless of sign.
type BeatByPointsEvaluator struct{}

func (BeatByPointsEvaluator) CanEvaluate(rule UnlockRule) bool {
	return rule.Kind == RuleBeatByPoints
}

func (BeatByPointsEvaluator) Evaluate(ctx EvaluationContext) EvaluationResult {
	edge := ctx.PortfolioReturnPct - ctx.BenchmarkReturnPct
	unlocked := edge >= ctx.Badge.Rule.RequiredEdgePoints
	return buildResult(ctx, unlocked, edge)
}

// BeatByPointsAndPositiveEvaluator: portfolio must be positive and beat the
// benchmark by at least the required edge.
type BeatByPointsAndPositiveEvaluator struct{}

func (BeatByPointsAndPositiveEvaluator) CanEvaluate(rule UnlockRule) bool {
	return rule.Kind == RuleBeatByPointsAndPositiv
}

func (BeatByPointsAndPositiveEvaluator) Evaluate(ctx EvaluationContext) EvaluationResult {
	edge := ctx.PortfolioReturnPct - ctx.BenchmarkReturnPct
	unlocked := ctx.PortfolioReturnPct > 0 && edge >= ctx.Badge.Rule.RequiredEdgePoints
	return buildResult(ctx, unlocked, edge)
}

// RulesEngine dispatches a context to the evaluator that handles its rule kind.
type RulesEngine struct {
	evaluators []RuleEvaluator
}

// NewRulesEngine wires an engine over the given evaluators.
func NewRulesEngine(evaluators []RuleEvaluator) *RulesEngine {
	return &RulesEngine{evaluators: evaluators}
}

// DefaultEvaluators returns the standard set covering every RuleKind.
func DefaultEvaluators() []RuleEvaluator {
	return []RuleEvaluator{
		PositiveAndBeatEvaluator{},
		BeatByPointsEvaluator{},
		BeatByPointsAndPositiveEvaluator{},
	}
}

// Evaluate scores a context, returning an error if no evaluator handles its rule
// kind.
func (e *RulesEngine) Evaluate(ctx EvaluationContext) (EvaluationResult, error) {
	for _, ev := range e.evaluators {
		if ev.CanEvaluate(ctx.Badge.Rule) {
			return ev.Evaluate(ctx), nil
		}
	}
	return EvaluationResult{}, fmt.Errorf("no evaluator found for rule kind %s", ctx.Badge.Rule.Kind)
}

func buildResult(ctx EvaluationContext, unlocked bool, edgePoints float64) EvaluationResult {
	roundedEdge := round(edgePoints, 4)
	if !unlocked {
		return EvaluationResult{
			Unlocked: false,
			Reason: fmt.Sprintf("Locked. Portfolio return %.4g%% vs benchmark %.4g%%, edge %.4g pts.",
				ctx.PortfolioReturnPct, ctx.BenchmarkReturnPct, roundedEdge),
		}
	}
	return EvaluationResult{
		Unlocked: true,
		Reason:   fmt.Sprintf("Unlocked. Portfolio beat benchmark by %.4g pts.", roundedEdge),
		Evidence: &AchievementEvidence{
			Period:             ctx.Badge.Period,
			StartDate:          ctx.StartDate,
			EndDate:            ctx.EndDate,
			PortfolioReturnPct: ctx.PortfolioReturnPct,
			BenchmarkReturnPct: ctx.BenchmarkReturnPct,
			EdgePoints:         roundedEdge,
			BenchmarkRecipeID:  ctx.Badge.RecipeID,
		},
	}
}
