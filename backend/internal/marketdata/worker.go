package marketdata

import (
	"context"
	"log/slog"
	"time"
)

type ActiveSymbolProvider interface {
	ListActiveSymbols() ([]string, error)
}

type QuoteRefreshWorker struct {
	svc      *Service
	symbols  ActiveSymbolProvider
	interval time.Duration
}

func NewQuoteRefreshWorker(svc *Service, symbols ActiveSymbolProvider, interval time.Duration) *QuoteRefreshWorker {
	if interval <= 0 {
		interval = 10 * time.Minute
	}
	return &QuoteRefreshWorker{svc: svc, symbols: symbols, interval: interval}
}

func (w *QuoteRefreshWorker) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				w.refresh(ctx)
			}
		}
	}()
}

func (w *QuoteRefreshWorker) refresh(ctx context.Context) {
	symbols, err := w.symbols.ListActiveSymbols()
	if err != nil {
		slog.Warn("quote refresh skipped: could not list active symbols", "error", err)
		return
	}
	symbols = dedupeSymbols(symbols)
	if len(symbols) == 0 {
		slog.Info("quote refresh skipped: no active symbols")
		return
	}
	count, err := w.svc.RefreshSymbols(ctx, symbols)
	if err != nil {
		slog.Warn("quote refresh failed", "error", err, "symbols", len(symbols))
		return
	}
	slog.Info("quote refresh completed", "symbols", len(symbols), "quotes", count)
}
