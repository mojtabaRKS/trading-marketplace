//go:build integration

// Integration tests for WalletService. Requires a running PostgreSQL (see .env /
// docker compose). Run with: go test -tags integration -race ./internal/service/...
package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"gorm.io/gorm"

	"github.com/herotech/market-dragon/internal/config"
	"github.com/herotech/market-dragon/internal/infra/database"
	"github.com/herotech/market-dragon/internal/repository"
)

const itGuildID = 9001

func setupWallet(t *testing.T, total int64) *gorm.DB {
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

	// Reset any state from a previous run.
	db.Exec("DELETE FROM wallet_transactions WHERE guild_id = ?", itGuildID)
	db.Exec("DELETE FROM wallets WHERE guild_id = ?", itGuildID)
	db.Exec("DELETE FROM guilds WHERE id = ?", itGuildID)

	if err := db.Create(&repository.Guild{ID: itGuildID, Name: "IT-Wallet"}).Error; err != nil {
		t.Fatalf("create guild: %v", err)
	}
	if err := db.Create(&repository.Wallet{ID: itGuildID, GuildID: itGuildID, TotalBalance: total}).Error; err != nil {
		t.Fatalf("create wallet: %v", err)
	}
	return db
}

func loadWallet(t *testing.T, db *gorm.DB) repository.Wallet {
	t.Helper()
	var w repository.Wallet
	if err := db.Where("guild_id = ?", itGuildID).First(&w).Error; err != nil {
		t.Fatalf("load wallet: %v", err)
	}
	return w
}

// TestWalletConcurrentReserveNoOvercommit runs many concurrent reserves and
// asserts the wallet is never over-committed (available never goes negative).
func TestWalletConcurrentReserveNoOvercommit(t *testing.T) {
	const total, amount, workers = 1000, 100, 20 // capacity for exactly 10 reserves
	db := setupWallet(t, total)
	svc := NewWalletService(db)
	ctx := context.Background()

	var wg sync.WaitGroup
	var mu sync.Mutex
	var success, insufficient int

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := svc.Reserve(ctx, itGuildID, amount, "test", 0)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				success++
			case errors.Is(err, ErrInsufficientFunds):
				insufficient++
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()

	if success != total/amount {
		t.Fatalf("successful reserves = %d, want %d", success, total/amount)
	}
	if insufficient != workers-total/amount {
		t.Fatalf("insufficient rejects = %d, want %d", insufficient, workers-total/amount)
	}

	w := loadWallet(t, db)
	if w.ReservedAmount != int64(success*amount) {
		t.Fatalf("reserved = %d, want %d", w.ReservedAmount, success*amount)
	}
	if AvailableBalance(w.TotalBalance, w.ReservedAmount) < 0 {
		t.Fatalf("available went negative: total=%d reserved=%d", w.TotalBalance, w.ReservedAmount)
	}

	// Ledger reconciles: reserved == sum(reserve) - sum(release).
	if got := ledgerReserved(t, db); got != w.ReservedAmount {
		t.Fatalf("ledger reserved = %d, wallet reserved = %d", got, w.ReservedAmount)
	}
}

// TestWalletMovementsReconcile exercises the full set of movements and checks
// the ledger reconciles with the wallet balances.
func TestWalletMovementsReconcile(t *testing.T) {
	const initial = 1000
	db := setupWallet(t, initial)
	svc := NewWalletService(db)
	ctx := context.Background()

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("movement failed: %v", err)
		}
	}
	must(svc.Reserve(ctx, itGuildID, 400, "auction", 1))
	must(svc.Release(ctx, itGuildID, 100, "auction", 1))        // reserved 300
	must(svc.SettleReserved(ctx, itGuildID, 200, "auction", 1)) // total 800, reserved 100
	must(svc.Debit(ctx, itGuildID, 50, "listing", 2))           // total 750
	must(svc.Credit(ctx, itGuildID, 25, "listing", 3))          // total 775

	w := loadWallet(t, db)
	if w.TotalBalance != 775 {
		t.Fatalf("total = %d, want 775", w.TotalBalance)
	}
	if w.ReservedAmount != 100 {
		t.Fatalf("reserved = %d, want 100", w.ReservedAmount)
	}
	if got := ledgerReserved(t, db); got != w.ReservedAmount {
		t.Fatalf("ledger reserved = %d, wallet reserved = %d", got, w.ReservedAmount)
	}
	// total == initial + credit - debit - settled(debit)
	if got := initial + ledgerSum(t, db, repository.TxCredit) - ledgerSum(t, db, repository.TxDebit); got != w.TotalBalance {
		t.Fatalf("ledger total = %d, wallet total = %d", got, w.TotalBalance)
	}
}

func TestWalletReleaseExceedsReserved(t *testing.T) {
	db := setupWallet(t, 1000)
	svc := NewWalletService(db)
	if err := svc.Release(context.Background(), itGuildID, 50, "x", 0); !errors.Is(err, ErrInsufficientReserved) {
		t.Fatalf("release with no reserve = %v, want ErrInsufficientReserved", err)
	}
}

func ledgerReserved(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	// reserved effect = reserve - release - settled(recorded as debit against auction).
	return ledgerSum(t, db, repository.TxReserve) -
		ledgerSum(t, db, repository.TxRelease) -
		ledgerSumRef(t, db, repository.TxDebit, "auction")
}

func ledgerSum(t *testing.T, db *gorm.DB, txType string) int64 {
	t.Helper()
	var sum int64
	db.Model(&repository.WalletTransaction{}).
		Where("guild_id = ? AND type = ?", itGuildID, txType).
		Select("COALESCE(SUM(amount),0)").Scan(&sum)
	return sum
}

func ledgerSumRef(t *testing.T, db *gorm.DB, txType, refType string) int64 {
	t.Helper()
	var sum int64
	db.Model(&repository.WalletTransaction{}).
		Where("guild_id = ? AND type = ? AND ref_type = ?", itGuildID, txType, refType).
		Select("COALESCE(SUM(amount),0)").Scan(&sum)
	return sum
}
