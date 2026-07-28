package marketdata

import (
	"context"
	"log/slog"
	"time"
)

type ActiveSymbolProvider interface {
	ListActiveSymbols(ctx context.Context) ([]string, error)
}

// Leader reports whether this process currently holds leadership over the
// gated jobs (implemented by *leaderlock.Elector). A nil Leader is treated as
// "always leader" so single-instance/dev/memory-mode deployments need no
// special-casing.
type Leader interface {
	IsLeader() bool
}

type QuoteRefreshWorker struct {
	svc      *Service
	symbols  ActiveSymbolProvider
	interval time.Duration
	leader   Leader // optional
}

func NewQuoteRefreshWorker(svc *Service, symbols ActiveSymbolProvider, interval time.Duration) *QuoteRefreshWorker {
	if interval <= 0 {
		interval = 10 * time.Minute
	}
	return &QuoteRefreshWorker{svc: svc, symbols: symbols, interval: interval}
}

// SetLeaderElector attaches the coordination gate that keeps quote refresh
// running on exactly one replica. Each replica's RequestLimiter tracks its
// own provider-call budget independently, so two replicas refreshing
// concurrently would each burn through their daily budget for the same
// symbols rather than sharing one combined budget.
func (w *QuoteRefreshWorker) SetLeaderElector(l Leader) {
	w.leader = l
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
				if w.leader != nil && !w.leader.IsLeader() {
					slog.Debug("quote refresh skipped: not leader")
					continue
				}
				w.refresh(ctx)
			}
		}
	}()
}

func (w *QuoteRefreshWorker) refresh(ctx context.Context) {
	symbols, err := w.symbols.ListActiveSymbols(ctx)
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
