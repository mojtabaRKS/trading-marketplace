package repository

import (
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Seed inserts a small, deterministic dataset for local development.
// Idempotent: safe to run repeatedly (conflicts on existing primary keys are ignored).
func Seed(db *gorm.DB) error {
	guilds := []Guild{
		{ID: 1, Name: "Emberforge", DailyPurchaseCap: 1_000_000},
		{ID: 2, Name: "Stormhaven", DailyPurchaseCap: 1_000_000},
		{ID: 3, Name: "Nightspire", DailyPurchaseCap: 0}, // unlimited
	}
	wallets := []Wallet{
		{ID: 1, GuildID: 1, TotalBalance: 500_000},
		{ID: 2, GuildID: 2, TotalBalance: 750_000},
		{ID: 3, GuildID: 3, TotalBalance: 2_000_000},
	}
	items := []Item{
		{ID: 1, Name: "Iron Dagger", Tier: TierCommon, OwnerGuildID: 1, Status: ItemAvailable, Stock: 999},
		{ID: 2, Name: "Elven Bow", Tier: TierRare, OwnerGuildID: 2, Status: ItemAvailable, Stock: 5},
		{ID: 3, Name: "Soul Reaver", Tier: TierLegendary, OwnerGuildID: 3, Status: ItemAvailable, Stock: 1},
		{ID: 4, Name: "Eye of the Dragon", Tier: TierLegendary, OwnerGuildID: 1, Status: ItemAvailable, Stock: 1},
	}

	// Fresh session per model: a *gorm.DB carrying a finished statement must not
	// be reused across different model types.
	skip := func() *gorm.DB { return db.Clauses(clause.OnConflict{DoNothing: true}) }

	if err := skip().Create(&guilds).Error; err != nil {
		return fmt.Errorf("seed guilds: %w", err)
	}
	if err := skip().Create(&wallets).Error; err != nil {
		return fmt.Errorf("seed wallets: %w", err)
	}
	if err := skip().Create(&items).Error; err != nil {
		return fmt.Errorf("seed items: %w", err)
	}
	return nil
}
