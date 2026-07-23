package strategy

import "github.com/ardakimyonok/finance_app/internal/portfolio"

const CopyDisclaimer = "This copies public strategy weights only. No trades are executed."

type SourceProfile struct {
	Handle      string `json:"handle"`
	DisplayName string `json:"display_name"`
	AvatarKey   string `json:"avatar_key"`
	StrategyTag string `json:"strategy_tag"`
}

type Weight struct {
	Symbol           string  `json:"symbol"`
	AssetType        string  `json:"asset_type"`
	WeightPercentage float64 `json:"weight_percentage"`
}

type CopyPreviewRequest struct {
	Handle string `json:"handle"`
}

type CopyPreviewResponse struct {
	SourceProfile SourceProfile `json:"source_profile"`
	Weights       []Weight      `json:"weights"`
	Disclaimer    string        `json:"disclaimer"`
}

type CopyFromProfileRequest struct {
	Handle  string   `json:"handle"`
	Weights []Weight `json:"weights"`
}

type CopyFromProfileResponse struct {
	SourceProfile SourceProfile `json:"source_profile"`
	Weights       []Weight      `json:"weights"`
	Disclaimer    string        `json:"disclaimer"`
}

type CompareProfileRequest struct {
	Handle string `json:"handle"`
}

type WeightDifference struct {
	Symbol                 string  `json:"symbol"`
	MyWeightPercentage     float64 `json:"my_weight_percentage"`
	TargetWeightPercentage float64 `json:"target_weight_percentage"`
	DifferencePercentage   float64 `json:"difference_percentage"`
	AssetType              string  `json:"asset_type"`
}

type ConcentrationComparison struct {
	MyPositionCount            int     `json:"my_position_count"`
	TargetPositionCount        int     `json:"target_position_count"`
	MyTop3WeightPercentage     float64 `json:"my_top3_weight_percentage"`
	TargetTop3WeightPercentage float64 `json:"target_top3_weight_percentage"`
}

type LearningPoint struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type CompareProfileResponse struct {
	TargetProfile           SourceProfile           `json:"target_profile"`
	Summary                 string                  `json:"summary"`
	OverlapScore            float64                 `json:"overlap_score"`
	SharedSymbols           []string                `json:"shared_symbols"`
	WeightDifferences       []WeightDifference      `json:"weight_differences"`
	ConcentrationComparison ConcentrationComparison `json:"concentration_comparison"`
	LearningPoints          []LearningPoint         `json:"learning_points"`
	Disclaimer              string                  `json:"disclaimer"`
}

func toPortfolioWeights(weights []Weight) []portfolio.StrategyWeightInput {
	out := make([]portfolio.StrategyWeightInput, 0, len(weights))
	for _, w := range weights {
		out = append(out, portfolio.StrategyWeightInput{
			Symbol:           w.Symbol,
			AssetType:        w.AssetType,
			WeightPercentage: w.WeightPercentage,
		})
	}
	return out
}
