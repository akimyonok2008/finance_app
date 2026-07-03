package marketdata

import "context"

type QuoteProvider interface {
	GetQuote(ctx context.Context, symbol string) (Quote, error)
	GetQuotes(ctx context.Context, symbols []string) ([]Quote, error)
}

type InstrumentProvider interface {
	SearchInstruments(ctx context.Context, query string) ([]Instrument, error)
}

type Provider interface {
	QuoteProvider
	InstrumentProvider
}
