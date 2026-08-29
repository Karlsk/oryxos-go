package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"net/http"
	"regexp"

	"github.com/Karlsk/oryxos-go/internal/observability"
	"github.com/gin-gonic/gin"
)

// Result is the canonical public HTTP response envelope.
type Result[T any] struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Data      *T             `json:"data,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
	RequestID string         `json:"request_id"`
}

var detailFieldPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

var errorDescriptors = map[int]map[string]string{
	http.StatusBadRequest: {
		"invalid_request":  "invalid request",
		"invalid_argument": "invalid argument",
	},
	http.StatusNotFound: {
		"not_found": "not found",
	},
	http.StatusMethodNotAllowed: {
		"method_not_allowed": "method not allowed",
	},
	http.StatusConflict: {
		"conflict": "conflict",
	},
	http.StatusRequestEntityTooLarge: {
		"payload_too_large": "payload too large",
	},
	http.StatusTooManyRequests: {
		"rate_limited": "rate limited",
	},
	http.StatusInternalServerError: {
		"internal": "internal server error",
	},
	http.StatusNotImplemented: {
		"not_implemented": "not implemented",
	},
	http.StatusServiceUnavailable: {
		"not_ready": "service not ready",
	},
}

var detailRules = map[string]struct{}{
	"required":       {},
	"invalid_format": {},
	"out_of_range":   {},
	"duplicate":      {},
	"too_large":      {},
}

// Success writes a validated success Result envelope.
func Success[T any](c *gin.Context, status int, code, message string, data T) {
	if !validSuccessDescriptor(status, code, message) {
		writeInternalFallback(c)
		return
	}
	write(c, status, Result[T]{
		Code:    code,
		Message: message,
		Data:    &data,
	})
}

// Page writes a validated paginated Result envelope.
func Page[T any](c *gin.Context, status int, code, message string, page PageResult[T]) {
	if !validSuccessDescriptor(status, code, message) || status != http.StatusOK || !validPage(page) {
		writeInternalFallback(c)
		return
	}
	if page.Items == nil {
		page.Items = []T{}
	}
	page.TotalPages = totalPages(page.Total, page.PageSize)
	write(c, status, Result[PageResult[T]]{
		Code:    code,
		Message: message,
		Data:    &page,
	})
}

// Error writes a validated error Result envelope.
func Error(c *gin.Context, status int, code, message string, details map[string]any) {
	if !validErrorDescriptor(status, code, message) {
		writeInternalFallback(c)
		return
	}
	safeDetails, ok := validateDetails(details)
	if !ok {
		writeInternalFallback(c)
		return
	}
	write(c, status, Result[struct{}]{
		Code:    code,
		Message: message,
		Details: safeDetails,
	})
}

func validSuccessDescriptor(status int, code, message string) bool {
	return (status == http.StatusOK && code == "ok" && message == "ok") ||
		(status == http.StatusCreated && code == "created" && message == "created")
}

func validErrorDescriptor(status int, code, message string) bool {
	messages, statusOK := errorDescriptors[status]
	expectedMessage, codeOK := messages[code]
	return statusOK && codeOK && expectedMessage == message
}

func validateDetails(details map[string]any) (map[string]any, bool) {
	if details == nil {
		return nil, true
	}
	if len(details) != 2 {
		return nil, false
	}
	field, fieldOK := details["field"].(string)
	rule, ruleOK := details["rule"].(string)
	if !fieldOK || !ruleOK || !detailFieldPattern.MatchString(field) {
		return nil, false
	}
	if _, ok := detailRules[rule]; !ok {
		return nil, false
	}
	return map[string]any{"field": field, "rule": rule}, true
}

func writeInternalFallback(c *gin.Context) {
	write(c, http.StatusInternalServerError, Result[struct{}]{
		Code:    "internal",
		Message: "internal server error",
	})
}

func write[T any](c *gin.Context, status int, result Result[T]) {
	if c.Writer.Written() {
		return
	}
	requestID := ensureRequestID(c)
	result.RequestID = requestID
	c.Header("X-Request-ID", requestID)
	c.JSON(status, result)
}

func ensureRequestID(c *gin.Context) string {
	ctx := context.Background()
	if c.Request != nil {
		ctx = c.Request.Context()
	}
	correlation := observability.CorrelationFromContext(ctx)
	if correlation.RequestID != "" {
		return correlation.RequestID
	}

	correlation.RequestID = emergencyRequestID()
	ctx = observability.WithCorrelation(ctx, correlation)
	if c.Request != nil {
		c.Request = c.Request.WithContext(ctx)
	}
	return correlation.RequestID
}

func emergencyRequestID() string {
	return emergencyRequestIDFrom(rand.Reader)
}

func emergencyRequestIDFrom(reader io.Reader) string {
	var bytes [16]byte
	if _, err := io.ReadFull(reader, bytes[:]); err != nil {
		panic("unable to generate emergency request ID")
	}
	return "req_" + hex.EncodeToString(bytes[:])
}
