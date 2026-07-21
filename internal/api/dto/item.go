package dto

import "github.com/herotech/market-dragon/internal/model"

// CreateItemRequest registers a new item into the market. The item is not for
// sale until it is listed (Common/Rare) or auctioned (Legendary).
type CreateItemRequest struct {
	OwnerGuildID uint64 `json:"owner_guild_id" binding:"required" example:"2"`
	Name         string `json:"name" binding:"required" example:"Elven Bow"`
	Tier         string `json:"tier" binding:"required" example:"rare"`
	Stock        int    `json:"stock" example:"5"`
}

// ListForSaleRequest offers a Common/Rare item at a fixed price.
type ListForSaleRequest struct {
	SellerGuildID uint64 `json:"seller_guild_id" binding:"required" example:"2"`
	Price         int64  `json:"price" binding:"required" example:"500"`
}

// OpenAuctionRequest opens an auction for a Legendary item.
type OpenAuctionRequest struct {
	SellerGuildID uint64 `json:"seller_guild_id" binding:"required" example:"3"`
}

// BuyRequest buys a fixed-price item.
type BuyRequest struct {
	BuyerGuildID uint64 `json:"buyer_guild_id" binding:"required" example:"1"`
}

// PlaceBidRequest places a bid on an item's active auction.
type PlaceBidRequest struct {
	BidderGuildID uint64 `json:"bidder_guild_id" binding:"required" example:"1"`
	Amount        int64  `json:"amount" binding:"required" example:"1200"`
}

// CancelBidRequest cancels a bid.
type CancelBidRequest struct {
	BidderGuildID uint64 `json:"bidder_guild_id" binding:"required" example:"1"`
}

// ItemResponse is an item, with its current validated Oracle price when known.
type ItemResponse struct {
	ID           uint64 `json:"id" example:"3"`
	Name         string `json:"name" example:"Soul Reaver"`
	Tier         string `json:"tier" example:"legendary"`
	OwnerGuildID uint64 `json:"owner_guild_id" example:"3"`
	Status       string `json:"status" example:"available"`
	Stock        int    `json:"stock" example:"1"`
	Price        *int64 `json:"price,omitempty" example:"250000"`
}

// ListingResponse is a fixed-price listing.
type ListingResponse struct {
	ID            uint64  `json:"id" example:"1"`
	ItemID        uint64  `json:"item_id" example:"2"`
	SellerGuildID uint64  `json:"seller_guild_id" example:"2"`
	Price         int64   `json:"price" example:"500"`
	Status        string  `json:"status" example:"open"`
	BuyerGuildID  *uint64 `json:"buyer_guild_id,omitempty"`
}

// ItemListResponse is a list of items.
type ItemListResponse struct {
	Items []ItemResponse `json:"items"`
}

// ItemDetailResponse is an item with its active offer (listing or auction).
type ItemDetailResponse struct {
	Item    ItemResponse     `json:"item"`
	Listing *ListingResponse `json:"listing,omitempty"`
	Auction *AuctionResponse `json:"auction,omitempty"`
}

// NewItemResponse builds an ItemResponse from a model. The live price is set by
// the caller (it comes from the Oracle service, not the item row).
func NewItemResponse(item *model.Item) ItemResponse {
	return ItemResponse{
		ID:           item.ID,
		Name:         item.Name,
		Tier:         item.Tier,
		OwnerGuildID: item.OwnerGuildID,
		Status:       item.Status,
		Stock:        item.Stock,
	}
}

// NewListingResponse builds a ListingResponse from a model.
func NewListingResponse(l *model.Listing) ListingResponse {
	return ListingResponse{
		ID:            l.ID,
		ItemID:        l.ItemID,
		SellerGuildID: l.SellerGuildID,
		Price:         l.Price,
		Status:        l.Status,
		BuyerGuildID:  l.BuyerGuildID,
	}
}
