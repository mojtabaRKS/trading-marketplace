package service

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/herotech/market-dragon/internal/repository"
)

// WalletService performs auditable money movements. Every mutation runs in a DB
// transaction that locks the wallet row (SELECT ... FOR UPDATE) and appends a
// ledger entry, so concurrent operations cannot over-commit funds.
type WalletService struct {
	db *gorm.DB
}

// NewWalletService builds a WalletService over the given DB handle.
func NewWalletService(db *gorm.DB) *WalletService {
	return &WalletService{db: db}
}

// Reserve moves `amount` from available into reserved (e.g. placing a bid).
func (s *WalletService) Reserve(ctx context.Context, guildID uint64, amount int64, refType string, refID uint64) error {
	return s.withWallet(ctx, guildID, func(tx *gorm.DB, w *repository.Wallet) error {
		if err := EnsureCanReserve(w.TotalBalance, w.ReservedAmount, amount); err != nil {
			return err
		}
		w.ReservedAmount += amount
		if err := tx.Save(w).Error; err != nil {
			return fmt.Errorf("update wallet: %w", err)
		}
		return record(tx, w, repository.TxReserve, amount, refType, refID)
	})
}

// Release returns `amount` from reserved back to available (e.g. an outbid loser).
func (s *WalletService) Release(ctx context.Context, guildID uint64, amount int64, refType string, refID uint64) error {
	return s.withWallet(ctx, guildID, func(tx *gorm.DB, w *repository.Wallet) error {
		if err := EnsureCanRelease(w.ReservedAmount, amount); err != nil {
			return err
		}
		w.ReservedAmount -= amount
		if err := tx.Save(w).Error; err != nil {
			return fmt.Errorf("update wallet: %w", err)
		}
		return record(tx, w, repository.TxRelease, amount, refType, refID)
	})
}

// Debit spends `amount` from available balance (e.g. a limit-order buyer).
func (s *WalletService) Debit(ctx context.Context, guildID uint64, amount int64, refType string, refID uint64) error {
	return s.withWallet(ctx, guildID, func(tx *gorm.DB, w *repository.Wallet) error {
		if err := EnsureSufficientAvailable(w.TotalBalance, w.ReservedAmount, amount); err != nil {
			return err
		}
		w.TotalBalance -= amount
		if err := tx.Save(w).Error; err != nil {
			return fmt.Errorf("update wallet: %w", err)
		}
		return record(tx, w, repository.TxDebit, amount, refType, refID)
	})
}

// Credit adds `amount` to the total balance (e.g. a seller receiving payment).
func (s *WalletService) Credit(ctx context.Context, guildID uint64, amount int64, refType string, refID uint64) error {
	return s.withWallet(ctx, guildID, func(tx *gorm.DB, w *repository.Wallet) error {
		if err := EnsurePositive(amount); err != nil {
			return err
		}
		w.TotalBalance += amount
		if err := tx.Save(w).Error; err != nil {
			return fmt.Errorf("update wallet: %w", err)
		}
		return record(tx, w, repository.TxCredit, amount, refType, refID)
	})
}

// SettleReserved converts a prior reservation into a spend (e.g. an auction
// winner): it reduces both total and reserved by `amount`.
func (s *WalletService) SettleReserved(ctx context.Context, guildID uint64, amount int64, refType string, refID uint64) error {
	return s.withWallet(ctx, guildID, func(tx *gorm.DB, w *repository.Wallet) error {
		if err := EnsureCanRelease(w.ReservedAmount, amount); err != nil {
			return err
		}
		w.TotalBalance -= amount
		w.ReservedAmount -= amount
		if err := tx.Save(w).Error; err != nil {
			return fmt.Errorf("update wallet: %w", err)
		}
		return record(tx, w, repository.TxDebit, amount, refType, refID)
	})
}

// withWallet runs fn inside a transaction with the guild's wallet row locked.
func (s *WalletService) withWallet(ctx context.Context, guildID uint64, fn func(*gorm.DB, *repository.Wallet) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var w repository.Wallet
		if err := tx.
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("guild_id = ?", guildID).
			First(&w).Error; err != nil {
			return fmt.Errorf("lock wallet for guild %d: %w", guildID, err)
		}
		return fn(tx, &w)
	})
}

// record appends a ledger entry for a wallet movement.
func record(tx *gorm.DB, w *repository.Wallet, txType string, amount int64, refType string, refID uint64) error {
	entry := repository.WalletTransaction{
		WalletID:  w.ID,
		GuildID:   w.GuildID,
		Type:      txType,
		Amount:    amount,
		RefType:   refType,
		RefID:     refID,
		CreatedAt: time.Now(),
	}
	if err := tx.Create(&entry).Error; err != nil {
		return fmt.Errorf("record ledger entry: %w", err)
	}
	return nil
}
