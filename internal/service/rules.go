package service

import "time"

// BidIncrementPercent is the minimum required increase over the current highest
// bid (business rule: a new bid must be at least 5% higher).
const BidIncrementPercent = 5

// AvailableBalance is the spendable balance: total minus what is reserved.
func AvailableBalance(total, reserved int64) int64 {
	return total - reserved
}

// EnsurePositive rejects non-positive amounts.
func EnsurePositive(amount int64) error {
	if amount <= 0 {
		return ErrInvalidAmount
	}
	return nil
}

// EnsureSufficientAvailable verifies a positive amount is covered by the wallet's
// available balance.
func EnsureSufficientAvailable(total, reserved, amount int64) error {
	if err := EnsurePositive(amount); err != nil {
		return err
	}
	if AvailableBalance(total, reserved) < amount {
		return ErrInsufficientFunds
	}
	return nil
}

// EnsureCanReserve verifies a positive amount can be reserved against the wallet
// (available balance must cover it).
func EnsureCanReserve(total, reserved, amount int64) error {
	return EnsureSufficientAvailable(total, reserved, amount)
}

// EnsureCanRelease verifies a positive amount can be released from the reserve.
func EnsureCanRelease(reserved, amount int64) error {
	if err := EnsurePositive(amount); err != nil {
		return err
	}
	if reserved < amount {
		return ErrInsufficientReserved
	}
	return nil
}

// EnsureNotSelfBid rejects a guild bidding on an item it owns.
func EnsureNotSelfBid(ownerGuildID, bidderGuildID uint64) error {
	if ownerGuildID == bidderGuildID {
		return ErrSelfBid
	}
	return nil
}

// MinNextBid returns the smallest acceptable next bid: current + 5%, rounded up.
// For current == 0 (no bids yet) any positive amount qualifies.
func MinNextBid(current int64) int64 {
	// ceil(current * (100 + BidIncrementPercent) / 100) using integer math.
	return (current*(100+BidIncrementPercent) + 99) / 100
}

// EnsureBidBeatsCurrent verifies a bid is positive and at least 5% above current.
func EnsureBidBeatsCurrent(current, amount int64) error {
	if err := EnsurePositive(amount); err != nil {
		return err
	}
	if amount < MinNextBid(current) {
		return ErrBidTooLow
	}
	return nil
}

// EnsureCanCancelBid allows cancellation only when the bidder is not the highest.
func EnsureCanCancelBid(isHighestBidder bool) error {
	if isHighestBidder {
		return ErrCancelHighestBid
	}
	return nil
}

// EnsureWithinDailyCap checks a purchase against a guild's daily cap.
// A dailyCap of 0 means unlimited.
func EnsureWithinDailyCap(dailyCap, spent, amount int64) error {
	if dailyCap == 0 {
		return nil
	}
	if spent+amount > dailyCap {
		return ErrDailyCapExceeded
	}
	return nil
}

// MaybeExtend applies the auction anti-snipe rule: a bid placed within `threshold`
// of the end pushes the end out to now+extension. Returns the (possibly new) end
// time and whether an extension occurred.
func MaybeExtend(now, endsAt time.Time, threshold, extension time.Duration) (time.Time, bool) {
	if endsAt.Sub(now) <= threshold {
		if newEnd := now.Add(extension); newEnd.After(endsAt) {
			return newEnd, true
		}
	}
	return endsAt, false
}

// AuctionEnded reports whether the auction end time has passed.
func AuctionEnded(now, endsAt time.Time) bool {
	return !now.Before(endsAt)
}
