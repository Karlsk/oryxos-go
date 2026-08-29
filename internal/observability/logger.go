// Package observability provides structured logging and in-process observations.
package observability

import (
	"io"
	"log/slog"
)

// NewLogger creates a JSON logger that redacts sensitive attributes before output.
func NewLogger(w io.Writer, level slog.Leveler) *slog.Logger {
	jsonHandler := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})
	return slog.New(NewRedactingHandler(jsonHandler))
}

// NewConsoleLogger creates a human-readable logger that redacts sensitive attributes before output.
func NewConsoleLogger(w io.Writer, level slog.Leveler) *slog.Logger {
	consoleHandler := NewConsoleHandler(w, level)
	return slog.New(NewRedactingHandler(consoleHandler))
}
