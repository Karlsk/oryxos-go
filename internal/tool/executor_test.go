package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Karlsk/oryxos-go/internal/llm"
	"github.com/Karlsk/oryxos-go/internal/observability"
	"github.com/Karlsk/oryxos-go/internal/store"
)

func TestExecutorValidatesLookupAllowListAndSchemaBeforeInvoke(t *testing.T) {
	schema := []byte(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"],"additionalProperties":false}`)
	fake := &fakeTool{definition: llm.ToolDefinition{Name: "weather", InputSchema: schema}, result: "sunny"}
	recorder := &fakeInvocationRecorder{}
	executor := newExecutor(fakeLookup{"weather": {Tool: fake, Timeout: time.Second}}, recorder)

	got, err := executor.Execute(context.Background(), "session-1", []string{"weather"}, llm.ToolCall{ID: "call-1", Function: llm.FunctionCall{Name: "weather", Arguments: `{"city":"Shanghai"}`}})
	if err != nil || got != "sunny" || fake.calls != 1 || len(recorder.records) != 1 || !recorder.records[0].Success {
		t.Fatalf("Execute() = %q, %v; calls=%d records=%#v", got, err, fake.calls, recorder.records)
	}

	cases := []struct {
		name    string
		allowed []string
		call    llm.ToolCall
	}{
		{name: "not_allowed", call: llm.ToolCall{ID: "2", Function: llm.FunctionCall{Name: "weather", Arguments: `{"city":"Shanghai"}`}}},
		{name: "not_found", allowed: []string{"missing"}, call: llm.ToolCall{ID: "3", Function: llm.FunctionCall{Name: "missing", Arguments: `{}`}}},
		{name: "malformed", allowed: []string{"weather"}, call: llm.ToolCall{ID: "4", Function: llm.FunctionCall{Name: "weather", Arguments: `{`}}},
		{name: "missing_required", allowed: []string{"weather"}, call: llm.ToolCall{ID: "5", Function: llm.FunctionCall{Name: "weather", Arguments: `{}`}}},
		{name: "additional_property", allowed: []string{"weather"}, call: llm.ToolCall{ID: "6", Function: llm.FunctionCall{Name: "weather", Arguments: `{"city":"Shanghai","secret":true}`}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			beforeCalls := fake.calls
			beforeRecords := len(recorder.records)
			if _, err := executor.Execute(context.Background(), "session-1", tc.allowed, tc.call); err == nil {
				t.Fatal("Execute() error = nil, want failure")
			}
			if fake.calls != beforeCalls {
				t.Fatalf("Invoke calls = %d, want %d", fake.calls, beforeCalls)
			}
			if len(recorder.records) != beforeRecords+1 || recorder.records[len(recorder.records)-1].Success {
				t.Fatalf("records = %#v, want one failed final outcome", recorder.records)
			}
		})
	}
}

func TestExecutorTimeoutRetryAndFinalAudit(t *testing.T) {
	t.Run("timeout_each_attempt_and_three_retries", func(t *testing.T) {
		fake := &fakeTool{
			definition: llm.ToolDefinition{Name: "lookup", InputSchema: []byte(`{"type":"object"}`)},
			invoke: func(ctx context.Context, call int) (string, error) {
				if _, ok := ctx.Deadline(); !ok {
					return "", errors.New("deadline missing")
				}
				if call < 4 {
					return "partial", transientError{errors.New("temporary")}
				}
				return "done", nil
			},
		}
		recorder := &fakeInvocationRecorder{}
		executor := newExecutor(fakeLookup{"lookup": {Tool: fake, Retryable: true, Idempotent: true, Timeout: time.Second}}, recorder)
		executor.wait = func(context.Context, time.Duration) error { return nil }
		got, err := executor.Execute(context.Background(), "session-1", []string{"lookup"}, llm.ToolCall{ID: "call", Function: llm.FunctionCall{Name: "lookup", Arguments: `{}`}})
		if err != nil || got != "done" || fake.calls != 4 || len(recorder.records) != 1 || !recorder.records[0].Success {
			t.Fatalf("Execute() = %q, %v; calls=%d records=%#v", got, err, fake.calls, recorder.records)
		}
	})

	t.Run("timeout_is_propagated_and_audited", func(t *testing.T) {
		fake := &fakeTool{
			definition: llm.ToolDefinition{Name: "slow", InputSchema: []byte(`{"type":"object"}`)},
			invoke: func(ctx context.Context, _ int) (string, error) {
				<-ctx.Done()
				return "timed-out", ctx.Err()
			},
		}
		recorder := &fakeInvocationRecorder{}
		executor := newExecutor(fakeLookup{"slow": {Tool: fake, Timeout: 5 * time.Millisecond}}, recorder)
		result, err := executor.Execute(context.Background(), "session-timeout", []string{"slow"}, llm.ToolCall{Function: llm.FunctionCall{Name: "slow", Arguments: `{}`}})
		if result != "timed-out" || !errors.Is(err, context.DeadlineExceeded) || fake.calls != 1 {
			t.Fatalf("Execute() = %q, %v; calls=%d", result, err, fake.calls)
		}
		if len(recorder.records) != 1 || recorder.records[0].Success || recorder.records[0].ErrorMessage == nil {
			t.Fatalf("records = %#v, want one timeout failure", recorder.records)
		}
	})

	t.Run("non_idempotent_never_retries_and_error_is_redacted", func(t *testing.T) {
		fake := &fakeTool{
			definition: llm.ToolDefinition{Name: "notify", InputSchema: []byte(`{"type":"object"}`)},
			invoke: func(context.Context, int) (string, error) {
				return "not-sent", transientError{errors.New("api_key=very-secret-value")}
			},
		}
		recorder := &fakeInvocationRecorder{}
		executor := newExecutor(fakeLookup{"notify": {Tool: fake, Retryable: true, Idempotent: false, Timeout: time.Second}}, recorder)
		result, err := executor.Execute(context.Background(), "session-1", []string{"notify"}, llm.ToolCall{ID: "call", Function: llm.FunctionCall{Name: "notify", Arguments: `{}`}})
		if err == nil || result != "not-sent" || fake.calls != 1 || len(recorder.records) != 1 {
			t.Fatalf("Execute() = %q, %v; calls=%d records=%#v", result, err, fake.calls, recorder.records)
		}
		message := *recorder.records[0].ErrorMessage
		if strings.Contains(message, "very-secret-value") || !strings.Contains(message, "[REDACTED]") {
			t.Fatalf("recorded error = %q, want redacted", message)
		}
	})
}

func TestExecutorLogsEveryPhysicalAttemptWithCorrelationAndRedaction(t *testing.T) {
	const secret = "attempt-secret"
	fake := &fakeTool{
		definition: llm.ToolDefinition{Name: "lookup", InputSchema: []byte(`{"type":"object"}`)},
		invoke: func(_ context.Context, call int) (string, error) {
			if call < 3 {
				return "", transientError{errors.New("api_key=" + secret)}
			}
			return "done", nil
		},
	}
	var output bytes.Buffer
	logger := observability.NewLogger(&output, slog.LevelDebug)
	executor := newExecutorWithLogger(fakeLookup{"lookup": {Tool: fake, Retryable: true, Idempotent: true, Timeout: time.Second}}, &fakeInvocationRecorder{}, logger)
	executor.wait = func(context.Context, time.Duration) error { return nil }
	ctx := observability.WithCorrelation(context.Background(), observability.Correlation{
		RequestID: "request-1", ProfileName: "ops", Channel: "cli",
	})

	if _, err := executor.Execute(ctx, "session-1", []string{"lookup"}, llm.ToolCall{Function: llm.FunctionCall{Name: "lookup", Arguments: `{}`}}); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("log lines = %d, want 3: %s", len(lines), output.String())
	}
	for index, line := range lines {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("decode log %d: %v", index, err)
		}
		if event["event"] != "tool_attempt" || event["session_id"] != "session-1" || event["tool_name"] != "lookup" || event["request_id"] != "request-1" || event["profile_name"] != "ops" || event["channel"] != "cli" {
			t.Fatalf("log %d correlation = %#v", index, event)
		}
		if event["attempt"] != float64(index+1) || event["final_attempt"] != (index == 2) {
			t.Fatalf("log %d attempt fields = %#v", index, event)
		}
		if strings.Contains(line, secret) {
			t.Fatalf("log %d leaked secret: %s", index, line)
		}
	}
}

func TestExecutorJoinsToolAndRecorderFailures(t *testing.T) {
	toolFailure := errors.New("tool failed")
	recordFailure := errors.New("record failed")
	fake := &fakeTool{definition: llm.ToolDefinition{Name: "lookup", InputSchema: []byte(`{"type":"object"}`)}, invoke: func(context.Context, int) (string, error) { return "partial", toolFailure }}
	recorder := &fakeInvocationRecorder{err: recordFailure}
	executor := newExecutor(fakeLookup{"lookup": {Tool: fake, Timeout: time.Second}}, recorder)
	result, err := executor.Execute(context.Background(), "session-1", []string{"lookup"}, llm.ToolCall{Function: llm.FunctionCall{Name: "lookup", Arguments: `{}`}})
	if result != "partial" || !errors.Is(err, toolFailure) || !errors.Is(err, recordFailure) || len(recorder.records) != 1 {
		t.Fatalf("Execute() = %q, %v; records=%d", result, err, len(recorder.records))
	}
}

type fakeLookup map[string]OryxTool

func (lookup fakeLookup) lookup(name string) (OryxTool, bool) {
	value, ok := lookup[name]
	return value, ok
}

type fakeTool struct {
	mu         sync.Mutex
	definition llm.ToolDefinition
	result     string
	err        error
	invoke     func(context.Context, int) (string, error)
	calls      int
}

func (fake *fakeTool) Info(context.Context) (llm.ToolDefinition, error) { return fake.definition, nil }

func (fake *fakeTool) Invoke(ctx context.Context, _ string) (string, error) {
	fake.mu.Lock()
	fake.calls++
	call := fake.calls
	fake.mu.Unlock()
	if fake.invoke != nil {
		return fake.invoke(ctx, call)
	}
	return fake.result, fake.err
}

type fakeInvocationRecorder struct {
	records []*store.ToolInvocation
	err     error
}

func (recorder *fakeInvocationRecorder) Create(_ context.Context, invocation *store.ToolInvocation) error {
	copyOfRecord := *invocation
	if invocation.ErrorMessage != nil {
		message := *invocation.ErrorMessage
		copyOfRecord.ErrorMessage = &message
	}
	recorder.records = append(recorder.records, &copyOfRecord)
	return recorder.err
}

type transientError struct{ error }

func (transientError) Retryable() bool { return true }
