package oracle

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCircuitBreakerTripsAndRecovers(t *testing.T) {
	b := NewCircuitBreaker(3, 50*time.Millisecond)

	if !b.Allow() {
		t.Fatal("fresh breaker should allow")
	}
	// Below threshold stays closed.
	b.Failure()
	b.Failure()
	if !b.Allow() || b.State() != "closed" {
		t.Fatalf("state=%s, want closed and allowing", b.State())
	}
	// Third failure trips it open.
	b.Failure()
	if b.Allow() || b.State() != "open" {
		t.Fatalf("state=%s allow=%v, want open and blocking", b.State(), b.Allow())
	}

	// After cooldown it half-opens for one trial.
	time.Sleep(60 * time.Millisecond)
	if !b.Allow() {
		t.Fatal("breaker should allow a trial after cooldown")
	}
	if b.State() != "half-open" {
		t.Fatalf("state=%s, want half-open", b.State())
	}
	// A success closes it again.
	b.Success()
	if b.State() != "closed" {
		t.Fatalf("state=%s, want closed after success", b.State())
	}
}

func TestResilientClientRetriesThenSucceeds(t *testing.T) {
	src := NewMockSource(Price{ItemID: 1, Amount: 100})
	src.SetError(errors.New("boom"))

	b := NewCircuitBreaker(5, time.Second)
	c := NewResilientClient(src, ClientConfig{Timeout: 100 * time.Millisecond, MaxRetries: 3, Backoff: time.Millisecond}, b)

	// Clear the error after a short delay so a later attempt succeeds.
	go func() {
		time.Sleep(5 * time.Millisecond)
		src.SetError(nil)
	}()

	prices, err := c.Fetch(context.Background())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(prices) != 1 || prices[0].Amount != 100 {
		t.Fatalf("prices=%v, want one price of 100", prices)
	}
}

func TestResilientClientOpensCircuitAfterFailures(t *testing.T) {
	src := NewMockSource()
	src.SetError(errors.New("down"))

	b := NewCircuitBreaker(1, time.Minute) // trip after a single failed Fetch
	c := NewResilientClient(src, ClientConfig{Timeout: 50 * time.Millisecond, MaxRetries: 0, Backoff: 0}, b)

	if _, err := c.Fetch(context.Background()); err == nil {
		t.Fatal("expected first fetch to fail")
	}
	// Breaker now open -> fail fast with ErrCircuitOpen.
	if _, err := c.Fetch(context.Background()); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("err=%v, want ErrCircuitOpen", err)
	}
}

func TestResilientClientTimesOutSlowSource(t *testing.T) {
	src := NewMockSource(Price{ItemID: 1, Amount: 100})
	src.SetLatency(200 * time.Millisecond)

	b := NewCircuitBreaker(5, time.Second)
	c := NewResilientClient(src, ClientConfig{Timeout: 20 * time.Millisecond, MaxRetries: 1, Backoff: time.Millisecond}, b)

	start := time.Now()
	if _, err := c.Fetch(context.Background()); err == nil {
		t.Fatal("expected timeout error from slow source")
	}
	// Two attempts of ~20ms each; must be well under the 200ms source latency.
	if elapsed := time.Since(start); elapsed > 150*time.Millisecond {
		t.Fatalf("fetch took %s, expected fast timeout", elapsed)
	}
}
