// Package oracle defines the external price-feed contract and its
// implementations (real client + mock). The concrete resilient client is added
// in the Oracle step; this file establishes the port the service layer depends on.
package oracle

import "context"

// Price is a base price observation for an item.
type Price struct {
	ItemID uint64
	Amount int64 // minor units; always > 0 once validated
}

// Port is the interface to the external Oracle Price Service. Implementations
// must tolerate slow/wrong/zero/negative upstream responses and never surface
// invalid prices to callers.
type Port interface {
	// FetchPrice returns the latest validated base price for an item.
	FetchPrice(ctx context.Context, itemID uint64) (Price, error)
}
