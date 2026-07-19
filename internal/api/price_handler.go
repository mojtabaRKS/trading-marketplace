package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// getPrice returns the current validated base price for an item, or 404 if no
// good price has been observed yet.
func getPrice(deps Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseUintParam(c, "id")
		if !ok {
			return
		}
		p, err := deps.Oracle.CurrentPrice(id)
		if err != nil {
			respondError(c, deps.Logger, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"item_id": p.ItemID, "price": p.Amount})
	}
}
