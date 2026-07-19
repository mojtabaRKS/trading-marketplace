package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/herotech/market-dragon/internal/repository"
)

type createAuctionRequest struct {
	SellerGuildID uint64 `json:"seller_guild_id" binding:"required"`
	ItemID        uint64 `json:"item_id" binding:"required"`
}

type placeBidRequest struct {
	BidderGuildID uint64 `json:"bidder_guild_id" binding:"required"`
	Amount        int64  `json:"amount" binding:"required"`
}

type auctionResponse struct {
	ID            uint64  `json:"id"`
	ItemID        uint64  `json:"item_id"`
	SellerGuildID uint64  `json:"seller_guild_id"`
	Status        string  `json:"status"`
	StartsAt      string  `json:"starts_at"`
	EndsAt        string  `json:"ends_at"`
	HighestBidID  *uint64 `json:"highest_bid_id,omitempty"`
	WinnerGuildID *uint64 `json:"winner_guild_id,omitempty"`
}

type bidResponse struct {
	ID            uint64 `json:"id"`
	AuctionID     uint64 `json:"auction_id"`
	BidderGuildID uint64 `json:"bidder_guild_id"`
	Amount        int64  `json:"amount"`
	Status        string `json:"status"`
}

func toAuctionResponse(a *repository.Auction) auctionResponse {
	return auctionResponse{
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

func toBidResponse(b *repository.Bid) bidResponse {
	return bidResponse{
		ID:            b.ID,
		AuctionID:     b.AuctionID,
		BidderGuildID: b.BidderGuildID,
		Amount:        b.Amount,
		Status:        b.Status,
	}
}

func createAuction(deps Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req createAuctionRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		a, err := deps.Auctions.CreateAuction(c.Request.Context(), req.SellerGuildID, req.ItemID)
		if err != nil {
			respondError(c, deps.Logger, err)
			return
		}
		c.JSON(http.StatusCreated, toAuctionResponse(a))
	}
}

func placeBid(deps Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseUintParam(c, "id")
		if !ok {
			return
		}
		var req placeBidRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		bid, err := deps.Auctions.PlaceBid(c.Request.Context(), id, req.BidderGuildID, req.Amount)
		if err != nil {
			respondError(c, deps.Logger, err)
			return
		}
		c.JSON(http.StatusCreated, toBidResponse(bid))
	}
}

func cancelBid(deps Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		auctionID, ok := parseUintParam(c, "id")
		if !ok {
			return
		}
		bidID, ok := parseUintParam(c, "bidId")
		if !ok {
			return
		}
		var req struct {
			BidderGuildID uint64 `json:"bidder_guild_id" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := deps.Auctions.CancelBid(c.Request.Context(), auctionID, bidID, req.BidderGuildID); err != nil {
			respondError(c, deps.Logger, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "cancelled"})
	}
}

func getAuction(deps Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseUintParam(c, "id")
		if !ok {
			return
		}
		a, err := deps.Auctions.GetAuction(c.Request.Context(), id)
		if err != nil {
			respondError(c, deps.Logger, err)
			return
		}
		resp := gin.H{"auction": toAuctionResponse(a)}
		if hb, err := deps.Auctions.HighestBid(c.Request.Context(), id); err == nil && hb != nil {
			b := toBidResponse(hb)
			resp["highest_bid"] = b
		}
		c.JSON(http.StatusOK, resp)
	}
}

func listBids(deps Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseUintParam(c, "id")
		if !ok {
			return
		}
		bids, err := deps.Auctions.ListBids(c.Request.Context(), id)
		if err != nil {
			respondError(c, deps.Logger, err)
			return
		}
		out := make([]bidResponse, 0, len(bids))
		for i := range bids {
			out = append(out, toBidResponse(&bids[i]))
		}
		c.JSON(http.StatusOK, gin.H{"bids": out})
	}
}
