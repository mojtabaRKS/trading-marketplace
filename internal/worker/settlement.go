// Package worker holds background processes that run alongside the HTTP server.
package worker

import (
	"context"
	"log/slog"
	"time"
)

// AuctionSettler settles auctions whose window has closed.
type AuctionSettler interface {
	SettleDue(ctx context.Context) (int, error)
}

// SettlementWorker periodically settles expired auctions. Settlement itself is
// idempotent and row-locked, so ticking too often (or running several workers)
// is harmless.
type SettlementWorker struct {
	settler  AuctionSettler
	interval time.Duration
	logger   *slog.Logger
}

// NewSettlementWorker builds a SettlementWorker with the given tick interval.
func NewSettlementWorker(settler AuctionSettler, interval time.Duration, logger *slog.Logger) *SettlementWorker {
	return &SettlementWorker{settler: settler, interval: interval, logger: logger}
}

// Run blocks until ctx is cancelled, settling due auctions on each tick. It also
// runs one pass immediately on start so a restart drains any backlog promptly.
func (w *SettlementWorker) Run(ctx context.Context) {
	w.logger.Info("settlement worker started", slog.Duration("interval", w.interval))
	w.tick(ctx)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			w.logger.Info("settlement worker stopped")
			return
		case <-ticker.C:
			w.tick(ctx)
		}
	}
}

func (w *SettlementWorker) tick(ctx context.Context) {
	n, err := w.settler.SettleDue(ctx)
	if err != nil {
		w.logger.Error("settlement tick failed", slog.Any("error", err))
		return
	}
	if n > 0 {
		w.logger.Info("auctions settled", slog.Int("count", n))
	}
}
