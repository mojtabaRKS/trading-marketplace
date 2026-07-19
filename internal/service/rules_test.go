package service

import (
	"errors"
	"testing"
	"time"
)

func TestAvailableBalance(t *testing.T) {
	if got := AvailableBalance(1000, 300); got != 700 {
		t.Fatalf("AvailableBalance = %d, want 700", got)
	}
}

func TestEnsureCanReserve(t *testing.T) {
	tests := []struct {
		name                    string
		total, reserved, amount int64
		want                    error
	}{
		{"ok", 1000, 200, 800, nil},
		{"exact available", 1000, 200, 800, nil},
		{"insufficient", 1000, 900, 200, ErrInsufficientFunds},
		{"zero amount", 1000, 0, 0, ErrInvalidAmount},
		{"negative amount", 1000, 0, -5, ErrInvalidAmount},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := EnsureCanReserve(tc.total, tc.reserved, tc.amount); !errors.Is(err, tc.want) {
				t.Fatalf("EnsureCanReserve = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestEnsureNotSelfBid(t *testing.T) {
	if err := EnsureNotSelfBid(1, 1); !errors.Is(err, ErrSelfBid) {
		t.Fatalf("self-bid should be rejected, got %v", err)
	}
	if err := EnsureNotSelfBid(1, 2); err != nil {
		t.Fatalf("distinct guilds should pass, got %v", err)
	}
}

func TestMinNextBid(t *testing.T) {
	tests := []struct {
		current, want int64
	}{
		{0, 0},     // first bid: any positive amount qualifies
		{100, 105}, // +5%
		{105, 111}, // ceil(110.25)
		{1000, 1050},
		{99, 104}, // ceil(103.95)
	}
	for _, tc := range tests {
		if got := MinNextBid(tc.current); got != tc.want {
			t.Fatalf("MinNextBid(%d) = %d, want %d", tc.current, got, tc.want)
		}
	}
}

func TestEnsureBidBeatsCurrent(t *testing.T) {
	tests := []struct {
		name            string
		current, amount int64
		want            error
	}{
		{"first bid positive", 0, 1, nil},
		{"exactly +5%", 100, 105, nil},
		{"above +5%", 100, 200, nil},
		{"below +5%", 100, 104, ErrBidTooLow},
		{"equal current", 100, 100, ErrBidTooLow},
		{"non-positive", 0, 0, ErrInvalidAmount},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := EnsureBidBeatsCurrent(tc.current, tc.amount); !errors.Is(err, tc.want) {
				t.Fatalf("EnsureBidBeatsCurrent(%d,%d) = %v, want %v", tc.current, tc.amount, err, tc.want)
			}
		})
	}
}

func TestEnsureCanCancelBid(t *testing.T) {
	if err := EnsureCanCancelBid(true); !errors.Is(err, ErrCancelHighestBid) {
		t.Fatalf("highest bidder cancel should be rejected, got %v", err)
	}
	if err := EnsureCanCancelBid(false); err != nil {
		t.Fatalf("non-highest cancel should pass, got %v", err)
	}
}

func TestEnsureWithinDailyCap(t *testing.T) {
	tests := []struct {
		name                    string
		dailyCap, spent, amount int64
		want                    error
	}{
		{"unlimited", 0, 1_000_000, 999, nil},
		{"within", 1000, 500, 500, nil},
		{"exceeds", 1000, 900, 200, ErrDailyCapExceeded},
		{"exact cap", 1000, 0, 1000, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := EnsureWithinDailyCap(tc.dailyCap, tc.spent, tc.amount); !errors.Is(err, tc.want) {
				t.Fatalf("EnsureWithinDailyCap = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestMaybeExtend(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	threshold := 5 * time.Minute
	extension := 5 * time.Minute

	t.Run("within threshold extends", func(t *testing.T) {
		endsAt := now.Add(2 * time.Minute) // only 2 min left
		got, extended := MaybeExtend(now, endsAt, threshold, extension)
		if !extended {
			t.Fatal("expected extension")
		}
		if want := now.Add(extension); !got.Equal(want) {
			t.Fatalf("new end = %v, want %v", got, want)
		}
	})

	t.Run("outside threshold no change", func(t *testing.T) {
		endsAt := now.Add(30 * time.Minute)
		got, extended := MaybeExtend(now, endsAt, threshold, extension)
		if extended {
			t.Fatal("did not expect extension")
		}
		if !got.Equal(endsAt) {
			t.Fatalf("end changed to %v, want %v", got, endsAt)
		}
	})
}

func TestAuctionEnded(t *testing.T) {
	now := time.Now()
	if !AuctionEnded(now, now.Add(-time.Second)) {
		t.Fatal("past end should be ended")
	}
	if AuctionEnded(now, now.Add(time.Hour)) {
		t.Fatal("future end should not be ended")
	}
}
