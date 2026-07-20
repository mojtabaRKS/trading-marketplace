package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/herotech/market-dragon/internal/api/dto"
)

// parseUintParam parses a uint64 path parameter, writing a 400 on failure.
func parseUintParam(c *gin.Context, name string) (uint64, bool) {
	v, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "invalid " + name})
		return 0, false
	}
	return v, true
}
