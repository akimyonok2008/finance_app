package marketdata

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ardakimyonok/finance_app/internal/prices"
)

type ServiceConfig struct {
	RealMarketDataEnabled bool
	QuoteCacheTTL         time.Duration
	QuoteStaleAfter       time.Duration
	AllowStaleOnError     bool
	SearchLimit           int
	MaxQuoteBatchSize     int
}

type Service struct {
	repo     Repository
	provider Provider
	cfg      ServiceConfig
}

func NewService(repo Repository, provider Provider, cfg ServiceConfig) *Service {
	if cfg.QuoteCacheTTL <= 0 {
		cfg.QuoteCacheTTL = 10 * time.Minute
	}
	if cfg.QuoteStaleAfter <= 0 {
		cfg.QuoteStaleAfter = 15 * time.Minute
	}
	if cfg.SearchLimit <= 0 {
		cfg.SearchLimit = 10
	}
	if cfg.MaxQuoteBatchSize <= 0 {
		cfg.MaxQuoteBatchSize = 25
	}
	return &Service{repo: repo, provider: provider, cfg: cfg}
}

func (s *Service) SearchInstruments(ctx context.Context, query string) (SearchResponse, error) {
	q := strings.TrimSpace(query)
	if len(q) < 1 || len(q) > 64 {
		return SearchResponse{}, ErrInvalidQuery
	}
	local, err := s.repo.SearchInstruments(ctx, q, s.cfg.SearchLimit)
	if err != nil {
		return SearchResponse{}, err
	}
	source := "cache"
	if len(local) >= s.cfg.SearchLimit || !s.cfg.RealMarketDataEnabled {
		if len(local) > 0 {
			return SearchResponse{Results: local, Source: source}, nil
		}
		seeded, err := s.provider.SearchInstruments(ctx, q)
		if err != nil {
			return SearchResponse{Results: local, Source: source}, nil
		}
		if err := s.repo.UpsertInstruments(ctx, seeded); err != nil {
			return SearchResponse{}, err
		}
		merged := mergeInstruments(local, seeded, strings.ToUpper(q), s.cfg.SearchLimit)
		return SearchResponse{Results: merged, Source: "provider"}, nil
	}
	remote, err := s.provider.SearchInstruments(ctx, q)
	if err != nil {
		if len(local) > 0 {
			return SearchResponse{Results: local, Source: "cache"}, nil
		}
		return SearchResponse{}, err
	}
	if err := s.repo.UpsertInstruments(ctx, remote); err != nil {
		return SearchResponse{}, err
	}
	merged := mergeInstruments(nil, remote, strings.ToUpper(q), s.cfg.SearchLimit)
	if len(merged) == 0 && len(local) > 0 {
		return SearchResponse{Results: local, Source: "cache"}, nil
	}
	source = "provider"
	return SearchResponse{Results: merged, Source: source}, nil
}

func (s *Service) Quotes(ctx context.Context, symbols []string) (QuotesResponse, error) {
	normalized := dedupeSymbols(symbols)
	if len(normalized) == 0 || len(normalized) > s.cfg.MaxQuoteBatchSize {
		return QuotesResponse{}, ErrInvalidSymbols
	}
	now := time.Now().UTC()
	out := make([]Quote, 0, len(normalized))
	missing := make([]string, 0)
	source := "cache"
	for _, symbol := range normalized {
		cached, ok, err := s.repo.GetQuote(ctx, symbol)
		if err != nil {
			return QuotesResponse{}, err
		}
		if ok {
			cached.IsStale = now.After(cached.ExpiresAt) || now.Sub(cached.FetchedAt) > s.cfg.QuoteStaleAfter
			if !cached.IsStale {
				cached.ProviderStatus = firstNonEmpty(cached.ProviderStatus, StatusCached)
				out = append(out, cached)
				continue
			}
		}
		missing = append(missing, symbol)
		if ok {
			out = append(out, markStale(cached, StatusStale))
		}
	}
	if len(missing) == 0 {
		return QuotesResponse{Quotes: sortQuotes(out, normalized), Source: source}, nil
	}
	fresh, err := s.provider.GetQuotes(ctx, missing)
	if err != nil {
		if s.cfg.AllowStaleOnError {
			stale := make([]Quote, 0, len(out))
			for _, q := range out {
				if containsSymbol(missing, q.Symbol) {
					status := StatusProviderUnavailable
					if errors.Is(err, ErrProviderRateLimited) || errors.Is(err, ErrDailyBudgetExhausted) {
						status = StatusRateLimited
					}
					stale = append(stale, markStale(q, status))
				} else {
					stale = append(stale, q)
				}
			}
			if len(stale) > 0 {
				return QuotesResponse{Quotes: sortQuotes(stale, normalized), Source: "cache"}, nil
			}
		}
		return QuotesResponse{}, err
	}
	for i := range fresh {
		fresh[i].ExpiresAt = fresh[i].FetchedAt.Add(s.cfg.QuoteCacheTTL)
		if fresh[i].ProviderStatus == "" {
			fresh[i].ProviderStatus = StatusOK
		}
	}
	if err := s.repo.UpsertQuotes(ctx, fresh); err != nil {
		return QuotesResponse{}, err
	}
	replaceBySymbol := map[string]Quote{}
	for _, q := range fresh {
		replaceBySymbol[normalizeSymbol(q.Symbol)] = q
	}
	combined := make([]Quote, 0, len(normalized))
	for _, q := range out {
		if freshQuote, ok := replaceBySymbol[normalizeSymbol(q.Symbol)]; ok {
			combined = append(combined, freshQuote)
			delete(replaceBySymbol, normalizeSymbol(q.Symbol))
		} else {
			combined = append(combined, q)
		}
	}
	for _, q := range replaceBySymbol {
		combined = append(combined, q)
	}
	if len(out) > 0 {
		source = "mixed"
	} else {
		source = "provider"
	}
	return QuotesResponse{Quotes: sortQuotes(combined, normalized), Source: source}, nil
}

func (s *Service) GetLatestPrice(ctx context.Context, symbol string) (*prices.Price, error) {
	resp, err := s.Quotes(ctx, []string{symbol})
	if err != nil {
		return nil, err
	}
	if len(resp.Quotes) == 0 {
		return nil, prices.ErrPriceUnavailable
	}
	q := resp.Quotes[0]
	return &prices.Price{
		Symbol: q.Symbol, Price: q.Price, Currency: q.Currency,
		Timestamp: q.FetchedAt, Source: q.Provider,
		ChangePercentage: q.ChangePercentage, PreviousClose: q.PreviousClose,
		MarketTime: q.MarketTime, IsDelayed: q.IsDelayed, DelayMinutes: q.DelayMinutes,
		IsStale: q.IsStale, FetchedAt: q.FetchedAt, ExpiresAt: q.ExpiresAt,
		ProviderStatus: q.ProviderStatus,
	}, nil
}

func (s *Service) RefreshSymbols(ctx context.Context, symbols []string) (int, error) {
	normalized := dedupeSymbols(symbols)
	if len(normalized) == 0 {
		return 0, nil
	}
	totalQuotes := 0
	for i := 0; i < len(normalized); i += s.cfg.MaxQuoteBatchSize {
		end := i + s.cfg.MaxQuoteBatchSize
		if end > len(normalized) {
			end = len(normalized)
		}
		resp, err := s.Quotes(ctx, normalized[i:end])
		if err != nil {
			return totalQuotes, err
		}
		totalQuotes += len(resp.Quotes)
	}
	return totalQuotes, nil
}

func mergeInstruments(a, b []Instrument, query string, limit int) []Instrument {
	seen := map[string]Instrument{}
	for _, item := range a {
		seen[normalizeSymbol(item.Symbol)] = item
	}
	for _, item := range b {
		seen[normalizeSymbol(item.Symbol)] = item
	}
	out := make([]Instrument, 0, len(seen))
	for _, item := range seen {
		out = append(out, item)
	}
	sortInstruments(out, query)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func markStale(q Quote, status string) Quote {
	q.IsStale = true
	q.ProviderStatus = status
	return q
}

func containsSymbol(symbols []string, symbol string) bool {
	target := normalizeSymbol(symbol)
	for _, item := range symbols {
		if normalizeSymbol(item) == target {
			return true
		}
	}
	return false
}

func sortQuotes(quotes []Quote, order []string) []Quote {
	bySymbol := map[string]Quote{}
	for _, q := range quotes {
		bySymbol[normalizeSymbol(q.Symbol)] = q
	}
	out := make([]Quote, 0, len(bySymbol))
	for _, symbol := range order {
		if q, ok := bySymbol[normalizeSymbol(symbol)]; ok {
			out = append(out, q)
		}
	}
	return out
}

func (s *Service) ValidateSymbols(symbols []string) error {
	normalized := dedupeSymbols(symbols)
	if len(normalized) == 0 || len(normalized) > s.cfg.MaxQuoteBatchSize {
		return fmt.Errorf("%w: max batch size %d", ErrInvalidSymbols, s.cfg.MaxQuoteBatchSize)
	}
	return nil
}
