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
