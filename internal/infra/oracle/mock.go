package oracle

import (
	"context"
	"sync"
	"time"
)

// MockSource is a configurable stand-in for the real Oracle Price Service. It
// can be told to be slow (Latency), to fail (Err), or to return arbitrary
// values including zero/negative/implausible prices — everything the resilient
// client and validation layer must tolerate.
type MockSource struct {
	mu      sync.Mutex
	prices  []Price
	err     error
	latency time.Duration
}

// NewMockSource returns a healthy mock seeded with the given prices.
func NewMockSource(prices ...Price) *MockSource {
	return &MockSource{prices: prices}
}

// SetPrices replaces the snapshot the mock will return.
func (m *MockSource) SetPrices(prices ...Price) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.prices = prices
}

// SetError makes Fetch return err (nil to clear).
func (m *MockSource) SetError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.err = err
}

// SetLatency makes Fetch block for d before returning (simulating a slow feed).
func (m *MockSource) SetLatency(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.latency = d
}

// Fetch returns the configured snapshot, honouring latency and ctx cancellation.
func (m *MockSource) Fetch(ctx context.Context) ([]Price, error) {
	m.mu.Lock()
	latency, err := m.latency, m.err
	snapshot := make([]Price, len(m.prices))
	copy(snapshot, m.prices)
	m.mu.Unlock()

	if latency > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(latency):
		}
	}
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}
