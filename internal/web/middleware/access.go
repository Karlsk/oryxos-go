// Package middleware provides HTTP request-boundary middleware.
package middleware

import (
	"log/slog"
	"time"

	"github.com/Karlsk/oryxos-go/internal/observability"
	"github.com/gin-gonic/gin"
)

// AccessObservation records and logs the final state of a completed request.
func AccessObservation(observer observability.Observer, baseLogger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()

		ctx := c.Request.Context()
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		duration := time.Since(started)
		status := c.Writer.Status()
		observer.ObserveHTTP(ctx, c.Request.Method, route, status, duration)
		observability.Logger(ctx, baseLogger).LogAttrs(ctx, slog.LevelInfo, "http.request_complete",
			slog.String("method", c.Request.Method),
			slog.String("route", route),
			slog.Int("status", status),
			slog.Int64("duration_ms", duration.Milliseconds()),
		)
	}
}
