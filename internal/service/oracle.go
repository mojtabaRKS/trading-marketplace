package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/herotech/market-dragon/internal/infra/oracle"
	"github.com/herotech/market-dragon/internal/repository"
)

// OracleConfig controls price validation.
type OracleConfig struct {
	// MaxPrice is a plausibility ceiling; a price above it is rejected. 0 disables.
	MaxPrice int64
	// MaxDeviationRatio rejects a price that jumps more than this factor away from
	// the last-known-good value (e.g. 10 => reject if >10x or <1/10x). 0 disables.
	MaxDeviationRatio float64
}

// OracleService consumes the (unreliable) price feed, validates each value, and
// keeps a last-known-good price per item both in memory and in oracle_prices.
// A bad or missing upstream never overwrites a good price: rejects are dropped
// and the previous value is retained.
type OracleService struct {
	src    oracle.Source
	db     *gorm.DB
	cfg    OracleConfig
	logger *slog.Logger
	now    func() time.Time

	mu    sync.RWMutex
	cache map[uint64]oracle.Price
}

// NewOracleService builds an OracleService over the given source and DB.
func NewOracleService(src oracle.Source, db *gorm.DB, cfg OracleConfig, logger *slog.Logger) *OracleService {
	return &OracleService{
		src:    src,
		db:     db,
		cfg:    cfg,
		logger: logger,
		now:    time.Now,
		cache:  make(map[uint64]oracle.Price),
	}
}

// LoadLastKnownGood warms the in-memory cache with the latest stored price per
// item, so the service serves good data immediately after a restart.
func (s *OracleService) LoadLastKnownGood(ctx context.Context) error {
	type row struct {
		ItemID uint64
		Price  int64
	}
	var rows []row
	if err := s.db.WithContext(ctx).Raw(
		`SELECT DISTINCT ON (item_id) item_id, price
		   FROM oracle_prices
		  ORDER BY item_id, created_at DESC, id DESC`,
	).Scan(&rows).Error; err != nil {
		return fmt.Errorf("load last-known-good prices: %w", err)
	}
	s.mu.Lock()
	for _, r := range rows {
		s.cache[r.ItemID] = oracle.Price{ItemID: r.ItemID, Amount: r.Price}
	}
	s.mu.Unlock()
	return nil
}

// Refresh pulls a snapshot from the source, validates each price, and persists
// the accepted ones. It returns the number accepted. A source error is returned
// to the caller (which keeps last-known-good); it never clears the cache.
func (s *OracleService) Refresh(ctx context.Context) (int, error) {
	prices, err := s.src.Fetch(ctx)
	if err != nil {
		return 0, err
	}

	accepted := 0
	for _, p := range prices {
		prev, _ := s.currentAmount(p.ItemID)
		if verr := validatePrice(p, prev, s.cfg); verr != nil {
			s.logger.Warn("oracle price rejected",
				slog.Uint64("item_id", p.ItemID),
				slog.Int64("amount", p.Amount),
				slog.Any("reason", verr),
			)
			continue
		}

		now := s.now()
		record := repository.OraclePrice{
			ItemID:     p.ItemID,
			Price:      p.Amount,
			Source:     "oracle",
			ObservedAt: now,
			CreatedAt:  now,
		}
		if err := s.db.WithContext(ctx).Create(&record).Error; err != nil {
			return accepted, fmt.Errorf("store oracle price for item %d: %w", p.ItemID, err)
		}
		s.setCache(p)
		accepted++
	}
	return accepted, nil
}

// CurrentPrice returns the last validated price for an item.
func (s *OracleService) CurrentPrice(itemID uint64) (oracle.Price, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.cache[itemID]
	if !ok {
		return oracle.Price{}, ErrPriceUnavailable
	}
	return p, nil
}

func (s *OracleService) currentAmount(itemID uint64) (int64, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.cache[itemID]
	return p.Amount, ok
}

func (s *OracleService) setCache(p oracle.Price) {
	s.mu.Lock()
	s.cache[p.ItemID] = p
	s.mu.Unlock()
}

// validatePrice rejects non-positive, implausibly large, or wildly deviating
// prices. `prev` is the last-known-good amount (0 if none).
func validatePrice(p oracle.Price, prev int64, cfg OracleConfig) error {
	if p.Amount <= 0 {
		return ErrOraclePriceInvalid
	}
	if cfg.MaxPrice > 0 && p.Amount > cfg.MaxPrice {
		return ErrOraclePriceImplausible
	}
	if cfg.MaxDeviationRatio > 0 && prev > 0 {
		hi := int64(float64(prev) * cfg.MaxDeviationRatio)
		lo := int64(float64(prev) / cfg.MaxDeviationRatio)
		if p.Amount > hi || p.Amount < lo {
			return ErrOraclePriceImplausible
		}
	}
	return nil
}
