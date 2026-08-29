package observability

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

type consoleHandler struct {
	writer io.Writer
	level  slog.Leveler
	attrs  []consoleAttr
	groups []string
	mu     *sync.Mutex
}

type consoleAttr struct {
	attr   slog.Attr
	groups []string
}

// NewConsoleHandler creates a standard-library slog handler for colored text logs.
func NewConsoleHandler(w io.Writer, level slog.Leveler) slog.Handler {
	return &consoleHandler{writer: w, level: level, mu: &sync.Mutex{}}
}

func (h *consoleHandler) Enabled(_ context.Context, level slog.Level) bool {
	return h.level == nil || level >= h.level.Level()
}

func (h *consoleHandler) Handle(_ context.Context, record slog.Record) error {
	var line strings.Builder
	line.WriteString(record.Time.UTC().Format("2006-01-02T15:04:05Z07:00"))
	line.WriteByte(' ')
	line.WriteString(coloredLevel(record.Level))
	line.WriteByte(' ')
	appendConsoleText(&line, record.Message)

	for _, attr := range h.attrs {
		appendConsoleAttr(&line, attr.groups, attr.attr)
	}
	record.Attrs(func(attr slog.Attr) bool {
		appendConsoleAttrs(&line, h.groups, []slog.Attr{attr})
		return true
	})
	line.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.writer, line.String())
	return err
}

func (h *consoleHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	derived := *h
	derived.attrs = append([]consoleAttr(nil), h.attrs...)
	for _, attr := range attrs {
		derived.attrs = append(derived.attrs, consoleAttr{
			attr:   attr,
			groups: append([]string(nil), h.groups...),
		})
	}
	return &derived
}

func (h *consoleHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	derived := *h
	derived.groups = append(append([]string(nil), h.groups...), name)
	return &derived
}

func coloredLevel(level slog.Level) string {
	var color string
	switch {
	case level >= slog.LevelError:
		color = "\x1b[31m"
	case level >= slog.LevelWarn:
		color = "\x1b[33m"
	case level >= slog.LevelInfo:
		color = "\x1b[32m"
	default:
		color = "\x1b[36m"
	}
	return color + level.String() + "\x1b[0m"
}

func appendConsoleAttrs(line *strings.Builder, groups []string, attrs []slog.Attr) {
	for _, attr := range attrs {
		appendConsoleAttr(line, groups, attr)
	}
}

func appendConsoleAttr(line *strings.Builder, groups []string, attr slog.Attr) {
	attr.Value = attr.Value.Resolve()
	if attr.Value.Kind() == slog.KindGroup {
		nextGroups := append(append([]string(nil), groups...), attr.Key)
		appendConsoleAttrs(line, nextGroups, attr.Value.Group())
		return
	}
	key := strings.Join(append(append([]string(nil), groups...), attr.Key), ".")
	line.WriteByte(' ')
	appendConsoleText(line, key)
	line.WriteByte('=')
	//nolint:staticcheck // StringBuilder preserves the console handler's deterministic attribute rendering.
	appendConsoleText(line, fmt.Sprint(attr.Value.Any()))
}

func appendConsoleText(line *strings.Builder, text string) {
	if strings.IndexFunc(text, unicode.IsControl) >= 0 || strings.ContainsAny(text, " =\\\"") {
		line.WriteString(strconv.QuoteToGraphic(text))
		return
	}
	line.WriteString(text)
}
