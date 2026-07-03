package marketdata

import "time"

const (
	ProviderMock       = "mock"
	ProviderTwelveData = "twelvedata"

	StatusOK                  = "ok"
	StatusCached              = "cached"
	StatusStale               = "stale"
	StatusRateLimited         = "rate_limited"
	StatusProviderUnavailable = "provider_unavailable"
	StatusMock                = "mock"
)

type Instrument struct {
	Symbol         string     `json:"symbol"`
	DisplaySymbol  string     `json:"display_symbol"`
	Name           string     `json:"name"`
	Exchange       string     `json:"exchange"`
	Country        string     `json:"country"`
	Currency       string     `json:"currency"`
	AssetType      string     `json:"asset_type"`
	Provider       string     `json:"provider"`
	ProviderSymbol string     `json:"-"`
	IsActive       bool       `json:"-"`
	CreatedAt      time.Time  `json:"-"`
	UpdatedAt      time.Time  `json:"-"`
	LastSeenAt     *time.Time `json:"-"`
}

type Quote struct {
	Symbol            string     `json:"symbol"`
	Price             float64    `json:"price"`
	Currency          string     `json:"currency"`
	ChangePercentage  *float64   `json:"change_percentage,omitempty"`
	PreviousClose     *float64   `json:"previous_close,omitempty"`
	MarketTime        *time.Time `json:"market_time,omitempty"`
	Provider          string     `json:"provider"`
	IsDelayed         bool       `json:"is_delayed"`
	DelayMinutes      *int       `json:"delay_minutes,omitempty"`
	IsStale           bool       `json:"is_stale"`
	FetchedAt         time.Time  `json:"fetched_at"`
	ExpiresAt         time.Time  `json:"expires_at"`
	ProviderStatus    string     `json:"provider_status"`
	RawProviderSymbol string     `json:"-"`
	UpdatedAt         time.Time  `json:"-"`
}

type SearchResponse struct {
	Results []Instrument `json:"results"`
	Source  string       `json:"source"`
}

type QuotesResponse struct {
	Quotes []Quote `json:"quotes"`
	Source string  `json:"source"`
}
