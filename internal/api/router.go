// Package api holds the HTTP server: routing and server lifecycle. Middlewares
// live in the api/middleware package.
package api

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/herotech/market-dragon/internal/api/middleware"
)

// NewRouter builds the Gin engine, wiring middlewares and routes.
func NewRouter(logger *slog.Logger) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	r.Use(middleware.Recovery())
	r.Use(middleware.RequestLogger(logger))

	r.GET("/healthz", healthz)

	return r
}

// healthz is a liveness probe.
func healthz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
