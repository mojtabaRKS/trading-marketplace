package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/herotech/market-dragon/internal/api/dto"
	"github.com/herotech/market-dragon/internal/repository"
)

// itemResponse builds an ItemResponse and attaches the current Oracle price
// (when one is available).
func (d Deps) itemResponse(itemID uint64, item *repository.Item) dto.ItemResponse {
	resp := dto.NewItemResponse(item)
	if p, err := d.Oracle.CurrentPrice(itemID); err == nil {
		amount := p.Amount
		resp.Price = &amount
	}
	return resp
}

// CreateItem godoc
//
//	@Summary		Register an item
//	@Description	Register a new item owned by a guild. The item is not for sale yet. Legendary items are unique (stock is forced to 1).
//	@Tags			items
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string					false	"Idempotency key to make retries safe"
//	@Param			request			body		dto.CreateItemRequest	true	"Item to register"
//	@Success		201				{object}	dto.ItemResponse
//	@Failure		400				{object}	dto.ErrorResponse
//	@Failure		404				{object}	dto.ErrorResponse
//	@Router			/items [post]
func CreateItem(deps Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req dto.CreateItemRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
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

// ListItems godoc
//
//	@Summary		List items
//	@Description	List every item with its current validated Oracle price (when available).
//	@Tags			items
//	@Produce		json
//	@Success		200	{object}	dto.ItemListResponse
//	@Router			/items [get]
func ListItems(deps Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		items, err := deps.Items.ListItems(c.Request.Context())
		if err != nil {
			respondError(c, deps.Logger, err)
			return
		}
		out := make([]dto.ItemResponse, 0, len(items))
		for i := range items {
			out = append(out, deps.itemResponse(items[i].ID, &items[i]))
		}
		c.JSON(http.StatusOK, dto.ItemListResponse{Items: out})
	}
}

// GetItem godoc
//
//	@Summary		Get an item
//	@Description	Get one item, its live price, and its active offer (fixed-price listing or auction) if any.
//	@Tags			items
//	@Produce		json
//	@Param			id	path		int	true	"Item ID"
//	@Success		200	{object}	dto.ItemDetailResponse
//	@Failure		404	{object}	dto.ErrorResponse
//	@Router			/items/{id} [get]
func GetItem(deps Deps) gin.HandlerFunc {
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
		resp := dto.ItemDetailResponse{Item: deps.itemResponse(id, item)}
		if l, err := deps.Listings.OpenListingByItem(c.Request.Context(), id); err == nil && l != nil {
			lr := dto.NewListingResponse(l)
			resp.Listing = &lr
		}
		if a, err := deps.Auctions.ActiveAuctionByItem(c.Request.Context(), id); err == nil && a != nil {
			ar := dto.NewAuctionResponse(a)
			resp.Auction = &ar
		}
		c.JSON(http.StatusOK, resp)
	}
}

// ListItemForSale godoc
//
//	@Summary		List an item for sale
//	@Description	Offer a Common/Rare item at a fixed price (a limit order). Legendary items must use an auction instead.
//	@Tags			items
//	@Accept			json
//	@Produce		json
//	@Param			id				path		int						true	"Item ID"
//	@Param			Idempotency-Key	header		string					false	"Idempotency key to make retries safe"
//	@Param			request			body		dto.ListForSaleRequest	true	"Price and seller"
//	@Success		201				{object}	dto.ListingResponse
//	@Failure		400				{object}	dto.ErrorResponse
//	@Failure		404				{object}	dto.ErrorResponse
//	@Failure		409				{object}	dto.ErrorResponse
//	@Router			/items/{id}/list [post]
func ListItemForSale(deps Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseUintParam(c, "id")
		if !ok {
			return
		}
		var req dto.ListForSaleRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
			return
		}
		listing, err := deps.Listings.CreateListing(c.Request.Context(), req.SellerGuildID, id, req.Price)
		if err != nil {
			respondError(c, deps.Logger, err)
			return
		}
		c.JSON(http.StatusCreated, dto.NewListingResponse(listing))
	}
}

// OpenAuction godoc
//
//	@Summary		Open an auction
//	@Description	Open an auction for a Legendary item. Only one active auction per item is allowed.
//	@Tags			items
//	@Accept			json
//	@Produce		json
//	@Param			id				path		int						true	"Item ID"
//	@Param			Idempotency-Key	header		string					false	"Idempotency key to make retries safe"
//	@Param			request			body		dto.OpenAuctionRequest	true	"Seller"
//	@Success		201				{object}	dto.AuctionResponse
//	@Failure		400				{object}	dto.ErrorResponse
//	@Failure		404				{object}	dto.ErrorResponse
//	@Failure		409				{object}	dto.ErrorResponse
//	@Router			/items/{id}/auction [post]
func OpenAuction(deps Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseUintParam(c, "id")
		if !ok {
			return
		}
		var req dto.OpenAuctionRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
			return
		}
		a, err := deps.Auctions.CreateAuction(c.Request.Context(), req.SellerGuildID, id)
		if err != nil {
			respondError(c, deps.Logger, err)
			return
		}
		c.JSON(http.StatusCreated, dto.NewAuctionResponse(a))
	}
}

// BuyItem godoc
//
//	@Summary		Buy an item
//	@Description	Buy a fixed-price item. Checks the balance and daily cap, moves money, and transfers the item in one transaction.
//	@Tags			items
//	@Accept			json
//	@Produce		json
//	@Param			id				path		int				true	"Item ID"
//	@Param			Idempotency-Key	header		string			false	"Idempotency key to make retries safe"
//	@Param			request			body		dto.BuyRequest	true	"Buyer"
//	@Success		200				{object}	dto.ListingResponse
//	@Failure		400				{object}	dto.ErrorResponse
//	@Failure		402				{object}	dto.ErrorResponse
//	@Failure		404				{object}	dto.ErrorResponse
//	@Failure		409				{object}	dto.ErrorResponse
//	@Router			/items/{id}/buy [post]
func BuyItem(deps Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseUintParam(c, "id")
		if !ok {
			return
		}
		var req dto.BuyRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
			return
		}
		listing, err := deps.Listings.BuyByItem(c.Request.Context(), req.BuyerGuildID, id)
		if err != nil {
			respondError(c, deps.Logger, err)
			return
		}
		c.JSON(http.StatusOK, dto.NewListingResponse(listing))
	}
}

// PlaceBid godoc
//
//	@Summary		Place a bid
//	@Description	Place a bid on an item's active auction. A new bid must be at least 5% above the current highest bid. Funds are reserved, not debited.
//	@Tags			items
//	@Accept			json
//	@Produce		json
//	@Param			id				path		int					true	"Item ID"
//	@Param			Idempotency-Key	header		string				false	"Idempotency key to make retries safe"
//	@Param			request			body		dto.PlaceBidRequest	true	"Bid to place"
//	@Success		201				{object}	dto.BidResponse
//	@Failure		400				{object}	dto.ErrorResponse
//	@Failure		402				{object}	dto.ErrorResponse
//	@Failure		404				{object}	dto.ErrorResponse
//	@Failure		409				{object}	dto.ErrorResponse
//	@Router			/items/{id}/bid [post]
func PlaceBid(deps Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseUintParam(c, "id")
		if !ok {
			return
		}
		var req dto.PlaceBidRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
			return
		}
		bid, err := deps.Auctions.PlaceBidOnItem(c.Request.Context(), id, req.BidderGuildID, req.Amount)
		if err != nil {
			respondError(c, deps.Logger, err)
			return
		}
		c.JSON(http.StatusCreated, dto.NewBidResponse(bid))
	}
}

// CancelBid godoc
//
//	@Summary		Cancel a bid
//	@Description	Cancel a bid on an item's active auction. You cannot cancel a bid while you are the highest bidder.
//	@Tags			items
//	@Accept			json
//	@Produce		json
//	@Param			id				path		int						true	"Item ID"
//	@Param			bid_id			path		int						true	"Bid ID"
//	@Param			Idempotency-Key	header		string					false	"Idempotency key to make retries safe"
//	@Param			request			body		dto.CancelBidRequest	true	"Bidder"
//	@Success		200				{object}	dto.StatusResponse
//	@Failure		400				{object}	dto.ErrorResponse
//	@Failure		404				{object}	dto.ErrorResponse
//	@Router			/items/{id}/bid/{bid_id} [delete]
func CancelBid(deps Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		itemID, ok := parseUintParam(c, "id")
		if !ok {
			return
		}
		bidID, ok := parseUintParam(c, "bid_id")
		if !ok {
			return
		}
		var req dto.CancelBidRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
			return
		}
		if err := deps.Auctions.CancelBidOnItem(c.Request.Context(), itemID, bidID, req.BidderGuildID); err != nil {
			respondError(c, deps.Logger, err)
			return
		}
		c.JSON(http.StatusOK, dto.StatusResponse{Status: "cancelled"})
	}
}
