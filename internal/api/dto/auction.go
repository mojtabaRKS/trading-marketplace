package dto

import "github.com/herotech/market-dragon/internal/repository"

// AuctionResponse is an auction record.
type AuctionResponse struct {
	ID            uint64  `json:"id" example:"1"`
	ItemID        uint64  `json:"item_id" example:"3"`
	SellerGuildID uint64  `json:"seller_guild_id" example:"3"`
	Status        string  `json:"status" example:"active"`
	StartsAt      string  `json:"starts_at" example:"2026-01-01T00:00:00Z"`
	EndsAt        string  `json:"ends_at" example:"2026-01-02T00:00:00Z"`
	HighestBidID  *uint64 `json:"highest_bid_id,omitempty"`
	WinnerGuildID *uint64 `json:"winner_guild_id,omitempty"`
}

// BidResponse is a bid record.
type BidResponse struct {
	ID            uint64 `json:"id" example:"1"`
	AuctionID     uint64 `json:"auction_id" example:"1"`
	BidderGuildID uint64 `json:"bidder_guild_id" example:"1"`
	Amount        int64  `json:"amount" example:"1200"`
	Status        string `json:"status" example:"active"`
}

// AuctionDetailResponse is an auction plus its current highest bid.
type AuctionDetailResponse struct {
	Auction    AuctionResponse `json:"auction"`
	HighestBid *BidResponse    `json:"highest_bid,omitempty"`
}

// AuctionListResponse is a list of auctions.
type AuctionListResponse struct {
	Auctions []AuctionResponse `json:"auctions"`
}

// NewAuctionResponse builds an AuctionResponse from a model.
func NewAuctionResponse(a *repository.Auction) AuctionResponse {
	return AuctionResponse{
		ID:            a.ID,
		ItemID:        a.ItemID,
		SellerGuildID: a.SellerGuildID,
		Status:        a.Status,
		StartsAt:      a.StartsAt.UTC().Format("2006-01-02T15:04:05Z"),
		EndsAt:        a.EndsAt.UTC().Format("2006-01-02T15:04:05Z"),
		HighestBidID:  a.HighestBidID,
		WinnerGuildID: a.WinnerGuildID,
	}
}

// NewBidResponse builds a BidResponse from a model.
func NewBidResponse(b *repository.Bid) BidResponse {
	return BidResponse{
		ID:            b.ID,
		AuctionID:     b.AuctionID,
		BidderGuildID: b.BidderGuildID,
		Amount:        b.Amount,
		Status:        b.Status,
	}
}
