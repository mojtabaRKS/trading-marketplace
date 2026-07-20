package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/herotech/market-dragon/internal/repository"
)

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

func toListingResponse(l *repository.Listing) ListingResponse {
	return ListingResponse{
		ID:            l.ID,
		ItemID:        l.ItemID,
		SellerGuildID: l.SellerGuildID,
		Price:         l.Price,
		Status:        l.Status,
		BuyerGuildID:  l.BuyerGuildID,
	}
}

func (d Deps) itemResponse(itemID uint64, item *repository.Item) ItemResponse {
	resp := ItemResponse{
		ID:           item.ID,
		Name:         item.Name,
		Tier:         item.Tier,
		OwnerGuildID: item.OwnerGuildID,
		Status:       item.Status,
		Stock:        item.Stock,
	}
	if p, err := d.Oracle.CurrentPrice(itemID); err == nil {
		amount := p.Amount
		resp.Price = &amount
	}
	return resp
}

// createItem godoc
//
//	@Summary		Register an item
//	@Description	Register a new item owned by a guild. The item is not for sale yet. Legendary items are unique (stock is forced to 1).
//	@Tags			items
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string				false	"Idempotency key to make retries safe"
//	@Param			request			body		CreateItemRequest	true	"Item to register"
//	@Success		201				{object}	ItemResponse
//	@Failure		400				{object}	ErrorResponse
//	@Failure		404				{object}	ErrorResponse
//	@Router			/items [post]
func createItem(deps Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req CreateItemRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}
		item, err := deps.Items.CreateItem(c.Request.Context(), req.OwnerGuildID, req.Name, req.Tier, req.Stock)
		if err != nil {
			respondError(c, deps.Logger, err)
			return
		}
		c.JSON(http.StatusCreated, deps.itemResponse(item.ID, item))
	}
}

// listItems godoc
//
//	@Summary		List items
//	@Description	List every item with its current validated Oracle price (when available).
//	@Tags			items
//	@Produce		json
//	@Success		200	{object}	ItemListResponse
//	@Router			/items [get]
func listItems(deps Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		items, err := deps.Items.ListItems(c.Request.Context())
		if err != nil {
			respondError(c, deps.Logger, err)
			return
		}
		out := make([]ItemResponse, 0, len(items))
		for i := range items {
			out = append(out, deps.itemResponse(items[i].ID, &items[i]))
		}
		c.JSON(http.StatusOK, ItemListResponse{Items: out})
	}
}

// getItem godoc
//
//	@Summary		Get an item
//	@Description	Get one item, its live price, and its active offer (fixed-price listing or auction) if any.
//	@Tags			items
//	@Produce		json
//	@Param			id	path		int	true	"Item ID"
//	@Success		200	{object}	ItemDetailResponse
//	@Failure		404	{object}	ErrorResponse
//	@Router			/items/{id} [get]
func getItem(deps Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseUintParam(c, "id")
		if !ok {
			return
		}
		item, err := deps.Items.GetItem(c.Request.Context(), id)
		if err != nil {
			respondError(c, deps.Logger, err)
			return
		}
		resp := ItemDetailResponse{Item: deps.itemResponse(id, item)}
		if l, err := deps.Listings.OpenListingByItem(c.Request.Context(), id); err == nil && l != nil {
			lr := toListingResponse(l)
			resp.Listing = &lr
		}
		if a, err := deps.Auctions.ActiveAuctionByItem(c.Request.Context(), id); err == nil && a != nil {
			ar := toAuctionResponse(a)
			resp.Auction = &ar
		}
		c.JSON(http.StatusOK, resp)
	}
}

// listItemForSale godoc
//
//	@Summary		List an item for sale
//	@Description	Offer a Common/Rare item at a fixed price (a limit order). Legendary items must use an auction instead.
//	@Tags			items
//	@Accept			json
//	@Produce		json
//	@Param			id				path		int					true	"Item ID"
//	@Param			Idempotency-Key	header		string				false	"Idempotency key to make retries safe"
//	@Param			request			body		ListForSaleRequest	true	"Price and seller"
//	@Success		201				{object}	ListingResponse
//	@Failure		400				{object}	ErrorResponse
//	@Failure		404				{object}	ErrorResponse
//	@Failure		409				{object}	ErrorResponse
//	@Router			/items/{id}/list [post]
func listItemForSale(deps Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseUintParam(c, "id")
		if !ok {
			return
		}
		var req ListForSaleRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}
		listing, err := deps.Listings.CreateListing(c.Request.Context(), req.SellerGuildID, id, req.Price)
		if err != nil {
			respondError(c, deps.Logger, err)
			return
		}
		c.JSON(http.StatusCreated, toListingResponse(listing))
	}
}

// openAuction godoc
//
//	@Summary		Open an auction
//	@Description	Open an auction for a Legendary item. Only one active auction per item is allowed.
//	@Tags			items
//	@Accept			json
//	@Produce		json
//	@Param			id				path		int					true	"Item ID"
//	@Param			Idempotency-Key	header		string				false	"Idempotency key to make retries safe"
//	@Param			request			body		OpenAuctionRequest	true	"Seller"
//	@Success		201				{object}	AuctionResponse
//	@Failure		400				{object}	ErrorResponse
//	@Failure		404				{object}	ErrorResponse
//	@Failure		409				{object}	ErrorResponse
//	@Router			/items/{id}/auction [post]
func openAuction(deps Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseUintParam(c, "id")
		if !ok {
			return
		}
		var req OpenAuctionRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}
		a, err := deps.Auctions.CreateAuction(c.Request.Context(), req.SellerGuildID, id)
		if err != nil {
			respondError(c, deps.Logger, err)
			return
		}
		c.JSON(http.StatusCreated, toAuctionResponse(a))
	}
}

// buyItem godoc
//
//	@Summary		Buy an item
//	@Description	Buy a fixed-price item. Checks the balance and daily cap, moves money, and transfers the item in one transaction.
//	@Tags			items
//	@Accept			json
//	@Produce		json
//	@Param			id				path		int			true	"Item ID"
//	@Param			Idempotency-Key	header		string		false	"Idempotency key to make retries safe"
//	@Param			request			body		BuyRequest	true	"Buyer"
//	@Success		200				{object}	ListingResponse
//	@Failure		400				{object}	ErrorResponse
//	@Failure		402				{object}	ErrorResponse
//	@Failure		404				{object}	ErrorResponse
//	@Failure		409				{object}	ErrorResponse
//	@Router			/items/{id}/buy [post]
func buyItem(deps Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseUintParam(c, "id")
		if !ok {
			return
		}
		var req BuyRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}
		listing, err := deps.Listings.BuyByItem(c.Request.Context(), req.BuyerGuildID, id)
		if err != nil {
			respondError(c, deps.Logger, err)
			return
		}
		c.JSON(http.StatusOK, toListingResponse(listing))
	}
}

// placeBid godoc
//
//	@Summary		Place a bid
//	@Description	Place a bid on an item's active auction. A new bid must be at least 5% above the current highest bid. Funds are reserved, not debited.
//	@Tags			items
//	@Accept			json
//	@Produce		json
//	@Param			id				path		int				true	"Item ID"
//	@Param			Idempotency-Key	header		string			false	"Idempotency key to make retries safe"
//	@Param			request			body		PlaceBidRequest	true	"Bid to place"
//	@Success		201				{object}	BidResponse
//	@Failure		400				{object}	ErrorResponse
//	@Failure		402				{object}	ErrorResponse
//	@Failure		404				{object}	ErrorResponse
//	@Failure		409				{object}	ErrorResponse
//	@Router			/items/{id}/bid [post]
func placeBid(deps Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseUintParam(c, "id")
		if !ok {
			return
		}
		var req PlaceBidRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}
		bid, err := deps.Auctions.PlaceBidOnItem(c.Request.Context(), id, req.BidderGuildID, req.Amount)
		if err != nil {
			respondError(c, deps.Logger, err)
			return
		}
		c.JSON(http.StatusCreated, toBidResponse(bid))
	}
}

// cancelBid godoc
//
//	@Summary		Cancel a bid
//	@Description	Cancel a bid on an item's active auction. You cannot cancel a bid while you are the highest bidder.
//	@Tags			items
//	@Accept			json
//	@Produce		json
//	@Param			id				path		int					true	"Item ID"
//	@Param			bid_id			path		int					true	"Bid ID"
//	@Param			Idempotency-Key	header		string				false	"Idempotency key to make retries safe"
//	@Param			request			body		CancelBidRequest	true	"Bidder"
//	@Success		200				{object}	StatusResponse
//	@Failure		400				{object}	ErrorResponse
//	@Failure		404				{object}	ErrorResponse
//	@Router			/items/{id}/bid/{bid_id} [delete]
func cancelBid(deps Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		itemID, ok := parseUintParam(c, "id")
		if !ok {
			return
		}
		bidID, ok := parseUintParam(c, "bid_id")
		if !ok {
			return
		}
		var req CancelBidRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}
		if err := deps.Auctions.CancelBidOnItem(c.Request.Context(), itemID, bidID, req.BidderGuildID); err != nil {
			respondError(c, deps.Logger, err)
			return
		}
		c.JSON(http.StatusOK, StatusResponse{Status: "cancelled"})
	}
}
