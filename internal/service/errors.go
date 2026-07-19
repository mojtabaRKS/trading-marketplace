package service

import "errors"

// Sentinel domain errors returned by the pure business rules. Services and the
// HTTP layer map these to appropriate responses.
var (
	ErrInvalidAmount        = errors.New("amount must be positive")
	ErrInsufficientFunds    = errors.New("insufficient available balance")
	ErrInsufficientReserved = errors.New("release exceeds reserved amount")
	ErrSelfBid              = errors.New("guild cannot bid on its own item")
	ErrBidTooLow            = errors.New("bid must be at least 5% above the current highest bid")
	ErrCancelHighestBid     = errors.New("cannot cancel bid while highest bidder")
	ErrDailyCapExceeded     = errors.New("daily purchase cap exceeded")
)
