//go:build integration

package service

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"gorm.io/gorm"

	"github.com/herotech/market-dragon/internal/config"
	"github.com/herotech/market-dragon/internal/infra/database"
	"github.com/herotech/market-dragon/internal/infra/oracle"
	"github.com/herotech/market-dragon/internal/model"
)

const orItem = 9310

func setupOracle(t *testing.T) *gorm.DB {
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
	db.Exec("DELETE FROM oracle_prices WHERE item_id = ?", orItem)
	return db
}

func countPrices(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	db.Model(&model.OraclePrice{}).Where("item_id = ?", orItem).Count(&n)
	return n
}

// TestOracleRefreshKeepsLastKnownGood proves bad values (zero/negative/slow/down)
// never overwrite a previously accepted price and are never stored.
func TestOracleRefreshKeepsLastKnownGood(t *testing.T) {
	db := setupOracle(t)
	src := oracle.NewMockSource(oracle.Price{ItemID: orItem, Amount: 1000})
	breaker := oracle.NewCircuitBreaker(5, 0)
	client := oracle.NewResilientClient(src, oracle.ClientConfig{Timeout: 50_000_000, MaxRetries: 0}, breaker)
	svc := NewOracleService(client, db, OracleConfig{MaxPrice: 1_000_000, MaxDeviationRatio: 0}, slog.Default())
	ctx := context.Background()

	// 1) Good price is accepted and stored.
	n, err := svc.Refresh(ctx)
	if err != nil || n != 1 {
		t.Fatalf("first refresh n=%d err=%v, want 1 accepted", n, err)
	}
	if p, err := svc.CurrentPrice(orItem); err != nil || p.Amount != 1000 {
		t.Fatalf("current price = %v (err %v), want 1000", p, err)
	}

	// 2) Zero/negative are rejected: nothing accepted, last-good retained.
	src.SetPrices(oracle.Price{ItemID: orItem, Amount: 0}, oracle.Price{ItemID: orItem, Amount: -50})
	n, err = svc.Refresh(ctx)
	if err != nil || n != 0 {
		t.Fatalf("bad refresh n=%d err=%v, want 0 accepted", n, err)
	}
	if p, _ := svc.CurrentPrice(orItem); p.Amount != 1000 {
		t.Fatalf("last-good clobbered by bad values: %d", p.Amount)
	}

	// 3) Upstream down: Refresh errors but last-good survives.
	src.SetPrices()
	src.SetError(errors.New("upstream unavailable"))
	if _, err := svc.Refresh(ctx); err == nil {
		t.Fatal("expected error when upstream is down")
	}
	if p, _ := svc.CurrentPrice(orItem); p.Amount != 1000 {
		t.Fatalf("last-good lost after upstream error: %d", p.Amount)
	}

	// 4) A new good price updates the cache and is stored.
	src.SetError(nil)
	src.SetPrices(oracle.Price{ItemID: orItem, Amount: 1500})
	if n, err := svc.Refresh(ctx); err != nil || n != 1 {
		t.Fatalf("recovery refresh n=%d err=%v, want 1", n, err)
	}
	if p, _ := svc.CurrentPrice(orItem); p.Amount != 1500 {
		t.Fatalf("current price = %d, want 1500", p.Amount)
	}

	// Only the two accepted prices (1000, 1500) were persisted.
	if got := countPrices(t, db); got != 2 {
		t.Fatalf("stored %d prices, want 2 (only validated ones)", got)
	}
}

// TestOracleLoadLastKnownGood warms the cache from the DB on startup.
func TestOracleLoadLastKnownGood(t *testing.T) {
	db := setupOracle(t)
	src := oracle.NewMockSource(oracle.Price{ItemID: orItem, Amount: 777})
	breaker := oracle.NewCircuitBreaker(5, 0)
	client := oracle.NewResilientClient(src, oracle.ClientConfig{Timeout: 50_000_000, MaxRetries: 0}, breaker)
	ctx := context.Background()

	// Populate via one service, then warm a brand-new service from the DB.
	seeder := NewOracleService(client, db, OracleConfig{}, slog.Default())
	if _, err := seeder.Refresh(ctx); err != nil {
		t.Fatalf("seed refresh: %v", err)
	}

	fresh := NewOracleService(client, db, OracleConfig{}, slog.Default())
	if _, err := fresh.CurrentPrice(orItem); err == nil {
		t.Fatal("cache should be empty before warming")
	}
	if err := fresh.LoadLastKnownGood(ctx); err != nil {
		t.Fatalf("load last-known-good: %v", err)
	}
	if p, err := fresh.CurrentPrice(orItem); err != nil || p.Amount != 777 {
		t.Fatalf("warmed price = %v (err %v), want 777", p, err)
	}
}
