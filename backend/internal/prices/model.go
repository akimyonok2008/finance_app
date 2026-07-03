package prices

import "time"

// Price is a point-in-time quote for a single symbol, annotated with the data
// source that produced it.
type Price struct {
	Symbol           string     `json:"symbol"`
	Price            float64    `json:"price"`
	Currency         string     `json:"currency"`
	Timestamp        time.Time  `json:"timestamp"`
	Source           string     `json:"source"`
	ChangePercentage *float64   `json:"change_percentage,omitempty"`
	PreviousClose    *float64   `json:"previous_close,omitempty"`
	MarketTime       *time.Time `json:"market_time,omitempty"`
	IsDelayed        bool       `json:"is_delayed"`
	DelayMinutes     *int       `json:"delay_minutes,omitempty"`
	IsStale          bool       `json:"is_stale"`
	FetchedAt        time.Time  `json:"fetched_at,omitempty"`
	ExpiresAt        time.Time  `json:"expires_at,omitempty"`
	ProviderStatus   string     `json:"provider_status,omitempty"`
}
