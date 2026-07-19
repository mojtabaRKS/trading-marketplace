// Package api holds the HTTP server: routing and server lifecycle. Middlewares
// live in the api/middleware package.
package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/herotech/market-dragon/internal/api/middleware"
)

// NewRouter builds the Gin engine, wiring middlewares and routes.
func NewRouter(deps Deps) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	r.Use(middleware.Recovery())
	r.Use(middleware.RequestLogger(deps.Logger))

	r.GET("/healthz", healthz)

	r.POST("/listings", createListing(deps))
	r.POST("/listings/:id/buy", buyListing(deps))

	r.POST("/auctions", createAuction(deps))
	r.GET("/auctions/:id", getAuction(deps))
	r.POST("/auctions/:id/bids", placeBid(deps))
	r.GET("/auctions/:id/bids", listBids(deps))
	r.DELETE("/auctions/:id/bids/:bidId", cancelBid(deps))

	return r
}

// healthz is a liveness probe.
func healthz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// parseUintParam parses a uint64 path parameter, writing a 400 on failure.
func parseUintParam(c *gin.Context, name string) (uint64, bool) {
	v, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid " + name})
		return 0, false
	}
	return v, true
}
