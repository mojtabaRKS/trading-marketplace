package middleware

import "github.com/gin-gonic/gin"

// Recovery recovers from panics and writes a 500, keeping the server alive.
func Recovery() gin.HandlerFunc {
	return gin.Recovery()
}
