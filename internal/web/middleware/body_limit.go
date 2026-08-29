package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// RequestBodyLimit bounds request bodies before a handler can decode them.
func RequestBodyLimit(limit int64) gin.HandlerFunc {
	if limit <= 0 {
		panic("request body limit must be positive")
	}
	return func(c *gin.Context) {
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		}
		c.Next()
	}
}
