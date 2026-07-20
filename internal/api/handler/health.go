package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/herotech/market-dragon/internal/api/dto"
)

// Health godoc
//
//	@Summary		Liveness probe
//	@Description	Return 200 while the service is up.
//	@Tags			system
//	@Produce		json
//	@Success		200	{object}	dto.HealthResponse
//	@Router			/health [get]
func Health(c *gin.Context) {
	c.JSON(http.StatusOK, dto.HealthResponse{Status: "ok"})
}
