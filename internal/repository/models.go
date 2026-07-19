package repository

import "time"

// Enum-like string values. Schema-level enforcement lives in the SQL migrations
// (CHECK constraints); these constants keep application code consistent.
const (
	TierCommon    = "common"
	TierRare      = "rare"
	TierLegendary = "legendary"

	ItemAvailable = "available"
	ItemListed    = "listed"
	ItemInAuction = "in_auction"
	ItemSold      = "sold"

	ListingOpen      = "open"
	ListingSold      = "sold"
	ListingCancelled = "cancelled"

	AuctionActive    = "active"
	AuctionSettled   = "settled"
	AuctionCancelled = "cancelled"

	BidActive    = "active"
	BidReleased  = "released"
	BidWon       = "won"
	BidCancelled = "cancelled"

	TxReserve = "reserve"
	TxRelease = "release"
	TxDebit   = "debit"
	TxCredit  = "credit"

	RefListing = "listing"
	RefAuction = "auction"
	RefBid     = "bid"
)

// Guild is a market participant with a wallet and a daily purchase cap.
type Guild struct {
	ID               uint64 `gorm:"primaryKey"`
	Name             string
	DailyPurchaseCap int64 // minor units; 0 = unlimited
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Wallet holds gold. Available = TotalBalance - ReservedAmount (computed, never stored).
type Wallet struct {
	ID             uint64 `gorm:"primaryKey"`
	GuildID        uint64
	TotalBalance   int64
	ReservedAmount int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// WalletTransaction is an append-only ledger entry for every money movement.
type WalletTransaction struct {
	ID        uint64 `gorm:"primaryKey"`
	WalletID  uint64
	GuildID   uint64
	Type      string // reserve|release|debit|credit
	Amount    int64
	RefType   string // listing|auction|bid
	RefID     uint64
	CreatedAt time.Time
}

// Item is a tradeable asset. Legendary items are unique (Stock = 1, never reproduced).
type Item struct {
	ID           uint64 `gorm:"primaryKey"`
	Name         string
	Tier         string // common|rare|legendary
	OwnerGuildID uint64
	Status       string
	Stock        int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Listing is a fixed-price limit order (Common & Rare).
type Listing struct {
	ID            uint64 `gorm:"primaryKey"`
	ItemID        uint64
	SellerGuildID uint64
	Price         int64
	Status        string
	BuyerGuildID  *uint64
	CreatedAt     time.Time
	SoldAt        *time.Time
}

// Auction is a Legendary-only sale. At most one active auction per item
// (enforced by the partial unique index uniq_active_auction_per_item).
type Auction struct {
	ID            uint64 `gorm:"primaryKey"`
	ItemID        uint64
	SellerGuildID uint64
	Status        string
	StartsAt      time.Time
	EndsAt        time.Time
	HighestBidID  *uint64
	WinnerGuildID *uint64
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Bid is an offer on an auction. Amount is reserved (not debited) while active.
type Bid struct {
	ID            uint64 `gorm:"primaryKey"`
	AuctionID     uint64
	BidderGuildID uint64
	Amount        int64
	Status        string
	CreatedAt     time.Time
}

// OraclePrice is an accepted (validated) base price from the external Oracle.
type OraclePrice struct {
	ID         uint64 `gorm:"primaryKey"`
	ItemID     uint64
	Price      int64
	Source     string
	ObservedAt time.Time
	CreatedAt  time.Time
}

// IdempotencyKey stores the replayed result of a state-changing request.
type IdempotencyKey struct {
	Key            string `gorm:"primaryKey"`
	RequestHash    string
	ResponseStatus int
	ResponseBody   []byte
	CreatedAt      time.Time
}

// DailyPurchaseTotal tracks per-guild spend per calendar day for cap enforcement.
type DailyPurchaseTotal struct {
	ID         uint64 `gorm:"primaryKey"`
	GuildID    uint64
	Day        time.Time `gorm:"type:date"`
	TotalSpent int64
}
