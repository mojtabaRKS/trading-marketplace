//go:build e2e

// Package e2e drives the full HTTP API (router + handlers + services + real
// Postgres) the way a client would, asserting the business rules through the
// wire. Requires a running database (see .env / docker compose).
//
// Run with: go test -tags e2e -race -v ./test/e2e/...
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/herotech/market-dragon/internal/api"
	"github.com/herotech/market-dragon/internal/api/handler"
	"github.com/herotech/market-dragon/internal/config"
	"github.com/herotech/market-dragon/internal/infra/database"
	"github.com/herotech/market-dragon/internal/infra/oracle"
	"github.com/herotech/market-dragon/internal/model"
	"github.com/herotech/market-dragon/internal/service"
)

// app bundles the running HTTP handler and the pieces a test may need to poke.
type app struct {
	t      *testing.T
	engine http.Handler
	db     *gorm.DB
	oracle *service.OracleService
	src    *oracle.MockSource
}

// setupApp wires real services over the test database and returns an in-process
// HTTP app. It uses httptest-style direct handler invocation (no socket) for
// speed and determinism.
func setupApp(t *testing.T) *app {
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

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	wallets := service.NewWalletService(db)
	items := service.NewItemService(db)
	listings := service.NewListingService(db, wallets)
	auctions := service.NewAuctionService(db, wallets, 24*time.Hour, 5*time.Minute)
	src := oracle.NewMockSource()
	oracles := service.NewOracleService(src, db, service.OracleConfig{}, logger)

	engine := api.NewRouter(handler.Deps{
		Logger:   logger,
		DB:       db,
		Items:    items,
		Listings: listings,
		Auctions: auctions,
		Wallets:  wallets,
		Oracle:   oracles,
	}, false)

	return &app{t: t, engine: engine, db: db, oracle: oracles, src: src}
}

// req performs an HTTP request against the router and returns the status and the
// decoded JSON body (as a generic map).
func (a *app) req(method, path string, body any, headers map[string]string) (int, map[string]any) {
	a.t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			a.t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(raw)
	}
	r, err := http.NewRequest(method, path, reader)
	if err != nil {
		a.t.Fatalf("new request: %v", err)
	}
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	a.engine.ServeHTTP(w, r)

	out := map[string]any{}
	if w.Body.Len() > 0 {
		_ = json.Unmarshal(w.Body.Bytes(), &out)
	}
	return w.Code, out
}

// createGuild resets and inserts a guild with a funded wallet. Using dedicated
// high IDs keeps e2e data isolated from seed data and repeatable across runs.
func (a *app) createGuild(id uint64, total, dailyCap int64) {
	a.t.Helper()
	a.db.Exec("DELETE FROM wallet_transactions WHERE guild_id = ?", id)
	a.db.Exec("DELETE FROM daily_purchase_totals WHERE guild_id = ?", id)
	a.db.Exec("DELETE FROM wallets WHERE guild_id = ?", id)
	a.db.Exec("DELETE FROM guilds WHERE id = ?", id)
	if err := a.db.Create(&model.Guild{ID: id, Name: fmt.Sprintf("E2E-%d", id), DailyPurchaseCap: dailyCap}).Error; err != nil {
		a.t.Fatalf("create guild %d: %v", id, err)
	}
	if err := a.db.Create(&model.Wallet{GuildID: id, TotalBalance: total}).Error; err != nil {
		a.t.Fatalf("create wallet %d: %v", id, err)
	}
}

// setPrice makes the Oracle report `amount` for an item and refreshes the cache.
func (a *app) setPrice(itemID uint64, amount int64) {
	a.t.Helper()
	a.src.SetPrices(oracle.Price{ItemID: itemID, Amount: amount})
	if _, err := a.oracle.Refresh(context.Background()); err != nil {
		a.t.Fatalf("oracle refresh: %v", err)
	}
}

// uintField pulls a numeric JSON field (decoded as float64) as uint64.
func uintField(t *testing.T, m map[string]any, key string) uint64 {
	t.Helper()
	v, ok := m[key].(float64)
	if !ok {
		t.Fatalf("field %q missing or not a number in %v", key, m)
	}
	return uint64(v)
}

// intField pulls a numeric JSON field as int64.
func intField(t *testing.T, m map[string]any, key string) int64 {
	t.Helper()
	v, ok := m[key].(float64)
	if !ok {
		t.Fatalf("field %q missing or not a number in %v", key, m)
	}
	return int64(v)
}
