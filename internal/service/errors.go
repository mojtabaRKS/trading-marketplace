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

	ErrNotFound              = errors.New("resource not found")
	ErrItemNotOwned          = errors.New("item is not owned by the seller")
	ErrLegendaryNeedsAuction = errors.New("legendary items must be sold via auction")
	ErrOutOfStock            = errors.New("item is out of stock")
	ErrListingNotOpen        = errors.New("listing is not open")
	ErrSelfPurchase          = errors.New("guild cannot buy its own listing")

	ErrNotLegendary        = errors.New("only legendary items can be auctioned")
	ErrItemNotAvailable    = errors.New("item is not available")
	ErrActiveAuctionExists = errors.New("item already has an active auction")
	ErrAuctionNotActive    = errors.New("auction is not active")
	ErrAuctionEnded        = errors.New("auction has ended")

	ErrOraclePriceInvalid     = errors.New("oracle price must be positive")
	ErrOraclePriceImplausible = errors.New("oracle price is implausible")
	ErrPriceUnavailable       = errors.New("no validated price available for item")
)
