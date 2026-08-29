package observability

import (
	"context"
	"log/slog"
)

// Correlation contains identifiers that safely connect related application events.
type Correlation struct {
	RequestID   string
	SessionID   string
	ProfileName string
	Channel     string
	ScheduleID  string
}

type correlationContextKey struct{}

// WithCorrelation returns a child context with immutable correlation values.
func WithCorrelation(ctx context.Context, correlation Correlation) context.Context {
	return context.WithValue(ctx, correlationContextKey{}, correlation)
}

// CorrelationFromContext returns correlation values associated with ctx, if any.
func CorrelationFromContext(ctx context.Context) Correlation {
	if correlation, ok := ctx.Value(correlationContextKey{}).(Correlation); ok {
		return correlation
	}
	return Correlation{}
}

// Logger returns base enriched with the non-empty correlation values in ctx.
func Logger(ctx context.Context, base *slog.Logger) *slog.Logger {
	correlation := CorrelationFromContext(ctx)
	attrs := make([]any, 0, 10)
	if correlation.RequestID != "" {
		attrs = append(attrs, "request_id", correlation.RequestID)
	}
	if correlation.SessionID != "" {
		attrs = append(attrs, "session_id", correlation.SessionID)
	}
	if correlation.ProfileName != "" {
		attrs = append(attrs, "profile_name", correlation.ProfileName)
	}
	if correlation.Channel != "" {
		attrs = append(attrs, "channel", correlation.Channel)
	}
	if correlation.ScheduleID != "" {
		attrs = append(attrs, "schedule_id", correlation.ScheduleID)
	}
	return base.With(attrs...)
}
