package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/herotech/market-dragon/internal/repository"
)

type createListingRequest struct {
	SellerGuildID uint64 `json:"seller_guild_id" binding:"required"`
	ItemID        uint64 `json:"item_id" binding:"required"`
	Price         int64  `json:"price" binding:"required"`
}

type buyRequest struct {
	BuyerGuildID uint64 `json:"buyer_guild_id" binding:"required"`
}

type listingResponse struct {
	ID            uint64  `json:"id"`
	ItemID        uint64  `json:"item_id"`
	SellerGuildID uint64  `json:"seller_guild_id"`
	Price         int64   `json:"price"`
	Status        string  `json:"status"`
	BuyerGuildID  *uint64 `json:"buyer_guild_id,omitempty"`
}

func toListingResponse(l *repository.Listing) listingResponse {
	return listingResponse{
		ID:            l.ID,
		ItemID:        l.ItemID,
		SellerGuildID: l.SellerGuildID,
		Price:         l.Price,
		Status:        l.Status,
		BuyerGuildID:  l.BuyerGuildID,
	}
}

func createListing(deps Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req createListingRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		listing, err := deps.Listings.CreateListing(c.Request.Context(), req.SellerGuildID, req.ItemID, req.Price)
		if err != nil {
			respondError(c, deps.Logger, err)
			return
		}
		c.JSON(http.StatusCreated, toListingResponse(listing))
	}
}

func buyListing(deps Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseUintParam(c, "id")
		if !ok {
			return
		}
		var req buyRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		listing, err := deps.Listings.Buy(c.Request.Context(), req.BuyerGuildID, id)
		if err != nil {
			respondError(c, deps.Logger, err)
			return
		}
		c.JSON(http.StatusOK, toListingResponse(listing))
	}
}
