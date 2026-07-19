package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/herotech/market-dragon/internal/repository"
)

// ListingService implements the fixed-price limit-order flow for Common and
// Rare items: listing an item and buying it. All money and asset changes for a
// purchase happen in a single transaction with row locks, so an item sells at
// most once and buyers never over-commit or exceed their daily cap.
type ListingService struct {
	db      *gorm.DB
	wallets *WalletService
	now     func() time.Time
}

// NewListingService builds a ListingService.
func NewListingService(db *gorm.DB, wallets *WalletService) *ListingService {
	return &ListingService{db: db, wallets: wallets, now: time.Now}
}

// CreateListing lists a seller-owned Common/Rare item at a fixed price.
func (s *ListingService) CreateListing(ctx context.Context, sellerGuildID, itemID uint64, price int64) (*repository.Listing, error) {
	if err := EnsurePositive(price); err != nil {
		return nil, err
	}

	var listing repository.Listing
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		item, err := lockItem(tx, itemID)
		if err != nil {
			return err
		}
		if item.OwnerGuildID != sellerGuildID {
			return ErrItemNotOwned
		}
		if item.Tier == repository.TierLegendary {
			return ErrLegendaryNeedsAuction
		}
		if item.Stock < 1 {
			return ErrOutOfStock
		}

		listing = repository.Listing{
			ItemID:        itemID,
			SellerGuildID: sellerGuildID,
			Price:         price,
			Status:        repository.ListingOpen,
			CreatedAt:     s.now(),
		}
		if err := tx.Create(&listing).Error; err != nil {
			return fmt.Errorf("create listing: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &listing, nil
}

// Buy purchases an open listing: it validates funds and the buyer's daily cap,
// moves money seller<-buyer, transfers one unit of the item, and marks the
// listing sold — atomically.
func (s *ListingService) Buy(ctx context.Context, buyerGuildID, listingID uint64) (*repository.Listing, error) {
	var listing repository.Listing
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Lock the listing first: this is what serializes concurrent buys.
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&listing, listingID).Error; err != nil {
			return notFoundOr(err, "lock listing")
		}
		if listing.Status != repository.ListingOpen {
			return ErrListingNotOpen
		}
		if listing.SellerGuildID == buyerGuildID {
			return ErrSelfPurchase
		}

		item, err := lockItem(tx, listing.ItemID)
		if err != nil {
			return err
		}
		if item.Stock < 1 {
			return ErrOutOfStock
		}

		price := listing.Price

		// Serialize this buyer's concurrent purchases by locking their wallet
		// before reading the daily total, then enforce the daily cap.
		if err := lockWallet(tx, buyerGuildID); err != nil {
			return err
		}
		var buyer repository.Guild
		if err := tx.First(&buyer, buyerGuildID).Error; err != nil {
			return notFoundOr(err, "load buyer guild")
		}
		spent, err := s.dailySpent(tx, buyerGuildID)
		if err != nil {
			return err
		}
		if err := EnsureWithinDailyCap(buyer.DailyPurchaseCap, spent, price); err != nil {
			return err
		}

		// Money moves within the same transaction.
		if err := s.wallets.DebitTx(tx, buyerGuildID, price, repository.RefListing, listingID); err != nil {
			return err
		}
		if err := s.wallets.CreditTx(tx, listing.SellerGuildID, price, repository.RefListing, listingID); err != nil {
			return err
		}

		// Transfer one unit: decrement seller stock, grant the buyer a unit.
		item.Stock--
		if item.Stock == 0 {
			item.Status = repository.ItemSold
		}
		if err := tx.Save(item).Error; err != nil {
			return fmt.Errorf("update seller item: %w", err)
		}
		bought := repository.Item{
			Name:         item.Name,
			Tier:         item.Tier,
			OwnerGuildID: buyerGuildID,
			Status:       repository.ItemAvailable,
			Stock:        1,
			CreatedAt:    s.now(),
			UpdatedAt:    s.now(),
		}
		if err := tx.Create(&bought).Error; err != nil {
			return fmt.Errorf("grant item to buyer: %w", err)
		}

		if err := s.addDailySpent(tx, buyerGuildID, price); err != nil {
			return err
		}

		// Mark the listing sold.
		now := s.now()
		listing.Status = repository.ListingSold
		listing.BuyerGuildID = &buyerGuildID
		listing.SoldAt = &now
		if err := tx.Save(&listing).Error; err != nil {
			return fmt.Errorf("mark listing sold: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &listing, nil
}

// dailySpent returns how much the guild has spent today (0 if no row yet).
func (s *ListingService) dailySpent(tx *gorm.DB, guildID uint64) (int64, error) {
	var dpt repository.DailyPurchaseTotal
	err := tx.Where("guild_id = ? AND day = ?", guildID, s.today()).First(&dpt).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read daily total: %w", err)
	}
	return dpt.TotalSpent, nil
}

// addDailySpent increments the guild's spend for today (upsert).
func (s *ListingService) addDailySpent(tx *gorm.DB, guildID uint64, amount int64) error {
	row := repository.DailyPurchaseTotal{GuildID: guildID, Day: s.today(), TotalSpent: amount}
	err := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "guild_id"}, {Name: "day"}},
		DoUpdates: clause.Assignments(map[string]any{"total_spent": gorm.Expr("daily_purchase_totals.total_spent + ?", amount)}),
	}).Create(&row).Error
	if err != nil {
		return fmt.Errorf("update daily total: %w", err)
	}
	return nil
}

func (s *ListingService) today() time.Time {
	n := s.now().UTC()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
}

// lockItem loads and row-locks an item.
func lockItem(tx *gorm.DB, itemID uint64) (*repository.Item, error) {
	var item repository.Item
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&item, itemID).Error; err != nil {
		return nil, notFoundOr(err, "lock item")
	}
	return &item, nil
}

// lockWallet row-locks a guild's wallet (used to serialize a guild's purchases).
func lockWallet(tx *gorm.DB, guildID uint64) error {
	var w repository.Wallet
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("guild_id = ?", guildID).First(&w).Error; err != nil {
		return fmt.Errorf("lock wallet for guild %d: %w", guildID, err)
	}
	return nil
}

// notFoundOr maps gorm's not-found to ErrNotFound, otherwise wraps with context.
func notFoundOr(err error, ctx string) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return fmt.Errorf("%s: %w", ctx, err)
}
