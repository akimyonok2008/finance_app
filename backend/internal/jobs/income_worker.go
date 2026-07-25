package jobs

import (
	"context"
	"log/slog"
	"time"
)

// IncomeJob is the pipeline surface the worker drives: one full ingest + apply
// cycle. It is implemented by the income service.
type IncomeJob interface {
	RunOnce(ctx context.Context) error
}

// IncomeWorker periodically ingests provider income events (dividends, ETF/fund
// distributions, bond coupons, return of capital, stock dividends) and applies
// routine income automatically. It is safe across instances: the income store
// enforces per-(event, portfolio) application uniqueness and transactional
// claiming, so two workers never credit the same event twice.
type IncomeWorker struct {
	job      IncomeJob
	interval time.Duration
}

func NewIncomeWorker(job IncomeJob, interval time.Duration) *IncomeWorker {
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	return &IncomeWorker{job: job, interval: interval}
}

func (w *IncomeWorker) Start(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		w.RunOnce(ctx)
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				w.RunOnce(ctx)
			}
		}
	}()
	return done
}

func (w *IncomeWorker) RunOnce(ctx context.Context) {
	if err := w.job.RunOnce(ctx); err != nil {
		slog.Error("income pass failed", "error", err)
		return
	}
	slog.Info("income pass completed")
}
