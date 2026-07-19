// Package oracle defines the external price-feed contract and its
// implementations: an unreliable upstream (mocked) plus a resilient client that
// wraps it with per-call timeouts, retries/backoff, and a circuit breaker.
//
// The upstream may be slow, flaky, or return zero/negative/implausible prices.
// Validation and last-known-good handling live in the service layer; this
// package's job is to fetch defensively and never hang or crash the caller.
package oracle

import (
	"context"
	"errors"
)

// Price is a raw base-price observation for an item, as reported by the upstream
// feed. It is NOT yet validated — Amount may be zero or negative.
type Price struct {
	ItemID uint64
	Amount int64 // minor units
}

// Source is the raw external price feed. Implementations return the latest
// snapshot of base prices. Callers must pass a context with a deadline.
type Source interface {
	Fetch(ctx context.Context) ([]Price, error)
}

// ErrCircuitOpen is returned by the resilient client when the breaker is open,
// so callers fall back to the last-known-good price instead of hammering a dead
// upstream.
var ErrCircuitOpen = errors.New("oracle circuit breaker open")
