package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/herotech/market-dragon/internal/repository"
)

// ItemService owns the lifecycle of tradeable items: registering a new item
// into the market and reading items. Offering an item for sale (a fixed-price
// listing or an auction) is a separate step handled by ListingService and
// AuctionService.
type ItemService struct {
	db  *gorm.DB
	now func() time.Time
}

// NewItemService builds an ItemService.
func NewItemService(db *gorm.DB) *ItemService {
	return &ItemService{db: db, now: time.Now}
}

// CreateItem registers a new item owned by a guild. The item starts as
// Available and is not yet for sale. Legendary items are unique, so their stock
// is forced to 1.
func (s *ItemService) CreateItem(ctx context.Context, ownerGuildID uint64, name, tier string, stock int) (*repository.Item, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrNameRequired
	}
	tier = strings.ToLower(strings.TrimSpace(tier))
	switch tier {
	case repository.TierCommon, repository.TierRare:
		if stock < 1 {
			return nil, ErrInvalidStock
		}
	case repository.TierLegendary:
		stock = 1
	default:
		return nil, ErrInvalidTier
	}

	var item repository.Item
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var owner repository.Guild
		if err := tx.First(&owner, ownerGuildID).Error; err != nil {
			return notFoundOr(err, "load owner guild")
		}
		now := s.now()
		item = repository.Item{
			Name:         name,
			Tier:         tier,
			OwnerGuildID: ownerGuildID,
			Status:       repository.ItemAvailable,
			Stock:        stock,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		if err := tx.Create(&item).Error; err != nil {
			return fmt.Errorf("create item: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// ListItems returns every item, newest first.
func (s *ItemService) ListItems(ctx context.Context) ([]repository.Item, error) {
	var items []repository.Item
	if err := s.db.WithContext(ctx).Order("id ASC").Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list items: %w", err)
	}
	return items, nil
}

// GetItem returns a single item by ID.
func (s *ItemService) GetItem(ctx context.Context, id uint64) (*repository.Item, error) {
	var item repository.Item
	if err := s.db.WithContext(ctx).First(&item, id).Error; err != nil {
		return nil, notFoundOr(err, "load item")
	}
	return &item, nil
}
