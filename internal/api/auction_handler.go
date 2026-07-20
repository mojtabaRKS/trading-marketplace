package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/herotech/market-dragon/internal/repository"
)

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

// StatusResponse is a simple status acknowledgement.
type StatusResponse struct {
	Status string `json:"status" example:"cancelled"`
}

func toAuctionResponse(a *repository.Auction) AuctionResponse {
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

func toBidResponse(b *repository.Bid) BidResponse {
	return BidResponse{
		ID:            b.ID,
		AuctionID:     b.AuctionID,
		BidderGuildID: b.BidderGuildID,
		Amount:        b.Amount,
		Status:        b.Status,
	}
}

// listAuctions godoc
//
//	@Summary		List active auctions
//	@Description	List every auction that is currently open for bids.
//	@Tags			auctions
//	@Produce		json
//	@Success		200	{object}	AuctionListResponse
//	@Router			/auctions [get]
func listAuctions(deps Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		auctions, err := deps.Auctions.ListActiveAuctions(c.Request.Context())
		if err != nil {
			respondError(c, deps.Logger, err)
			return
		}
		out := make([]AuctionResponse, 0, len(auctions))
		for i := range auctions {
			out = append(out, toAuctionResponse(&auctions[i]))
		}
		c.JSON(http.StatusOK, AuctionListResponse{Auctions: out})
	}
}

// getAuction godoc
//
//	@Summary		Get an auction
//	@Description	Get one auction and its current highest bid (if any).
//	@Tags			auctions
//	@Produce		json
//	@Param			id	path		int	true	"Auction ID"
//	@Success		200	{object}	AuctionDetailResponse
//	@Failure		404	{object}	ErrorResponse
//	@Router			/auctions/{id} [get]
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
		resp := AuctionDetailResponse{Auction: toAuctionResponse(a)}
		if hb, err := deps.Auctions.HighestBid(c.Request.Context(), id); err == nil && hb != nil {
			b := toBidResponse(hb)
			resp.HighestBid = &b
		}
		c.JSON(http.StatusOK, resp)
	}
}
