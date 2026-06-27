package profile

import "time"

const DefaultStrategyTag = "balanced_global"

type Profile struct {
	UserID            string
	Handle            string
	DisplayName       string
	AvatarKey         string
	Bio               string
	StrategyTag       string
	IsPublic          bool
	ShowPublicWeights bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type UpdateInput struct {
	Handle            *string `json:"handle"`
	DisplayName       *string `json:"display_name"`
	AvatarKey         *string `json:"avatar_key"`
	Bio               *string `json:"bio"`
	StrategyTag       *string `json:"strategy_tag"`
	IsPublic          *bool   `json:"is_public"`
	ShowPublicWeights *bool   `json:"show_public_weights"`
}

type PublicBadge struct {
	Key        string     `json:"key"`
	Name       string     `json:"name"`
	Icon       string     `json:"icon"`
	UnlockedAt *time.Time `json:"unlocked_at,omitempty"`
}

type PublicWeight struct {
	Symbol    string  `json:"symbol"`
	AssetType string  `json:"asset_type"`
	Weight    float64 `json:"weight"`
}

type Exposure struct {
	Name   string  `json:"name"`
	Weight float64 `json:"weight"`
}

type Concentration struct {
	LargestPosition float64 `json:"largest_position"`
	TopThree        float64 `json:"top_three"`
}

type PublicProfile struct {
	Handle            string         `json:"handle"`
	DisplayName       string         `json:"display_name"`
	AvatarKey         string         `json:"avatar_key"`
	Bio               string         `json:"bio"`
	StrategyTag       string         `json:"strategy_tag"`
	JoinedAt          time.Time      `json:"joined_at"`
	PortfolioIndex    float64        `json:"portfolio_index"`
	ReturnPercentage  float64        `json:"return_percentage"`
	GlobalRank        *int           `json:"global_rank,omitempty"`
	SprintRank        *int           `json:"sprint_rank,omitempty"`
	Badges            []PublicBadge  `json:"badges"`
	PublicWeights     []PublicWeight `json:"public_weights"`
	AssetTypeExposure []Exposure     `json:"asset_type_exposure"`
	CurrencyExposure  []Exposure     `json:"currency_exposure"`
	Concentration     Concentration  `json:"concentration"`
}

type OwnerProfile struct {
	Handle            string        `json:"handle"`
	DisplayName       string        `json:"display_name"`
	AvatarKey         string        `json:"avatar_key"`
	Bio               string        `json:"bio"`
	StrategyTag       string        `json:"strategy_tag"`
	IsPublic          bool          `json:"is_public"`
	ShowPublicWeights bool          `json:"show_public_weights"`
	CreatedAt         time.Time     `json:"created_at"`
	UpdatedAt         time.Time     `json:"updated_at"`
	PublicPreview     PublicProfile `json:"public_preview"`
}
