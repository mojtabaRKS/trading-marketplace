package oracle

import (
	"sync"
	"time"
)

type breakerState int

const (
	stateClosed breakerState = iota
	stateOpen
	stateHalfOpen
)

func (s breakerState) String() string {
	switch s {
	case stateOpen:
		return "open"
	case stateHalfOpen:
		return "half-open"
	default:
		return "closed"
	}
}

// CircuitBreaker trips open after `threshold` consecutive failures and stays
// open for `cooldown`, after which it allows a single trial call (half-open). A
// success closes it; a failure re-opens it. It is safe for concurrent use.
type CircuitBreaker struct {
	mu        sync.Mutex
	threshold int
	cooldown  time.Duration
	failures  int
	state     breakerState
	openedAt  time.Time
	now       func() time.Time
}

// NewCircuitBreaker builds a breaker. A threshold <= 0 disables tripping.
func NewCircuitBreaker(threshold int, cooldown time.Duration) *CircuitBreaker {
	return &CircuitBreaker{threshold: threshold, cooldown: cooldown, now: time.Now}
}

// Allow reports whether a call may proceed, moving open->half-open once the
// cooldown has elapsed.
func (b *CircuitBreaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.state == stateOpen {
		if b.now().Sub(b.openedAt) >= b.cooldown {
			b.state = stateHalfOpen
			return true
		}
		return false
	}
	return true
}

// Success resets the breaker to closed.
func (b *CircuitBreaker) Success() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures = 0
	b.state = stateClosed
}

// Failure records a failure, tripping the breaker open when the threshold is
// reached (or immediately if it fails while half-open).
func (b *CircuitBreaker) Failure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.state == stateHalfOpen {
		b.state = stateOpen
		b.openedAt = b.now()
		return
	}
	b.failures++
	if b.threshold > 0 && b.failures >= b.threshold {
		b.state = stateOpen
		b.openedAt = b.now()
	}
}

// State returns the current breaker state as a string (for observability).
func (b *CircuitBreaker) State() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state.String()
}
