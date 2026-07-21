//go:build integration

package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"gorm.io/gorm"

	"github.com/herotech/market-dragon/internal/config"
	"github.com/herotech/market-dragon/internal/infra/database"
	"github.com/herotech/market-dragon/internal/model"
)

const (
	itSeller = 9101
	itBuyerA = 9102
	itBuyerB = 9103
	itItem   = 9110
)

func setupMarket(t *testing.T, sellerStock int, buyerCap int64) *gorm.DB {
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

	ids := []uint64{itSeller, itBuyerA, itBuyerB}
	db.Exec("DELETE FROM wallet_transactions WHERE guild_id IN ?", ids)
	db.Exec("DELETE FROM daily_purchase_totals WHERE guild_id IN ?", ids)
	db.Exec("DELETE FROM listings WHERE seller_guild_id = ?", itSeller)
	db.Exec("DELETE FROM items WHERE id = ? OR owner_guild_id IN ?", itItem, ids)
	db.Exec("DELETE FROM wallets WHERE guild_id IN ?", ids)
	db.Exec("DELETE FROM guilds WHERE id IN ?", ids)

	guilds := []model.Guild{
		{ID: itSeller, Name: "IT-Seller"},
		{ID: itBuyerA, Name: "IT-BuyerA", DailyPurchaseCap: buyerCap},
		{ID: itBuyerB, Name: "IT-BuyerB", DailyPurchaseCap: buyerCap},
	}
	wallets := []model.Wallet{
		{ID: itSeller, GuildID: itSeller, TotalBalance: 0},
		{ID: itBuyerA, GuildID: itBuyerA, TotalBalance: 1_000_000},
		{ID: itBuyerB, GuildID: itBuyerB, TotalBalance: 1_000_000},
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
		ID: itItem, Name: "Elven Bow", Tier: model.TierRare,
		OwnerGuildID: itSeller, Status: model.ItemAvailable, Stock: sellerStock,
	}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("create item: %v", err)
	}
	return db
}

// TestBuyConcurrentSellsOnce lists an item and fires two concurrent buys; exactly
// one must succeed and the money must move once.
func TestBuyConcurrentSellsOnce(t *testing.T) {
	const price = 1000
	db := setupMarket(t, 1, 0) // unlimited cap
	wallets := NewWalletService(db)
	listings := NewListingService(db, wallets)
	ctx := context.Background()

	listing, err := listings.CreateListing(ctx, itSeller, itItem, price)
	if err != nil {
		t.Fatalf("create listing: %v", err)
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var success, conflict int
	buyers := []uint64{itBuyerA, itBuyerB}
	for _, b := range buyers {
		wg.Add(1)
		go func(buyer uint64) {
			defer wg.Done()
			_, err := listings.Buy(ctx, buyer, listing.ID)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				success++
			case errors.Is(err, ErrListingNotOpen):
				conflict++
			default:
				t.Errorf("unexpected buy error: %v", err)
			}
		}(b)
	}
	wg.Wait()

	if success != 1 || conflict != 1 {
		t.Fatalf("want 1 success + 1 conflict, got success=%d conflict=%d", success, conflict)
	}

	// Seller credited exactly once.
	var seller model.Wallet
	db.Where("guild_id = ?", itSeller).First(&seller)
	if seller.TotalBalance != price {
		t.Fatalf("seller balance = %d, want %d", seller.TotalBalance, price)
	}

	// Listing is sold.
	var got model.Listing
	db.First(&got, listing.ID)
	if got.Status != model.ListingSold {
		t.Fatalf("listing status = %s, want sold", got.Status)
	}
}

// TestBuyDailyCapEnforced rejects a purchase that would exceed the daily cap.
func TestBuyDailyCapEnforced(t *testing.T) {
	const price = 1000
	db := setupMarket(t, 5, 1500) // cap allows one 1000 buy, not two
	wallets := NewWalletService(db)
	listings := NewListingService(db, wallets)
	ctx := context.Background()

	l1, err := listings.CreateListing(ctx, itSeller, itItem, price)
	if err != nil {
		t.Fatalf("create listing 1: %v", err)
	}
	if _, err := listings.Buy(ctx, itBuyerA, l1.ID); err != nil {
		t.Fatalf("first buy should succeed: %v", err)
	}

	l2, err := listings.CreateListing(ctx, itSeller, itItem, price)
	if err != nil {
		t.Fatalf("create listing 2: %v", err)
	}
	if _, err := listings.Buy(ctx, itBuyerA, l2.ID); !errors.Is(err, ErrDailyCapExceeded) {
		t.Fatalf("second buy = %v, want ErrDailyCapExceeded", err)
	}
}

// TestListLegendaryRejected ensures legendary items cannot be listed.
func TestListLegendaryRejected(t *testing.T) {
	db := setupMarket(t, 1, 0)
	db.Model(&model.Item{}).Where("id = ?", itItem).Update("tier", model.TierLegendary)
	listings := NewListingService(db, NewWalletService(db))
	if _, err := listings.CreateListing(context.Background(), itSeller, itItem, 100); !errors.Is(err, ErrLegendaryNeedsAuction) {
		t.Fatalf("listing legendary = %v, want ErrLegendaryNeedsAuction", err)
	}
}
