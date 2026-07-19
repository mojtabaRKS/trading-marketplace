package api

import (
	"log/slog"

	"github.com/herotech/market-dragon/internal/service"
)

// Deps holds everything the HTTP handlers need. Services are added here as
// features land.
type Deps struct {
	Logger   *slog.Logger
	Listings *service.ListingService
	Auctions *service.AuctionService
	Oracle   *service.OracleService
}
