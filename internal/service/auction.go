package service

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/herotech/market-dragon/internal/model"
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
func (s *AuctionService) CreateAuction(ctx context.Context, sellerGuildID, itemID uint64) (*model.Auction, error) {
	var auction model.Auction
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		item, err := lockItem(tx, itemID)
		if err != nil {
			return err
		}
		if item.OwnerGuildID != sellerGuildID {
			return ErrItemNotOwned
		}
		if item.Tier != model.TierLegendary {
			return ErrNotLegendary
		}
		if item.Status == model.ItemInAuction {
			return ErrActiveAuctionExists
		}
		if item.Status != model.ItemAvailable {
			return ErrItemNotAvailable
		}

		now := s.now()
		auction = model.Auction{
			ItemID:        itemID,
			SellerGuildID: sellerGuildID,
			Status:        model.AuctionActive,
			StartsAt:      now,
			EndsAt:        now.Add(s.window),
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if err := tx.Create(&auction).Error; err != nil {
			return fmt.Errorf("create auction: %w", err)
		}

		item.Status = model.ItemInAuction
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
func (s *AuctionService) PlaceBid(ctx context.Context, auctionID, bidderGuildID uint64, amount int64) (*model.Bid, error) {
	var bid model.Bid
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		auction, err := lockAuction(tx, auctionID)
		if err != nil {
			return err
		}
		if auction.Status != model.AuctionActive {
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
		var prev *model.Bid
		if auction.HighestBidID != nil {
			var p model.Bid
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
		if err := s.wallets.ReserveTx(tx, bidderGuildID, amount, model.RefBid, auctionID); err != nil {
			return err
		}
		if prev != nil {
			if err := s.wallets.ReleaseTx(tx, prev.BidderGuildID, prev.Amount, model.RefBid, auctionID); err != nil {
				return err
			}
			prev.Status = model.BidReleased
			if err := tx.Save(prev).Error; err != nil {
				return fmt.Errorf("release previous bid: %w", err)
			}
		}

		bid = model.Bid{
			AuctionID:     auctionID,
			BidderGuildID: bidderGuildID,
			Amount:        amount,
			Status:        model.BidActive,
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
		if auction.Status != model.AuctionActive {
			return ErrAuctionNotActive
		}

		var bid model.Bid
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

		if bid.Status == model.BidActive {
			if err := s.wallets.ReleaseTx(tx, bidderGuildID, bid.Amount, model.RefBid, auctionID); err != nil {
				return err
			}
		}
		bid.Status = model.BidCancelled
		if err := tx.Save(&bid).Error; err != nil {
			return fmt.Errorf("cancel bid: %w", err)
		}
		return nil
	})
}

// SettleDue finds every active auction whose window has closed and settles each
// in its own transaction. It returns the number actually settled. Safe to run
// from multiple workers: each settlement locks its auction row.
func (s *AuctionService) SettleDue(ctx context.Context) (int, error) {
	var ids []uint64
	if err := s.db.WithContext(ctx).
		Model(&model.Auction{}).
		Where("status = ? AND ends_at <= ?", model.AuctionActive, s.now()).
		Pluck("id", &ids).Error; err != nil {
		return 0, fmt.Errorf("find due auctions: %w", err)
	}
	settled := 0
	for _, id := range ids {
		ok, err := s.SettleAuction(ctx, id)
		if err != nil {
			return settled, fmt.Errorf("settle auction %d: %w", id, err)
		}
		if ok {
			settled++
		}
	}
	return settled, nil
}

// SettleAuction closes a single ended auction atomically and idempotently. If
// there is a highest bid, the winner's reserved funds are converted to a spend,
// the seller is credited, and the item is transferred; otherwise the item simply
// returns to Available. Returns false (no error) when the auction is not active
// or has not ended yet, so retries and concurrent workers are no-ops.
func (s *AuctionService) SettleAuction(ctx context.Context, auctionID uint64) (bool, error) {
	settled := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		auction, err := lockAuction(tx, auctionID)
		if err != nil {
			return err
		}
		if auction.Status != model.AuctionActive {
			return nil // already settled/cancelled — idempotent no-op
		}
		if !AuctionEnded(s.now(), auction.EndsAt) {
			return nil // window still open
		}

		item, err := lockItem(tx, auction.ItemID)
		if err != nil {
			return err
		}

		if auction.HighestBidID != nil {
			var win model.Bid
			if err := tx.First(&win, *auction.HighestBidID).Error; err != nil {
				return notFoundOr(err, "load winning bid")
			}
			// Winner pays out of the funds reserved at bid time; seller is paid.
			if err := s.wallets.SettleReservedTx(tx, win.BidderGuildID, win.Amount, model.RefAuction, auctionID); err != nil {
				return err
			}
			if err := s.wallets.CreditTx(tx, auction.SellerGuildID, win.Amount, model.RefAuction, auctionID); err != nil {
				return err
			}
			item.OwnerGuildID = win.BidderGuildID
			item.Status = model.ItemAvailable
			win.Status = model.BidWon
			if err := tx.Save(&win).Error; err != nil {
				return fmt.Errorf("mark bid won: %w", err)
			}
			winner := win.BidderGuildID
			auction.WinnerGuildID = &winner
		} else {
			// No bids: the legendary is available again.
			item.Status = model.ItemAvailable
		}

		auction.Status = model.AuctionSettled
		auction.UpdatedAt = s.now()
		if err := tx.Save(item).Error; err != nil {
			return fmt.Errorf("update item: %w", err)
		}
		if err := tx.Save(auction).Error; err != nil {
			return fmt.Errorf("settle auction: %w", err)
		}
		settled = true
		return nil
	})
	return settled, err
}

// ActiveAuctionByItem returns the current active auction for an item, or
// ErrNotFound if the item has no active auction.
func (s *AuctionService) ActiveAuctionByItem(ctx context.Context, itemID uint64) (*model.Auction, error) {
	var a model.Auction
	err := s.db.WithContext(ctx).
		Where("item_id = ? AND status = ?", itemID, model.AuctionActive).
		Order("id DESC").
		First(&a).Error
	if err != nil {
		return nil, notFoundOr(err, "load active auction")
	}
	return &a, nil
}

// ListActiveAuctions returns all currently active auctions, newest first.
func (s *AuctionService) ListActiveAuctions(ctx context.Context) ([]model.Auction, error) {
	var auctions []model.Auction
	if err := s.db.WithContext(ctx).
		Where("status = ?", model.AuctionActive).
		Order("id DESC").
		Find(&auctions).Error; err != nil {
		return nil, fmt.Errorf("list active auctions: %w", err)
	}
	return auctions, nil
}

// PlaceBidOnItem resolves the item's active auction and places a bid on it.
func (s *AuctionService) PlaceBidOnItem(ctx context.Context, itemID, bidderGuildID uint64, amount int64) (*model.Bid, error) {
	a, err := s.ActiveAuctionByItem(ctx, itemID)
	if err != nil {
		return nil, err
	}
	return s.PlaceBid(ctx, a.ID, bidderGuildID, amount)
}

// CancelBidOnItem resolves the item's active auction and cancels the given bid.
func (s *AuctionService) CancelBidOnItem(ctx context.Context, itemID, bidID, bidderGuildID uint64) error {
	a, err := s.ActiveAuctionByItem(ctx, itemID)
	if err != nil {
		return err
	}
	return s.CancelBid(ctx, a.ID, bidID, bidderGuildID)
}

// GetAuction returns an auction by ID.
func (s *AuctionService) GetAuction(ctx context.Context, auctionID uint64) (*model.Auction, error) {
	var a model.Auction
	if err := s.db.WithContext(ctx).First(&a, auctionID).Error; err != nil {
		return nil, notFoundOr(err, "load auction")
	}
	return &a, nil
}

// HighestBid returns the current highest (active) bid, or nil if none.
func (s *AuctionService) HighestBid(ctx context.Context, auctionID uint64) (*model.Bid, error) {
	a, err := s.GetAuction(ctx, auctionID)
	if err != nil {
		return nil, err
	}
	if a.HighestBidID == nil {
		return nil, nil
	}
	var b model.Bid
	if err := s.db.WithContext(ctx).First(&b, *a.HighestBidID).Error; err != nil {
		return nil, notFoundOr(err, "load highest bid")
	}
	return &b, nil
}

// ListBids returns an auction's bids, newest first.
func (s *AuctionService) ListBids(ctx context.Context, auctionID uint64) ([]model.Bid, error) {
	var bids []model.Bid
	if err := s.db.WithContext(ctx).
		Where("auction_id = ?", auctionID).
		Order("created_at DESC, id DESC").
		Find(&bids).Error; err != nil {
		return nil, fmt.Errorf("list bids: %w", err)
	}
	return bids, nil
}

// lockAuction loads and row-locks an auction.
func lockAuction(tx *gorm.DB, auctionID uint64) (*model.Auction, error) {
	var a model.Auction
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&a, auctionID).Error; err != nil {
		return nil, notFoundOr(err, "lock auction")
	}
	return &a, nil
}
