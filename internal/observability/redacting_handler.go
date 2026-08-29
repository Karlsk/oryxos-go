package observability

import (
	"context"
	"log/slog"
	"net/url"
	"strings"

	"github.com/Karlsk/oryxos-go/internal/config"
)

// RedactingHandler sanitizes every structured attribute before it reaches its sink.
type RedactingHandler struct {
	next   slog.Handler
	groups []string
}

// NewRedactingHandler wraps next with recursive sensitive-data redaction.
func NewRedactingHandler(next slog.Handler) slog.Handler {
	return &RedactingHandler{next: next}
}

// Enabled implements slog.Handler.
func (h *RedactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

// Handle implements slog.Handler.
func (h *RedactingHandler) Handle(ctx context.Context, record slog.Record) error {
	message := config.SanitizeErrorString(record.Message)
	sanitized := slog.NewRecord(record.Time, record.Level, message, record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		sanitized.AddAttrs(sanitizeAttr(attr, h.groups))
		return true
	})
	return h.next.Handle(ctx, sanitized)
}

// WithAttrs implements slog.Handler.
func (h *RedactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	sanitized := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		sanitized = append(sanitized, sanitizeAttr(attr, h.groups))
	}
	return &RedactingHandler{
		next:   h.next.WithAttrs(sanitized),
		groups: append([]string(nil), h.groups...),
	}
}

// WithGroup implements slog.Handler.
func (h *RedactingHandler) WithGroup(name string) slog.Handler {
	return &RedactingHandler{
		next:   h.next.WithGroup(name),
		groups: append(append([]string(nil), h.groups...), name),
	}
}

func sanitizeAttr(attr slog.Attr, groups []string) slog.Attr {
	attr.Value = attr.Value.Resolve()
	if config.IsSensitiveKey(groups, attr.Key) {
		return slog.String(attr.Key, "[REDACTED]")
	}
	if isErrorKey(attr.Key) {
		return slog.String(attr.Key, "[REDACTED]")
	}
	if attr.Value.Kind() == slog.KindString {
		text := attr.Value.String()
		if sanitized := config.SanitizeErrorString(text); sanitized != text {
			return slog.String(attr.Key, sanitized)
		}
	}
	if attr.Value.Kind() == slog.KindAny {
		if text, ok := safeURLText(attr.Value.Any()); ok {
			return slog.String(attr.Key, config.SanitizeErrorString(text))
		}
		return slog.String(attr.Key, "[REDACTED]")
	}
	if attr.Value.Kind() != slog.KindGroup {
		return attr
	}

	nextGroups := append(append([]string(nil), groups...), attr.Key)
	children := attr.Value.Group()
	sanitized := make([]any, 0, len(children))
	for _, child := range children {
		sanitized = append(sanitized, sanitizeAttr(child, nextGroups))
	}
	return slog.Group(attr.Key, sanitized...)
}

func safeURLText(value any) (string, bool) {
	switch typed := value.(type) {
	case url.URL:
		return typed.String(), true
	case *url.URL:
		if typed == nil {
			return "", false
		}
		return typed.String(), true
	default:
		return "", false
	}
}

func isErrorKey(key string) bool {
	normalized := strings.ToLower(key)
	return normalized == "error" || strings.HasSuffix(normalized, "_error")
}
