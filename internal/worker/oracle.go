package worker

import (
	"context"
	"log/slog"
	"time"
)

// PriceRefresher pulls and validates a fresh price snapshot, returning how many
// prices were accepted.
type PriceRefresher interface {
	Refresh(ctx context.Context) (int, error)
}

// OraclePoller periodically refreshes prices from the external feed. A failed or
// slow upstream is logged and skipped — the refresher retains last-known-good —
// so the poller never crashes the process.
type OraclePoller struct {
	refresher PriceRefresher
	interval  time.Duration
	logger    *slog.Logger
}

// NewOraclePoller builds an OraclePoller with the given poll interval.
func NewOraclePoller(refresher PriceRefresher, interval time.Duration, logger *slog.Logger) *OraclePoller {
	return &OraclePoller{refresher: refresher, interval: interval, logger: logger}
}

// Run blocks until ctx is cancelled, refreshing once immediately and then every
// interval.
func (w *OraclePoller) Run(ctx context.Context) {
	w.logger.Info("oracle poller started", slog.Duration("interval", w.interval))
	w.tick(ctx)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			w.logger.Info("oracle poller stopped")
			return
		case <-ticker.C:
			w.tick(ctx)
		}
	}
}

func (w *OraclePoller) tick(ctx context.Context) {
	n, err := w.refresher.Refresh(ctx)
	if err != nil {
		w.logger.Warn("oracle refresh failed; keeping last-known-good", slog.Any("error", err))
		return
	}
	if n > 0 {
		w.logger.Debug("oracle prices refreshed", slog.Int("accepted", n))
	}
}
