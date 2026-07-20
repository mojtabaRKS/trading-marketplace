// Package api holds the HTTP server: routing and server lifecycle. Middlewares
// live in the api/middleware package.
package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	swaggerfiles "github.com/swaggo/files"
	ginswagger "github.com/swaggo/gin-swagger"

	_ "github.com/herotech/market-dragon/docs"
	"github.com/herotech/market-dragon/internal/api/middleware"
)

// NewRouter builds the Gin engine, wiring middlewares and routes.
func NewRouter(deps Deps) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	r.Use(middleware.Recovery())
	r.Use(middleware.RequestLogger(deps.Logger))

	r.GET("/health", health)

	// Interactive API docs (generated from swaggo annotations).
	r.GET("/swagger/*any", ginswagger.WrapHandler(swaggerfiles.Handler))

	// Reads.
	r.GET("/items", listItems(deps))
	r.GET("/items/:id", getItem(deps))
	r.GET("/auctions", listAuctions(deps))
	r.GET("/auctions/:id", getAuction(deps))
	r.GET("/guilds/:id/wallet", getWallet(deps))

	// State-changing endpoints are guarded by the idempotency middleware so
	// retries/duplicates cannot cause a double effect.
	mutating := r.Group("")
	if deps.DB != nil {
		mutating.Use(middleware.Idempotency(deps.DB, deps.Logger))
	}
	mutating.POST("/items", createItem(deps))
	mutating.POST("/items/:id/list", listItemForSale(deps))
	mutating.POST("/items/:id/auction", openAuction(deps))
	mutating.POST("/items/:id/buy", buyItem(deps))
	mutating.POST("/items/:id/bid", placeBid(deps))
	mutating.DELETE("/items/:id/bid/:bid_id", cancelBid(deps))

	return r
}

// HealthResponse is the liveness probe body.
type HealthResponse struct {
	Status string `json:"status" example:"ok"`
}

// health godoc
//
//	@Summary		Liveness probe
//	@Description	Return 200 while the service is up.
//	@Tags			system
//	@Produce		json
//	@Success		200	{object}	HealthResponse
//	@Router			/health [get]
func health(c *gin.Context) {
	c.JSON(http.StatusOK, HealthResponse{Status: "ok"})
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
