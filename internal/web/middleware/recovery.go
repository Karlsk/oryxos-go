package middleware

import (
	"log/slog"
	"net/http"

	"github.com/Karlsk/oryxos-go/internal/observability"
	"github.com/Karlsk/oryxos-go/internal/web/api"
	"github.com/gin-gonic/gin"
)

// Recovery converts uncommitted handler panics into the shared safe error envelope.
func Recovery(baseLogger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recover() == nil {
				return
			}
			observability.Logger(c.Request.Context(), baseLogger).LogAttrs(
				c.Request.Context(), slog.LevelError, "http.recovered_panic",
			)
			c.Abort()
			if !c.Writer.Written() {
				api.Error(c, http.StatusInternalServerError, "internal", "internal server error", nil)
			}
		}()
		c.Next()
	}
}
