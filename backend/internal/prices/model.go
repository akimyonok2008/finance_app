package prices

import "time"

// Price is a point-in-time quote for a single symbol, annotated with the data
// source that produced it.
type Price struct {
	Symbol    string    `json:"symbol"`
	Price     float64   `json:"price"`
	Currency  string    `json:"currency"`
	Timestamp time.Time `json:"timestamp"`
	Source    string    `json:"source"`
}
