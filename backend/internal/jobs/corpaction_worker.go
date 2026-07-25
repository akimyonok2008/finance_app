package jobs

import (
	"context"
	"log/slog"
	"time"
)

// CorporateActionJob is the pipeline surface the worker drives: one full
// ingest + apply cycle. It is implemented by the corpactions service.
type CorporateActionJob interface {
	RunOnce(ctx context.Context) error
}

// CorporateActionWorker periodically ingests provider corporate-action events
// and applies routine transformations automatically. It is safe across
// instances: the corpactions store enforces per-(event, portfolio) application
// uniqueness and transactional claiming, so two workers never apply an event
// twice.
type CorporateActionWorker struct {
	job      CorporateActionJob
	interval time.Duration
}

func NewCorporateActionWorker(job CorporateActionJob, interval time.Duration) *CorporateActionWorker {
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	return &CorporateActionWorker{job: job, interval: interval}
}

func (w *CorporateActionWorker) Start(ctx context.Context) <-chan struct{} {
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

func (w *CorporateActionWorker) RunOnce(ctx context.Context) {
	if err := w.job.RunOnce(ctx); err != nil {
		slog.Error("corporate-action pass failed", "error", err)
		return
	}
	slog.Info("corporate-action pass completed")
}
