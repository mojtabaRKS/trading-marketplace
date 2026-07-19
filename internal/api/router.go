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

	// State-changing endpoints are guarded by the idempotency middleware so
	// retries/duplicates cannot cause a double effect.
	mutating := r.Group("")
	if deps.DB != nil {
		mutating.Use(middleware.Idempotency(deps.DB, deps.Logger))
	}
	mutating.POST("/listings", createListing(deps))
	mutating.POST("/listings/:id/buy", buyListing(deps))
	mutating.POST("/auctions", createAuction(deps))
	mutating.POST("/auctions/:id/bids", placeBid(deps))
	mutating.DELETE("/auctions/:id/bids/:bidId", cancelBid(deps))

	r.GET("/auctions/:id", getAuction(deps))
	r.GET("/auctions/:id/bids", listBids(deps))
	r.GET("/prices/:id", getPrice(deps))

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
