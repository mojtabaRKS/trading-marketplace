//go:build integration

package service

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/herotech/market-dragon/internal/model"
)

func expireAuction(t *testing.T, db *gorm.DB, auctionID uint64) {
	t.Helper()
	if err := db.Model(&model.Auction{}).
		Where("id = ?", auctionID).
		Update("ends_at", time.Now().Add(-time.Hour)).Error; err != nil {
		t.Fatalf("expire auction: %v", err)
	}
}

func walletOf(t *testing.T, db *gorm.DB, guildID uint64) model.Wallet {
	t.Helper()
	var w model.Wallet
	if err := db.Where("guild_id = ?", guildID).First(&w).Error; err != nil {
		t.Fatalf("load wallet %d: %v", guildID, err)
	}
	return w
}

// TestSettleAuctionWithWinner settles an ended auction: the winner pays from
// reserved funds, the seller is credited, the item transfers, and re-settling is
// a no-op.
func TestSettleAuctionWithWinner(t *testing.T) {
	db := setupAuction(t)
	svc := NewAuctionService(db, NewWalletService(db), 24*time.Hour, 5*time.Minute)
	ctx := context.Background()

	auction, err := svc.CreateAuction(ctx, auSeller, auItem)
	if err != nil {
		t.Fatalf("create auction: %v", err)
	}
	const bid = 1200
	if _, err := svc.PlaceBid(ctx, auction.ID, auBidderA, bid); err != nil {
		t.Fatalf("bid: %v", err)
	}

	expireAuction(t, db, auction.ID)
	settled, err := svc.SettleAuction(ctx, auction.ID)
	if err != nil {
		t.Fatalf("settle: %v", err)
	}
	if !settled {
		t.Fatal("expected auction to settle")
	}

	// Winner paid from reserved funds.
	winner := walletOf(t, db, auBidderA)
	if winner.ReservedAmount != 0 {
		t.Fatalf("winner reserved = %d, want 0", winner.ReservedAmount)
	}
	if winner.TotalBalance != 1_000_000-bid {
		t.Fatalf("winner total = %d, want %d", winner.TotalBalance, 1_000_000-bid)
	}
	// Seller credited.
	seller := walletOf(t, db, auSeller)
	if seller.TotalBalance != bid {
		t.Fatalf("seller total = %d, want %d", seller.TotalBalance, bid)
	}
	// Item transferred and available again.
	var item model.Item
	db.First(&item, auItem)
	if item.OwnerGuildID != auBidderA || item.Status != model.ItemAvailable {
		t.Fatalf("item owner=%d status=%s, want owner=%d available", item.OwnerGuildID, item.Status, auBidderA)
	}
	// Auction + bid marked terminal.
	var got model.Auction
	db.First(&got, auction.ID)
	if got.Status != model.AuctionSettled || got.WinnerGuildID == nil || *got.WinnerGuildID != auBidderA {
		t.Fatalf("auction status=%s winner=%v, want settled winner=%d", got.Status, got.WinnerGuildID, auBidderA)
	}

	// Idempotent: settling again changes nothing.
	settled, err = svc.SettleAuction(ctx, auction.ID)
	if err != nil {
		t.Fatalf("re-settle: %v", err)
	}
	if settled {
		t.Fatal("second settle should be a no-op")
	}
	if w := walletOf(t, db, auBidderA); w.TotalBalance != 1_000_000-bid {
		t.Fatalf("winner total changed on re-settle: %d", w.TotalBalance)
	}
	if w := walletOf(t, db, auSeller); w.TotalBalance != bid {
		t.Fatalf("seller total changed on re-settle: %d", w.TotalBalance)
	}
}

// TestSettleAuctionNoBids returns the item to Available when no one bid.
func TestSettleAuctionNoBids(t *testing.T) {
	db := setupAuction(t)
	svc := NewAuctionService(db, NewWalletService(db), 24*time.Hour, 5*time.Minute)
	ctx := context.Background()

	auction, err := svc.CreateAuction(ctx, auSeller, auItem)
	if err != nil {
		t.Fatalf("create auction: %v", err)
	}
	expireAuction(t, db, auction.ID)

	settled, err := svc.SettleAuction(ctx, auction.ID)
	if err != nil {
		t.Fatalf("settle: %v", err)
	}
	if !settled {
		t.Fatal("expected auction to settle")
	}

	var item model.Item
	db.First(&item, auItem)
	if item.OwnerGuildID != auSeller || item.Status != model.ItemAvailable {
		t.Fatalf("item owner=%d status=%s, want owner=%d available", item.OwnerGuildID, item.Status, auSeller)
	}
	var got model.Auction
	db.First(&got, auction.ID)
	if got.Status != model.AuctionSettled || got.WinnerGuildID != nil {
		t.Fatalf("auction status=%s winner=%v, want settled no-winner", got.Status, got.WinnerGuildID)
	}
}

// TestSettleDueSkipsOpenAuctions only settles auctions past their window.
func TestSettleDueSkipsOpenAuctions(t *testing.T) {
	db := setupAuction(t)
	svc := NewAuctionService(db, NewWalletService(db), 24*time.Hour, 5*time.Minute)
	ctx := context.Background()

	auction, err := svc.CreateAuction(ctx, auSeller, auItem)
	if err != nil {
		t.Fatalf("create auction: %v", err)
	}

	// Still open (24h window) -> nothing due.
	n, err := svc.SettleDue(ctx)
	if err != nil {
		t.Fatalf("settle due: %v", err)
	}
	if n != 0 {
		t.Fatalf("settled %d open auctions, want 0", n)
	}

	expireAuction(t, db, auction.ID)
	n, err = svc.SettleDue(ctx)
	if err != nil {
		t.Fatalf("settle due: %v", err)
	}
	if n != 1 {
		t.Fatalf("settled %d, want 1", n)
	}
}
