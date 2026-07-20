package service

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/herotech/market-dragon/internal/repository"
)

// WalletService performs auditable money movements. Every mutation locks the
// wallet row (SELECT ... FOR UPDATE) and appends a ledger entry, so concurrent
// operations cannot over-commit funds.
//
// Each operation has two forms:
//   - the exported method (e.g. Reserve) runs in its own transaction;
//   - the *Tx variant (e.g. ReserveTx) joins a caller-provided transaction, so a
//     larger use-case (like a purchase) can move money atomically with other work.
type WalletService struct {
	db *gorm.DB
}

// NewWalletService builds a WalletService over the given DB handle.
func NewWalletService(db *gorm.DB) *WalletService {
	return &WalletService{db: db}
}

// Reserve moves `amount` from available into reserved (e.g. placing a bid).
func (s *WalletService) Reserve(ctx context.Context, guildID uint64, amount int64, refType string, refID uint64) error {
	return s.inTx(ctx, func(tx *gorm.DB) error { return s.ReserveTx(tx, guildID, amount, refType, refID) })
}

// ReserveTx is Reserve within an existing transaction.
func (s *WalletService) ReserveTx(tx *gorm.DB, guildID uint64, amount int64, refType string, refID uint64) error {
	return withWalletTx(tx, guildID, func(w *repository.Wallet) error {
		if err := EnsureCanReserve(w.TotalBalance, w.ReservedAmount, amount); err != nil {
			return err
		}
		w.ReservedAmount += amount
		return saveAndRecord(tx, w, repository.TxReserve, amount, refType, refID)
	})
}

// Release returns `amount` from reserved back to available (e.g. an outbid loser).
func (s *WalletService) Release(ctx context.Context, guildID uint64, amount int64, refType string, refID uint64) error {
	return s.inTx(ctx, func(tx *gorm.DB) error { return s.ReleaseTx(tx, guildID, amount, refType, refID) })
}

// ReleaseTx is Release within an existing transaction.
func (s *WalletService) ReleaseTx(tx *gorm.DB, guildID uint64, amount int64, refType string, refID uint64) error {
	return withWalletTx(tx, guildID, func(w *repository.Wallet) error {
		if err := EnsureCanRelease(w.ReservedAmount, amount); err != nil {
			return err
		}
		w.ReservedAmount -= amount
		return saveAndRecord(tx, w, repository.TxRelease, amount, refType, refID)
	})
}

// Debit spends `amount` from available balance (e.g. a limit-order buyer).
func (s *WalletService) Debit(ctx context.Context, guildID uint64, amount int64, refType string, refID uint64) error {
	return s.inTx(ctx, func(tx *gorm.DB) error { return s.DebitTx(tx, guildID, amount, refType, refID) })
}

// DebitTx is Debit within an existing transaction.
func (s *WalletService) DebitTx(tx *gorm.DB, guildID uint64, amount int64, refType string, refID uint64) error {
	return withWalletTx(tx, guildID, func(w *repository.Wallet) error {
		if err := EnsureSufficientAvailable(w.TotalBalance, w.ReservedAmount, amount); err != nil {
			return err
		}
		w.TotalBalance -= amount
		return saveAndRecord(tx, w, repository.TxDebit, amount, refType, refID)
	})
}

// Credit adds `amount` to the total balance (e.g. a seller receiving payment).
func (s *WalletService) Credit(ctx context.Context, guildID uint64, amount int64, refType string, refID uint64) error {
	return s.inTx(ctx, func(tx *gorm.DB) error { return s.CreditTx(tx, guildID, amount, refType, refID) })
}

// CreditTx is Credit within an existing transaction.
func (s *WalletService) CreditTx(tx *gorm.DB, guildID uint64, amount int64, refType string, refID uint64) error {
	return withWalletTx(tx, guildID, func(w *repository.Wallet) error {
		if err := EnsurePositive(amount); err != nil {
			return err
		}
		w.TotalBalance += amount
		return saveAndRecord(tx, w, repository.TxCredit, amount, refType, refID)
	})
}

// SettleReserved converts a prior reservation into a spend (e.g. an auction
// winner): it reduces both total and reserved by `amount`.
func (s *WalletService) SettleReserved(ctx context.Context, guildID uint64, amount int64, refType string, refID uint64) error {
	return s.inTx(ctx, func(tx *gorm.DB) error { return s.SettleReservedTx(tx, guildID, amount, refType, refID) })
}

// SettleReservedTx is SettleReserved within an existing transaction.
func (s *WalletService) SettleReservedTx(tx *gorm.DB, guildID uint64, amount int64, refType string, refID uint64) error {
	return withWalletTx(tx, guildID, func(w *repository.Wallet) error {
		if err := EnsureCanRelease(w.ReservedAmount, amount); err != nil {
			return err
		}
		w.TotalBalance -= amount
		w.ReservedAmount -= amount
		return saveAndRecord(tx, w, repository.TxDebit, amount, refType, refID)
	})
}

// WalletBalance is a read-only view of a guild's wallet.
type WalletBalance struct {
	GuildID   uint64
	Total     int64
	Reserved  int64
	Available int64
}

// Balance returns a guild's current wallet balance. Available is derived as
// Total - Reserved (never stored).
func (s *WalletService) Balance(ctx context.Context, guildID uint64) (*WalletBalance, error) {
	var w repository.Wallet
	if err := s.db.WithContext(ctx).Where("guild_id = ?", guildID).First(&w).Error; err != nil {
		return nil, notFoundOr(err, "load wallet")
	}
	return &WalletBalance{
		GuildID:   w.GuildID,
		Total:     w.TotalBalance,
		Reserved:  w.ReservedAmount,
		Available: w.TotalBalance - w.ReservedAmount,
	}, nil
}

func (s *WalletService) inTx(ctx context.Context, fn func(*gorm.DB) error) error {
	return s.db.WithContext(ctx).Transaction(fn)
}

// withWalletTx locks the guild's wallet row within tx and runs fn against it.
func withWalletTx(tx *gorm.DB, guildID uint64, fn func(*repository.Wallet) error) error {
	var w repository.Wallet
	if err := tx.
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("guild_id = ?", guildID).
		First(&w).Error; err != nil {
		return fmt.Errorf("lock wallet for guild %d: %w", guildID, err)
	}
	return fn(&w)
}

// saveAndRecord persists the wallet and appends a ledger entry.
func saveAndRecord(tx *gorm.DB, w *repository.Wallet, txType string, amount int64, refType string, refID uint64) error {
	if err := tx.Save(w).Error; err != nil {
		return fmt.Errorf("update wallet: %w", err)
	}
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
