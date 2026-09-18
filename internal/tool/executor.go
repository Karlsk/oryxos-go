package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/Karlsk/oryxos-go/internal/config"
	"github.com/Karlsk/oryxos-go/internal/llm"
	"github.com/Karlsk/oryxos-go/internal/observability"
	"github.com/Karlsk/oryxos-go/internal/store"
)

const maxToolRetries = 3

// Executor validates, invokes, and records one logical Tool call.
type Executor struct {
	lookup   toolLookup
	recorder invocationRecorder
	logger   *slog.Logger
	now      func() time.Time
	wait     func(context.Context, time.Duration) error
}

// NewExecutor constructs the centralized Tool execution boundary.
func NewExecutor(lookup toolLookup, recorder invocationRecorder, logger *slog.Logger) (*Executor, error) {
	if lookup == nil {
		return nil, fmt.Errorf("create tool executor: lookup is nil")
	}
	if recorder == nil {
		return nil, fmt.Errorf("create tool executor: recorder is nil")
	}
	if logger == nil {
		return nil, fmt.Errorf("create tool executor: logger is nil")
	}
	return newExecutorWithLogger(lookup, recorder, logger), nil
}

func newExecutor(lookup toolLookup, recorder invocationRecorder) *Executor {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return newExecutorWithLogger(lookup, recorder, logger)
}

func newExecutorWithLogger(lookup toolLookup, recorder invocationRecorder, logger *slog.Logger) *Executor {
	return &Executor{lookup: lookup, recorder: recorder, logger: logger, now: time.Now, wait: waitForRetry}
}

// Execute runs one Tool call and records exactly one final logical outcome.
func (executor *Executor) Execute(ctx context.Context, sessionID string, allowedTools []string, call llm.ToolCall) (string, error) {
	if executor == nil || executor.lookup == nil || executor.recorder == nil || executor.logger == nil {
		return "", fmt.Errorf("execute tool: executor is not initialized")
	}
	if ctx == nil {
		return "", fmt.Errorf("execute tool: context is nil")
	}
	startedAt := executor.now()
	toolName := strings.TrimSpace(call.Function.Name)
	result, invokeErr := executor.execute(ctx, sessionID, toolName, allowedTools, call.Function.Arguments)
	finishedAt := executor.now()
	duration := finishedAt.Sub(startedAt).Milliseconds()
	if duration < 0 {
		duration = 0
	}

	safeInvokeErr := sanitizeToolError(invokeErr)
	invocation := &store.ToolInvocation{
		SessionID:  sessionID,
		ToolName:   toolName,
		InputJSON:  normalizedJSON(call.Function.Arguments),
		ResultJSON: marshalText(result),
		Success:    safeInvokeErr == nil,
		DurationMS: duration,
		CreatedAt:  finishedAt.UTC(),
	}
	if invocation.ToolName == "" {
		invocation.ToolName = "<unknown>"
	}
	if safeInvokeErr != nil {
		message := safeInvokeErr.Error()
		invocation.ErrorMessage = &message
	}
	recordErr := executor.recorder.Create(context.WithoutCancel(ctx), invocation)
	if recordErr != nil {
		recordErr = fmt.Errorf("record tool invocation: %w", recordErr)
	}
	if safeInvokeErr != nil && recordErr != nil {
		return result, errors.Join(safeInvokeErr, recordErr)
	}
	if safeInvokeErr != nil {
		return result, safeInvokeErr
	}
	if recordErr != nil {
		return result, recordErr
	}
	return result, nil
}

func (executor *Executor) execute(ctx context.Context, sessionID, toolName string, allowedTools []string, arguments string) (string, error) {
	if toolName == "" {
		return "", fmt.Errorf("execute tool: tool name is required")
	}
	if !containsExact(allowedTools, toolName) {
		return "", fmt.Errorf("execute tool: tool %q is not allowed by profile", toolName)
	}
	configured, ok := executor.lookup.lookup(toolName)
	if !ok {
		return "", fmt.Errorf("execute tool: tool %q is not registered", toolName)
	}
	if configured.Tool == nil {
		return "", fmt.Errorf("execute tool: tool %q has no implementation", toolName)
	}
	if configured.Timeout <= 0 {
		return "", fmt.Errorf("execute tool: tool %q timeout must be positive", toolName)
	}
	definition, err := configured.Tool.Info(ctx)
	if err != nil {
		return "", fmt.Errorf("execute tool: read %q metadata: %w", toolName, err)
	}
	if definition.Name != toolName {
		return "", fmt.Errorf("execute tool: metadata name %q does not match call %q", definition.Name, toolName)
	}
	if err := validateArguments(definition.InputSchema, arguments); err != nil {
		return "", fmt.Errorf("execute tool %q: %w", toolName, err)
	}

	var result string
	for attempt := 0; ; attempt++ {
		attemptStartedAt := executor.now()
		attemptContext, cancel := context.WithTimeout(ctx, configured.Timeout)
		result, err = configured.Tool.Invoke(attemptContext, arguments)
		cancel()
		attemptFinishedAt := executor.now()
		attemptDuration := attemptFinishedAt.Sub(attemptStartedAt).Milliseconds()
		if attemptDuration < 0 {
			attemptDuration = 0
		}
		if err == nil {
			executor.logAttempt(ctx, sessionID, toolName, attempt+1, true, true, attemptDuration, nil)
			return result, nil
		}
		retryEligible := attempt < maxToolRetries && configured.Retryable && configured.Idempotent && isRetryable(err)
		if !retryEligible {
			executor.logAttempt(ctx, sessionID, toolName, attempt+1, true, false, attemptDuration, err)
			return result, fmt.Errorf("execute tool %q: %w", toolName, err)
		}
		if waitErr := executor.wait(ctx, time.Duration(1<<attempt)*10*time.Millisecond); waitErr != nil {
			executor.logAttempt(ctx, sessionID, toolName, attempt+1, true, false, attemptDuration, err)
			return result, fmt.Errorf("execute tool %q retry wait: %w", toolName, waitErr)
		}
		executor.logAttempt(ctx, sessionID, toolName, attempt+1, false, false, attemptDuration, err)
	}
}

func (executor *Executor) logAttempt(ctx context.Context, sessionID, toolName string, attempt int, final, success bool, durationMS int64, attemptErr error) {
	correlation := observability.CorrelationFromContext(ctx)
	if correlation.SessionID == "" {
		correlation.SessionID = sessionID
	}
	logContext := observability.WithCorrelation(ctx, correlation)
	attributes := []any{
		"event", "tool_attempt",
		"tool_name", toolName,
		"attempt", attempt,
		"final_attempt", final,
		"success", success,
		"duration_ms", durationMS,
	}
	if attemptErr != nil {
		attributes = append(attributes, "error_message", config.SanitizeErrorString(attemptErr.Error()))
	}
	observability.Logger(logContext, executor.logger).InfoContext(logContext, "tool invocation attempt", attributes...)
}

func containsExact(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func isRetryable(err error) bool {
	var candidate retryableError
	return errors.As(err, &candidate) && candidate.Retryable()
}

func waitForRetry(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func normalizedJSON(raw string) string {
	if json.Valid([]byte(raw)) {
		return raw
	}
	return marshalText(raw)
}

func marshalText(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func sanitizeToolError(err error) error {
	if err == nil {
		return nil
	}
	message := config.SanitizeErrorString(err.Error())
	if message == err.Error() {
		return err
	}
	return errors.New(message)
}
