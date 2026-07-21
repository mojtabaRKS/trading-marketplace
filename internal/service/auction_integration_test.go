//go:build integration

package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/herotech/market-dragon/internal/config"
	"github.com/herotech/market-dragon/internal/infra/database"
	"github.com/herotech/market-dragon/internal/model"
)

const (
	auSeller  = 9201
	auBidderA = 9202
	auBidderB = 9203
	auItem    = 9210
)

func setupAuction(t *testing.T) *gorm.DB {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if err := database.Migrate(cfg.DatabaseURL()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db, err := database.Open(cfg.DSN())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	ids := []uint64{auSeller, auBidderA, auBidderB}
	db.Exec("DELETE FROM bids WHERE bidder_guild_id IN ?", ids)
	db.Exec("DELETE FROM auctions WHERE seller_guild_id = ?", auSeller)
	db.Exec("DELETE FROM wallet_transactions WHERE guild_id IN ?", ids)
	db.Exec("DELETE FROM items WHERE id = ? OR owner_guild_id IN ?", auItem, ids)
	db.Exec("DELETE FROM wallets WHERE guild_id IN ?", ids)
	db.Exec("DELETE FROM guilds WHERE id IN ?", ids)

	guilds := []model.Guild{
		{ID: auSeller, Name: "AU-Seller"},
		{ID: auBidderA, Name: "AU-BidderA"},
		{ID: auBidderB, Name: "AU-BidderB"},
	}
	wallets := []model.Wallet{
		{ID: auSeller, GuildID: auSeller, TotalBalance: 0},
		{ID: auBidderA, GuildID: auBidderA, TotalBalance: 1_000_000},
		{ID: auBidderB, GuildID: auBidderB, TotalBalance: 1_000_000},
	}
	for i := range guilds {
		if err := db.Create(&guilds[i]).Error; err != nil {
			t.Fatalf("create guild: %v", err)
		}
	}
	for i := range wallets {
		if err := db.Create(&wallets[i]).Error; err != nil {
			t.Fatalf("create wallet: %v", err)
		}
	}
	item := model.Item{
		ID: auItem, Name: "Dragon Blade", Tier: model.TierLegendary,
		OwnerGuildID: auSeller, Status: model.ItemAvailable, Stock: 1,
	}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("create item: %v", err)
	}
	return db
}

func reservedOf(t *testing.T, db *gorm.DB, guildID uint64) int64 {
	t.Helper()
	var w model.Wallet
	if err := db.Where("guild_id = ?", guildID).First(&w).Error; err != nil {
		t.Fatalf("load wallet %d: %v", guildID, err)
	}
	return w.ReservedAmount
}

// TestAuctionBidFlow covers create rules, the +5% rule, reserve/outbid-release,
// self-bid rejection, and cancel restrictions.
func TestAuctionBidFlow(t *testing.T) {
	db := setupAuction(t)
	svc := NewAuctionService(db, NewWalletService(db), 24*time.Hour, 5*time.Minute)
	ctx := context.Background()

	auction, err := svc.CreateAuction(ctx, auSeller, auItem)
	if err != nil {
		t.Fatalf("create auction: %v", err)
	}

	// A second auction on the same (now in-auction) item must be rejected.
	if _, err := svc.CreateAuction(ctx, auSeller, auItem); !errors.Is(err, ErrActiveAuctionExists) {
		t.Fatalf("second auction = %v, want ErrActiveAuctionExists", err)
	}

	// Seller cannot bid on their own auction.
	if _, err := svc.PlaceBid(ctx, auction.ID, auSeller, 1000); !errors.Is(err, ErrSelfBid) {
		t.Fatalf("self-bid = %v, want ErrSelfBid", err)
	}

	// First valid bid reserves funds.
	bidA, err := svc.PlaceBid(ctx, auction.ID, auBidderA, 1000)
	if err != nil {
		t.Fatalf("bid A: %v", err)
	}
	if r := reservedOf(t, db, auBidderA); r != 1000 {
		t.Fatalf("A reserved = %d, want 1000", r)
	}

	// A bid below the +5% minimum is rejected (1000 -> min 1050).
	if _, err := svc.PlaceBid(ctx, auction.ID, auBidderB, 1049); !errors.Is(err, ErrBidTooLow) {
		t.Fatalf("low bid = %v, want ErrBidTooLow", err)
	}

	// A valid higher bid reserves B and releases A.
	bidB, err := svc.PlaceBid(ctx, auction.ID, auBidderB, 1050)
	if err != nil {
		t.Fatalf("bid B: %v", err)
	}
	if r := reservedOf(t, db, auBidderA); r != 0 {
		t.Fatalf("A reserved after outbid = %d, want 0", r)
	}
	if r := reservedOf(t, db, auBidderB); r != 1050 {
		t.Fatalf("B reserved = %d, want 1050", r)
	}

	// Cancelling the current highest bid is forbidden.
	if err := svc.CancelBid(ctx, auction.ID, bidB.ID, auBidderB); !errors.Is(err, ErrCancelHighestBid) {
		t.Fatalf("cancel highest = %v, want ErrCancelHighestBid", err)
	}
	// Cancelling a non-winning bid is allowed.
	if err := svc.CancelBid(ctx, auction.ID, bidA.ID, auBidderA); err != nil {
		t.Fatalf("cancel non-highest: %v", err)
	}
	var cancelled model.Bid
	db.First(&cancelled, bidA.ID)
	if cancelled.Status != model.BidCancelled {
		t.Fatalf("bid A status = %s, want cancelled", cancelled.Status)
	}
}

// TestBidConcurrentConsistency fires two competing bids at once. Whatever the
// interleaving, exactly one ends up highest, only that bidder's funds stay
// reserved, and the loser is never over-committed.
func TestBidConcurrentConsistency(t *testing.T) {
	db := setupAuction(t)
	svc := NewAuctionService(db, NewWalletService(db), 24*time.Hour, 5*time.Minute)
	ctx := context.Background()

	auction, err := svc.CreateAuction(ctx, auSeller, auItem)
	if err != nil {
		t.Fatalf("create auction: %v", err)
	}

	const lowBid, highBid = 1000, 2000
	var wg sync.WaitGroup
	errsCh := make(chan error, 2)
	bids := map[uint64]int64{auBidderA: lowBid, auBidderB: highBid}
	for bidder, amount := range bids {
		wg.Add(1)
		go func(bidder uint64, amount int64) {
			defer wg.Done()
			_, err := svc.PlaceBid(ctx, auction.ID, bidder, amount)
			errsCh <- err
		}(bidder, amount)
	}
	wg.Wait()
	close(errsCh)

	// Any error must be an expected business rejection (loser placed too low).
	for err := range errsCh {
		if err != nil && !errors.Is(err, ErrBidTooLow) {
			t.Fatalf("unexpected bid error: %v", err)
		}
	}

	// The high bid must be the winner; the low bidder holds no reserve.
	if r := reservedOf(t, db, auBidderB); r != highBid {
		t.Fatalf("high bidder reserved = %d, want %d", r, highBid)
	}
	if r := reservedOf(t, db, auBidderA); r != 0 {
		t.Fatalf("low bidder reserved = %d, want 0 (no over-commit)", r)
	}

	var got model.Auction
	db.First(&got, auction.ID)
	if got.HighestBidID == nil {
		t.Fatal("auction has no highest bid")
	}
	var highest model.Bid
	db.First(&highest, *got.HighestBidID)
	if highest.BidderGuildID != auBidderB || highest.Amount != highBid {
		t.Fatalf("highest bid = guild %d amount %d, want guild %d amount %d",
			highest.BidderGuildID, highest.Amount, auBidderB, highBid)
	}
}

// TestAuctionAntiSnipeExtends verifies a bid inside the extension window pushes
// the end time out.
func TestAuctionAntiSnipeExtends(t *testing.T) {
	db := setupAuction(t)
	extension := 5 * time.Minute
	svc := NewAuctionService(db, NewWalletService(db), 24*time.Hour, extension)
	ctx := context.Background()

	auction, err := svc.CreateAuction(ctx, auSeller, auItem)
	if err != nil {
		t.Fatalf("create auction: %v", err)
	}

	// Force the auction to be ending in 2 minutes (inside the snipe window).
	soon := time.Now().Add(2 * time.Minute)
	db.Model(&model.Auction{}).Where("id = ?", auction.ID).Update("ends_at", soon)

	before := time.Now()
	if _, err := svc.PlaceBid(ctx, auction.ID, auBidderA, 1000); err != nil {
		t.Fatalf("bid: %v", err)
	}

	var got model.Auction
	db.First(&got, auction.ID)
	// End time should now be ~ now + extension, i.e. well beyond the old 2 min.
	if !got.EndsAt.After(before.Add(4 * time.Minute)) {
		t.Fatalf("ends_at = %v, want extended by ~%s from now", got.EndsAt, extension)
	}
}
