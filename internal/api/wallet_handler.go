package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// WalletResponse is a guild's wallet balance. Available = Total - Reserved.
type WalletResponse struct {
	GuildID   uint64 `json:"guild_id" example:"1"`
	Total     int64  `json:"total" example:"100000"`
	Reserved  int64  `json:"reserved" example:"1200"`
	Available int64  `json:"available" example:"98800"`
}

// getWallet godoc
//
//	@Summary		Get a guild wallet
//	@Description	Return a guild's wallet balance. Available balance is Total minus Reserved.
//	@Tags			guilds
//	@Produce		json
//	@Param			id	path		int	true	"Guild ID"
//	@Success		200	{object}	WalletResponse
//	@Failure		404	{object}	ErrorResponse
//	@Router			/guilds/{id}/wallet [get]
func getWallet(deps Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseUintParam(c, "id")
		if !ok {
			return
		}
		b, err := deps.Wallets.Balance(c.Request.Context(), id)
		if err != nil {
			respondError(c, deps.Logger, err)
			return
		}
		c.JSON(http.StatusOK, WalletResponse{
			GuildID:   b.GuildID,
			Total:     b.Total,
			Reserved:  b.Reserved,
			Available: b.Available,
		})
	}
}
