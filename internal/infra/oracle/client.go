package oracle

import (
	"context"
	"fmt"
	"time"
)

// ClientConfig tunes the resilient client's fault tolerance.
type ClientConfig struct {
	Timeout    time.Duration // per-attempt deadline
	MaxRetries int           // extra attempts after the first (0 = single try)
	Backoff    time.Duration // base backoff; doubles each retry
}

// ResilientClient decorates a Source with a per-attempt timeout, bounded
// exponential-backoff retries, and a circuit breaker. It implements Source, so
// the service layer depends only on the plain interface.
type ResilientClient struct {
	src     Source
	cfg     ClientConfig
	breaker *CircuitBreaker
}

// NewResilientClient wraps src with the given config and breaker.
func NewResilientClient(src Source, cfg ClientConfig, breaker *CircuitBreaker) *ResilientClient {
	return &ResilientClient{src: src, cfg: cfg, breaker: breaker}
}

// Fetch attempts the upstream call with timeout + retries, guarded by the
// breaker. When the breaker is open it fails fast with ErrCircuitOpen so callers
// fall back to last-known-good instead of blocking.
func (c *ResilientClient) Fetch(ctx context.Context) ([]Price, error) {
	if !c.breaker.Allow() {
		return nil, ErrCircuitOpen
	}

	var lastErr error
	for attempt := 0; attempt <= c.cfg.MaxRetries; attempt++ {
		if attempt > 0 && c.cfg.Backoff > 0 {
			wait := c.cfg.Backoff << (attempt - 1)
			select {
			case <-ctx.Done():
				c.breaker.Failure()
				return nil, ctx.Err()
			case <-time.After(wait):
			}
		}

		attemptCtx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
		prices, err := c.src.Fetch(attemptCtx)
		cancel()
		if err == nil {
			c.breaker.Success()
			return prices, nil
		}
		lastErr = err
	}

	c.breaker.Failure()
	return nil, fmt.Errorf("oracle fetch failed after %d attempt(s): %w", c.cfg.MaxRetries+1, lastErr)
}

// BreakerState exposes the current breaker state for logging/metrics.
func (c *ResilientClient) BreakerState() string { return c.breaker.State() }
