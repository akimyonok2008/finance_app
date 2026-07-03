package marketdata

import (
	"context"
	"strings"
	"time"

	"github.com/ardakimyonok/finance_app/internal/prices"
)

type MockProvider struct {
	prices *prices.MockPriceProvider
	items  []Instrument
}

func NewMockProvider(priceProvider *prices.MockPriceProvider) *MockProvider {
	items := []Instrument{
		{Symbol: "AAPL", DisplaySymbol: "AAPL", Name: "Apple Inc.", Exchange: "NASDAQ", Country: "US", Currency: "USD", AssetType: "stock", Provider: ProviderMock, ProviderSymbol: "AAPL"},
		{Symbol: "MSFT", DisplaySymbol: "MSFT", Name: "Microsoft Corp.", Exchange: "NASDAQ", Country: "US", Currency: "USD", AssetType: "stock", Provider: ProviderMock, ProviderSymbol: "MSFT"},
		{Symbol: "NVDA", DisplaySymbol: "NVDA", Name: "NVIDIA Corp.", Exchange: "NASDAQ", Country: "US", Currency: "USD", AssetType: "stock", Provider: ProviderMock, ProviderSymbol: "NVDA"},
		{Symbol: "SPY", DisplaySymbol: "SPY", Name: "SPDR S&P 500 ETF Trust", Exchange: "NYSE ARCA", Country: "US", Currency: "USD", AssetType: "etf", Provider: ProviderMock, ProviderSymbol: "SPY"},
		{Symbol: "QQQ", DisplaySymbol: "QQQ", Name: "Invesco QQQ Trust", Exchange: "NASDAQ", Country: "US", Currency: "USD", AssetType: "etf", Provider: ProviderMock, ProviderSymbol: "QQQ"},
		{Symbol: "VOO", DisplaySymbol: "VOO", Name: "Vanguard S&P 500 ETF", Exchange: "NYSE ARCA", Country: "US", Currency: "USD", AssetType: "etf", Provider: ProviderMock, ProviderSymbol: "VOO"},
		{Symbol: "IJR", DisplaySymbol: "IJR", Name: "iShares Core S&P Small-Cap ETF", Exchange: "NYSE ARCA", Country: "US", Currency: "USD", AssetType: "etf", Provider: ProviderMock, ProviderSymbol: "IJR"},
		{Symbol: "EEM", DisplaySymbol: "EEM", Name: "iShares MSCI Emerging Markets ETF", Exchange: "NYSE ARCA", Country: "US", Currency: "USD", AssetType: "etf", Provider: ProviderMock, ProviderSymbol: "EEM"},
		{Symbol: "GLD", DisplaySymbol: "GLD", Name: "SPDR Gold Shares", Exchange: "NYSE ARCA", Country: "US", Currency: "USD", AssetType: "etf", Provider: ProviderMock, ProviderSymbol: "GLD"},
		{Symbol: "SIVR", DisplaySymbol: "SIVR", Name: "abrdn Physical Silver Shares ETF", Exchange: "NYSE ARCA", Country: "US", Currency: "USD", AssetType: "etf", Provider: ProviderMock, ProviderSymbol: "SIVR"},
		{Symbol: "URA", DisplaySymbol: "URA", Name: "Global X Uranium ETF", Exchange: "NYSE ARCA", Country: "US", Currency: "USD", AssetType: "etf", Provider: ProviderMock, ProviderSymbol: "URA"},
		{Symbol: "SGOV", DisplaySymbol: "SGOV", Name: "iShares 0-3 Month Treasury Bond ETF", Exchange: "NYSE ARCA", Country: "US", Currency: "USD", AssetType: "etf", Provider: ProviderMock, ProviderSymbol: "SGOV"},
		{Symbol: "BTC-USD", DisplaySymbol: "BTC-USD", Name: "Bitcoin USD", Exchange: "CRYPTO", Country: "US", Currency: "USD", AssetType: "crypto", Provider: ProviderMock, ProviderSymbol: "BTC-USD"},
		{Symbol: "ETH-USD", DisplaySymbol: "ETH-USD", Name: "Ethereum USD", Exchange: "CRYPTO", Country: "US", Currency: "USD", AssetType: "crypto", Provider: ProviderMock, ProviderSymbol: "ETH-USD"},
		{Symbol: "THYAO.IS", DisplaySymbol: "THYAO.IS", Name: "Turkish Airlines", Exchange: "BIST", Country: "TR", Currency: "TRY", AssetType: "stock", Provider: ProviderMock, ProviderSymbol: "THYAO.IS"},
		{Symbol: "GARAN.IS", DisplaySymbol: "GARAN.IS", Name: "Garanti BBVA", Exchange: "BIST", Country: "TR", Currency: "TRY", AssetType: "stock", Provider: ProviderMock, ProviderSymbol: "GARAN.IS"},
		{Symbol: "ASELS.IS", DisplaySymbol: "ASELS.IS", Name: "Aselsan", Exchange: "BIST", Country: "TR", Currency: "TRY", AssetType: "stock", Provider: ProviderMock, ProviderSymbol: "ASELS.IS"},
	}
	priceProvider.Set("QQQ", 480, "USD")
	priceProvider.Set("VOO", 505, "USD")
	priceProvider.Set("IJR", 112, "USD")
	priceProvider.Set("EEM", 43, "USD")
	priceProvider.Set("GLD", 225, "USD")
	priceProvider.Set("SIVR", 28, "USD")
	priceProvider.Set("URA", 31, "USD")
	priceProvider.Set("SGOV", 100.4, "USD")
	return &MockProvider{prices: priceProvider, items: items}
}

func (p *MockProvider) SearchInstruments(_ context.Context, query string) ([]Instrument, error) {
	q := strings.ToUpper(strings.TrimSpace(query))
	out := make([]Instrument, 0)
	for _, item := range p.items {
		if matchesInstrument(item, q) {
			out = append(out, item)
		}
	}
	sortInstruments(out, q)
	return out, nil
}

func (p *MockProvider) GetQuote(ctx context.Context, symbol string) (Quote, error) {
	price, err := p.prices.GetLatestPrice(ctx, symbol)
	if err != nil {
		return Quote{}, err
	}
	now := time.Now().UTC()
	return Quote{
		Symbol:         price.Symbol,
		Price:          price.Price,
		Currency:       price.Currency,
		Provider:       ProviderMock,
		ProviderStatus: StatusMock,
		FetchedAt:      now,
		ExpiresAt:      now.Add(10 * time.Minute),
		UpdatedAt:      now,
	}, nil
}

func (p *MockProvider) GetQuotes(ctx context.Context, symbols []string) ([]Quote, error) {
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
