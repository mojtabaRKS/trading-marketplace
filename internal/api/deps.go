package api

import (
	"log/slog"

	"gorm.io/gorm"

	"github.com/herotech/market-dragon/internal/service"
)

// Deps holds everything the HTTP handlers need. Services are added here as
// features land.
type Deps struct {
	Logger   *slog.Logger
	DB       *gorm.DB
	Listings *service.ListingService
	Auctions *service.AuctionService
	Oracle   *service.OracleService
}
