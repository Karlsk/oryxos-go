package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"io"

	"github.com/Karlsk/oryxos-go/internal/observability"
	"github.com/gin-gonic/gin"
)

// RequestID correlates each request with a validated client ID or opaque replacement.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if !validRequestID(requestID) {
			requestID = newRequestID()
		}
		c.Header("X-Request-ID", requestID)
		c.Request = c.Request.WithContext(observability.WithCorrelation(
			c.Request.Context(), observability.Correlation{RequestID: requestID},
		))
		c.Next()
	}
}

func validRequestID(value string) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		//nolint:staticcheck // Explicit ASCII ranges are the public request-ID contract.
		if !(character >= 'a' && character <= 'z') &&
			!(character >= 'A' && character <= 'Z') &&
			!(character >= '0' && character <= '9') &&
			character != '.' && character != '_' && character != '-' {
			return false
		}
	}
	return true
}

func newRequestID() string {
	var bytes [16]byte
	if _, err := io.ReadFull(rand.Reader, bytes[:]); err != nil {
		panic("unable to generate request ID")
	}
	return "req_" + hex.EncodeToString(bytes[:])
}
