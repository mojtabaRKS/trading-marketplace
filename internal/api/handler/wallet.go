package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/herotech/market-dragon/internal/api/dto"
)

// GetWallet godoc
//
//	@Summary		Get a guild wallet
//	@Description	Return a guild's wallet balance. Available balance is Total minus Reserved.
//	@Tags			guilds
//	@Produce		json
//	@Param			id	path		int	true	"Guild ID"
//	@Success		200	{object}	dto.WalletResponse
//	@Failure		404	{object}	dto.ErrorResponse
//	@Router			/guilds/{id}/wallet [get]
func GetWallet(deps Deps) gin.HandlerFunc {
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
		c.JSON(http.StatusOK, dto.WalletResponse{
			GuildID:   b.GuildID,
			Total:     b.Total,
			Reserved:  b.Reserved,
			Available: b.Available,
		})
	}
}
