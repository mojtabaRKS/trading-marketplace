package service

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/herotech/market-dragon/internal/repository"
)

// AuctionService implements the Legendary-only auction flow: starting an
// auction, bidding (with fund reservation, outbid release, and anti-snipe
// extension), and cancelling a non-winning bid. Every mutation runs in a single
// transaction with the auction row locked, so bids on one auction are serialized.
type AuctionService struct {
	db        *gorm.DB
	wallets   *WalletService
	window    time.Duration
	extension time.Duration
	now       func() time.Time
}

// NewAuctionService builds an AuctionService with the configured auction window
// and anti-snipe extension.
func NewAuctionService(db *gorm.DB, wallets *WalletService, window, extension time.Duration) *AuctionService {
	return &AuctionService{db: db, wallets: wallets, window: window, extension: extension, now: time.Now}
}

// CreateAuction opens an auction for a seller-owned Legendary item. Only one
// active auction may exist per item (guarded by the item lock + status and the
// partial unique index).
func (s *AuctionService) CreateAuction(ctx context.Context, sellerGuildID, itemID uint64) (*repository.Auction, error) {
	var auction repository.Auction
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		item, err := lockItem(tx, itemID)
		if err != nil {
			return err
		}
		if item.OwnerGuildID != sellerGuildID {
			return ErrItemNotOwned
		}
		if item.Tier != repository.TierLegendary {
			return ErrNotLegendary
		}
		if item.Status == repository.ItemInAuction {
			return ErrActiveAuctionExists
		}
		if item.Status != repository.ItemAvailable {
			return ErrItemNotAvailable
		}

		now := s.now()
		auction = repository.Auction{
			ItemID:        itemID,
			SellerGuildID: sellerGuildID,
			Status:        repository.AuctionActive,
			StartsAt:      now,
			EndsAt:        now.Add(s.window),
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if err := tx.Create(&auction).Error; err != nil {
			return fmt.Errorf("create auction: %w", err)
		}

		item.Status = repository.ItemInAuction
		if err := tx.Save(item).Error; err != nil {
			return fmt.Errorf("mark item in auction: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &auction, nil
}

// PlaceBid reserves the bid amount, releases the previous highest bidder's
// reserve, records the bid, and applies the anti-snipe extension — atomically.
func (s *AuctionService) PlaceBid(ctx context.Context, auctionID, bidderGuildID uint64, amount int64) (*repository.Bid, error) {
	var bid repository.Bid
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		auction, err := lockAuction(tx, auctionID)
		if err != nil {
			return err
		}
		if auction.Status != repository.AuctionActive {
			return ErrAuctionNotActive
		}
		now := s.now()
		if AuctionEnded(now, auction.EndsAt) {
			return ErrAuctionEnded
		}
		if err := EnsureNotSelfBid(auction.SellerGuildID, bidderGuildID); err != nil {
			return err
		}

		var current int64
		var prev *repository.Bid
		if auction.HighestBidID != nil {
			var p repository.Bid
			if err := tx.First(&p, *auction.HighestBidID).Error; err != nil {
				return notFoundOr(err, "load highest bid")
			}
			prev = &p
			current = p.Amount
		}
		if err := EnsureBidBeatsCurrent(current, amount); err != nil {
			return err
		}

		// Reserve the new bidder's funds, then release the outbid leader.
		if err := s.wallets.ReserveTx(tx, bidderGuildID, amount, repository.RefBid, auctionID); err != nil {
			return err
		}
		if prev != nil {
			if err := s.wallets.ReleaseTx(tx, prev.BidderGuildID, prev.Amount, repository.RefBid, auctionID); err != nil {
				return err
			}
			prev.Status = repository.BidReleased
			if err := tx.Save(prev).Error; err != nil {
				return fmt.Errorf("release previous bid: %w", err)
			}
		}

		bid = repository.Bid{
			AuctionID:     auctionID,
			BidderGuildID: bidderGuildID,
			Amount:        amount,
			Status:        repository.BidActive,
			CreatedAt:     now,
		}
		if err := tx.Create(&bid).Error; err != nil {
			return fmt.Errorf("create bid: %w", err)
		}

		auction.HighestBidID = &bid.ID
		if newEnd, extended := MaybeExtend(now, auction.EndsAt, s.extension, s.extension); extended {
			auction.EndsAt = newEnd
		}
		auction.UpdatedAt = now
		if err := tx.Save(auction).Error; err != nil {
			return fmt.Errorf("update auction: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &bid, nil
}

// CancelBid withdraws a bidder's own bid. The current highest bid cannot be
// cancelled. Non-winning bids already had their funds released when outbid; this
// marks them cancelled (and releases any still-reserved amount defensively).
func (s *AuctionService) CancelBid(ctx context.Context, auctionID, bidID, bidderGuildID uint64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		auction, err := lockAuction(tx, auctionID)
		if err != nil {
			return err
		}
		if auction.Status != repository.AuctionActive {
			return ErrAuctionNotActive
		}

		var bid repository.Bid
		if err := tx.First(&bid, bidID).Error; err != nil {
			return notFoundOr(err, "load bid")
		}
		if bid.AuctionID != auctionID || bid.BidderGuildID != bidderGuildID {
			return ErrNotFound
		}

		isHighest := auction.HighestBidID != nil && *auction.HighestBidID == bidID
		if err := EnsureCanCancelBid(isHighest); err != nil {
			return err
		}

		if bid.Status == repository.BidActive {
			if err := s.wallets.ReleaseTx(tx, bidderGuildID, bid.Amount, repository.RefBid, auctionID); err != nil {
				return err
			}
		}
		bid.Status = repository.BidCancelled
		if err := tx.Save(&bid).Error; err != nil {
			return fmt.Errorf("cancel bid: %w", err)
		}
		return nil
	})
}

// GetAuction returns an auction by ID.
func (s *AuctionService) GetAuction(ctx context.Context, auctionID uint64) (*repository.Auction, error) {
	var a repository.Auction
	if err := s.db.WithContext(ctx).First(&a, auctionID).Error; err != nil {
		return nil, notFoundOr(err, "load auction")
	}
	return &a, nil
}

// HighestBid returns the current highest (active) bid, or nil if none.
func (s *AuctionService) HighestBid(ctx context.Context, auctionID uint64) (*repository.Bid, error) {
	a, err := s.GetAuction(ctx, auctionID)
	if err != nil {
		return nil, err
	}
	if a.HighestBidID == nil {
		return nil, nil
	}
	var b repository.Bid
	if err := s.db.WithContext(ctx).First(&b, *a.HighestBidID).Error; err != nil {
		return nil, notFoundOr(err, "load highest bid")
	}
	return &b, nil
}

// ListBids returns an auction's bids, newest first.
func (s *AuctionService) ListBids(ctx context.Context, auctionID uint64) ([]repository.Bid, error) {
	var bids []repository.Bid
	if err := s.db.WithContext(ctx).
		Where("auction_id = ?", auctionID).
		Order("created_at DESC, id DESC").
		Find(&bids).Error; err != nil {
		return nil, fmt.Errorf("list bids: %w", err)
	}
	return bids, nil
}

// lockAuction loads and row-locks an auction.
func lockAuction(tx *gorm.DB, auctionID uint64) (*repository.Auction, error) {
	var a repository.Auction
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&a, auctionID).Error; err != nil {
		return nil, notFoundOr(err, "lock auction")
	}
	return &a, nil
}
