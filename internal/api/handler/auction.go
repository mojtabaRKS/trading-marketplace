package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/herotech/market-dragon/internal/api/dto"
)

// ListAuctions godoc
//
//	@Summary		List active auctions
//	@Description	List every auction that is currently open for bids.
//	@Tags			auctions
//	@Produce		json
//	@Success		200	{object}	dto.AuctionListResponse
//	@Router			/auctions [get]
func ListAuctions(deps Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		auctions, err := deps.Auctions.ListActiveAuctions(c.Request.Context())
		if err != nil {
			respondError(c, deps.Logger, err)
			return
		}
		out := make([]dto.AuctionResponse, 0, len(auctions))
		for i := range auctions {
			out = append(out, dto.NewAuctionResponse(&auctions[i]))
		}
		c.JSON(http.StatusOK, dto.AuctionListResponse{Auctions: out})
	}
}

// GetAuction godoc
//
//	@Summary		Get an auction
//	@Description	Get one auction and its current highest bid (if any).
//	@Tags			auctions
//	@Produce		json
//	@Param			id	path		int	true	"Auction ID"
//	@Success		200	{object}	dto.AuctionDetailResponse
//	@Failure		404	{object}	dto.ErrorResponse
//	@Router			/auctions/{id} [get]
func GetAuction(deps Deps) gin.HandlerFunc {
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
		resp := dto.AuctionDetailResponse{Auction: dto.NewAuctionResponse(a)}
		if hb, err := deps.Auctions.HighestBid(c.Request.Context(), id); err == nil && hb != nil {
			b := dto.NewBidResponse(hb)
			resp.HighestBid = &b
		}
		c.JSON(http.StatusOK, resp)
	}
}
