package api

import (
	"log/slog"

	"gorm.io/gorm"

	"github.com/herotech/market-dragon/internal/service"
)

// Deps holds everything the HTTP handlers need.
type Deps struct {
	Logger   *slog.Logger
	DB       *gorm.DB
	Items    *service.ItemService
	Listings *service.ListingService
	Auctions *service.AuctionService
	Wallets  *service.WalletService
	Oracle   *service.OracleService
}
