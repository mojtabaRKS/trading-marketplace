// Package api holds the HTTP server: routing and server lifecycle. Handlers
// live in the api/handler package, request/response types in api/dto, and
// middlewares in the api/middleware package.
package api

import (
	swaggerfiles "github.com/swaggo/files"
	ginswagger "github.com/swaggo/gin-swagger"

	"github.com/gin-gonic/gin"

	_ "github.com/herotech/market-dragon/docs"
	"github.com/herotech/market-dragon/internal/api/handler"
	"github.com/herotech/market-dragon/internal/api/middleware"
)

// NewRouter builds the Gin engine, wiring middlewares and routes.
func NewRouter(deps handler.Deps) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	r.Use(middleware.Recovery())
	r.Use(middleware.RequestLogger(deps.Logger))

	r.GET("/health", handler.Health)

	// Interactive API docs (generated from swaggo annotations).
	r.GET("/swagger/*any", ginswagger.WrapHandler(swaggerfiles.Handler))

	// Reads.
	r.GET("/items", handler.ListItems(deps))
	r.GET("/items/:id", handler.GetItem(deps))
	r.GET("/auctions", handler.ListAuctions(deps))
	r.GET("/auctions/:id", handler.GetAuction(deps))
	r.GET("/guilds/:id/wallet", handler.GetWallet(deps))

	// State-changing endpoints are guarded by the idempotency middleware so
	// retries/duplicates cannot cause a double effect.
	mutating := r.Group("")
	if deps.DB != nil {
		mutating.Use(middleware.Idempotency(deps.DB, deps.Logger))
	}
	mutating.POST("/items", handler.CreateItem(deps))
	mutating.POST("/items/:id/list", handler.ListItemForSale(deps))
	mutating.POST("/items/:id/auction", handler.OpenAuction(deps))
	mutating.POST("/items/:id/buy", handler.BuyItem(deps))
	mutating.POST("/items/:id/bid", handler.PlaceBid(deps))
	mutating.DELETE("/items/:id/bid/:bid_id", handler.CancelBid(deps))

	return r
}
