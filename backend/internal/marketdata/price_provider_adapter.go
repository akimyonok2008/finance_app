package marketdata

import (
	"context"
	"time"

	"github.com/ardakimyonok/finance_app/internal/prices"
)

type PriceProviderAdapter struct {
	provider prices.PriceProvider
	source   string
	ttl      time.Duration
}

func NewPriceProviderAdapter(provider prices.PriceProvider, source string, ttl time.Duration) *PriceProviderAdapter {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &PriceProviderAdapter{provider: provider, source: source, ttl: ttl}
}

func (p *PriceProviderAdapter) SearchInstruments(_ context.Context, _ string) ([]Instrument, error) {
	return nil, ErrProviderUnavailable
}

func (p *PriceProviderAdapter) GetQuote(ctx context.Context, symbol string) (Quote, error) {
	price, err := p.provider.GetLatestPrice(ctx, symbol)
	if err != nil {
		return Quote{}, err
	}
	now := time.Now().UTC()
	return Quote{
		Symbol: price.Symbol, Price: price.Price, Currency: price.Currency,
		Provider: p.source, ProviderStatus: StatusOK,
		FetchedAt: now, ExpiresAt: now.Add(p.ttl), UpdatedAt: now,
	}, nil
}

func (p *PriceProviderAdapter) GetQuotes(ctx context.Context, symbols []string) ([]Quote, error) {
	out := make([]Quote, 0, len(symbols))
	for _, symbol := range symbols {
		q, err := p.GetQuote(ctx, symbol)
		if err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, nil
}
