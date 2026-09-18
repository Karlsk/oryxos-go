package runtime

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Karlsk/oryxos-go/internal/bootstrap"
	"github.com/Karlsk/oryxos-go/internal/llm"
	"github.com/Karlsk/oryxos-go/internal/profile"
	"github.com/Karlsk/oryxos-go/internal/session"
	"github.com/Karlsk/oryxos-go/internal/skill"
)

func TestReActLoopCompletesFinalAndToolAssistedResponses(t *testing.T) {
	t.Run("final_response", func(t *testing.T) {
		model := &scriptedChat{responses: []llm.Response{{Message: llm.Message{Role: llm.RoleAssistant, Content: "hello"}}}}
		loop := newTestLoop(t, model, &scriptedExecutor{})
		s := session.New("session-1")
		content, err := loop.Run(context.Background(), s, "hi", testProfile(3))
		if err != nil || content != "hello" || model.calls != 1 {
			t.Fatalf("Run() = %q, %v; calls=%d", content, err, model.calls)
		}
		messages := s.Messages()
		if len(messages) != 2 || messages[0].Role != llm.RoleUser || messages[1].Role != llm.RoleAssistant {
			t.Fatalf("messages = %#v", messages)
		}
	})

	t.Run("multiple_tools_are_serial_and_correlated", func(t *testing.T) {
		assistant := llm.Message{Role: llm.RoleAssistant, Content: "checking", ToolCalls: []llm.ToolCall{
			{ID: "call-1", Function: llm.FunctionCall{Name: "first", Arguments: `{}`}},
			{ID: "call-2", Function: llm.FunctionCall{Name: "second", Arguments: `{}`}},
		}}
		model := &scriptedChat{responses: []llm.Response{{Message: assistant}, {Message: llm.Message{Role: llm.RoleAssistant, Content: "done"}}}}
		executor := &scriptedExecutor{results: map[string]string{"first": "one", "second": "two"}}
		loop := newTestLoop(t, model, executor)
		s := session.New("session-1")
		content, err := loop.Run(context.Background(), s, "work", testProfile(3))
		if err != nil || content != "done" || !reflect.DeepEqual(executor.order, []string{"first", "second"}) {
			t.Fatalf("Run() = %q, %v; order=%v", content, err, executor.order)
		}
		messages := s.Messages()
		if len(messages) != 5 || !reflect.DeepEqual(messages[1], assistant) || messages[2].Role != llm.RoleTool || messages[2].ToolCallID != "call-1" || messages[2].ToolName != "first" || messages[3].ToolCallID != "call-2" {
			t.Fatalf("messages = %#v, want complete assistant and correlated Tool messages", messages)
		}
	})
}

func TestReActLoopPropagatesCancellationAndToolFailure(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	model := &scriptedChat{err: context.Canceled}
	loop := newTestLoop(t, model, &scriptedExecutor{})
	_, err := loop.Run(canceled, session.New("session-1"), "hi", testProfile(2))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}

	toolFailure := errors.New("tool failure")
	model = &scriptedChat{responses: []llm.Response{{Message: llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "call-1", Function: llm.FunctionCall{Name: "bad", Arguments: `{}`}}}}}}}
	executor := &scriptedExecutor{results: map[string]string{"bad": "partial"}, err: toolFailure}
	loop = newTestLoop(t, model, executor)
	s := session.New("session-2")
	_, err = loop.Run(context.Background(), s, "hi", testProfile(2))
	if !errors.Is(err, toolFailure) {
		t.Fatalf("Run() error = %v, want tool failure", err)
	}
	messages := s.Messages()
	if len(messages) != 3 || messages[2].Role != llm.RoleTool || messages[2].Content != "partial" {
		t.Fatalf("messages = %#v, want failure result retained", messages)
	}
}

func TestReActLoopPreservesSuccessfulResponseButSkipsToolsWhenAuditFails(t *testing.T) {
	auditFailure := errors.New("persist llm call: database is read-only")
	assistant := llm.Message{
		Role:    llm.RoleAssistant,
		Content: "I need to inspect the file",
		ToolCalls: []llm.ToolCall{{
			ID:       "call-1",
			Function: llm.FunctionCall{Name: "read_file", Arguments: `{"path":"README.md"}`},
		}},
	}
	model := &scriptedChat{
		responses: []llm.Response{{Message: assistant}},
		err:       modelResponseAuditError{cause: auditFailure},
	}
	executor := &scriptedExecutor{results: map[string]string{"read_file": "secret"}}
	s := session.New("session-1")

	content, err := newTestLoop(t, model, executor).Run(context.Background(), s, "inspect", testProfile(2))

	if content != "" || !errors.Is(err, auditFailure) {
		t.Fatalf("Run() = %q, %v; want empty content and audit failure", content, err)
	}
	if len(executor.order) != 0 {
		t.Fatalf("tool execution order = %v, want no execution", executor.order)
	}
	if got := s.Messages(); len(got) != 2 || got[0].Role != llm.RoleUser || !reflect.DeepEqual(got[1], assistant) {
		t.Fatalf("messages = %#v, want user plus preserved assistant response", got)
	}
}

func TestReActLoopDoesNotPreserveResponseForOrdinaryModelError(t *testing.T) {
	modelFailure := errors.New("provider failed")
	model := &scriptedChat{
		responses: []llm.Response{{Message: llm.Message{Role: llm.RoleAssistant, Content: "untrusted partial response"}}},
		err:       modelFailure,
	}
	s := session.New("session-1")

	_, err := newTestLoop(t, model, &scriptedExecutor{}).Run(context.Background(), s, "hello", testProfile(2))

	if !errors.Is(err, modelFailure) {
		t.Fatalf("Run() error = %v, want model failure", err)
	}
	if got := s.Messages(); len(got) != 1 || got[0].Role != llm.RoleUser {
		t.Fatalf("messages = %#v, want only the user message", got)
	}
}

func TestReActLoopStopsAfterExactMaximumIterations(t *testing.T) {
	response := llm.Response{Message: llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "call", Function: llm.FunctionCall{Name: "again", Arguments: `{}`}}}}}
	model := &scriptedChat{responses: []llm.Response{response, response, response}}
	loop := newTestLoop(t, model, &scriptedExecutor{results: map[string]string{"again": "ok"}})
	_, err := loop.Run(context.Background(), session.New("session-1"), "loop", testProfile(3))
	if !errors.Is(err, ErrMaxIterations) || model.calls != 3 {
		t.Fatalf("Run() error = %v; calls=%d, want ErrMaxIterations after 3", err, model.calls)
	}
}

func TestReActLoopSecondModelRequestContainsCurrentUserOnce(t *testing.T) {
	builder, err := NewPromptBuilder(
		"rules",
		map[string]bootstrap.Snapshot{"default": {}},
		map[string]skill.Snapshot{"default": {}},
		staticMemory(""),
		map[string]int{"default": 10_000},
		func() time.Time { return time.Date(2026, 9, 18, 9, 30, 0, 0, time.UTC) },
	)
	if err != nil {
		t.Fatal(err)
	}
	model := &scriptedChat{responses: []llm.Response{
		{Message: llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "call-1", Function: llm.FunctionCall{Name: "first", Arguments: `{}`}}}}},
		{Message: llm.Message{Role: llm.RoleAssistant, Content: "done"}},
	}}
	loop, err := NewReActLoop(model, &scriptedExecutor{results: map[string]string{"first": "observed"}}, builder)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := loop.Run(context.Background(), session.New("session-1"), "current", testProfile(3)); err != nil {
		t.Fatal(err)
	}
	if len(model.requests) != 2 {
		t.Fatalf("model requests = %d, want 2", len(model.requests))
	}
	currentCount := 0
	for _, message := range model.requests[1] {
		if message.Role == llm.RoleUser && message.Content == "current" {
			currentCount++
		}
	}
	if currentCount != 1 {
		t.Fatalf("second request current user count = %d, want 1; messages=%#v", currentCount, model.requests[1])
	}
}

func newTestLoop(t *testing.T, chat chatService, executor toolExecutor) *ReActLoop {
	t.Helper()
	loop, err := NewReActLoop(chat, executor, sessionPrompt{})
	if err != nil {
		t.Fatalf("NewReActLoop() error = %v", err)
	}
	return loop
}

func testProfile(iterations int) *profile.Profile {
	return &profile.Profile{Name: "default", Tools: []string{"first", "second", "bad", "again"}, Settings: profile.SettingsConfig{MaxIterations: iterations, MaxHistoryTurns: 20}}
}

type scriptedChat struct {
	responses []llm.Response
	err       error
	calls     int
	requests  [][]llm.Message
}

func (chat *scriptedChat) Chat(_ context.Context, _ string, _ *profile.Profile, messages []llm.Message) (llm.Response, error) {
	chat.calls++
	chat.requests = append(chat.requests, append([]llm.Message(nil), messages...))
	if chat.err != nil {
		if len(chat.responses) > 0 {
			return chat.responses[chat.calls-1], chat.err
		}
		return llm.Response{}, chat.err
	}
	return chat.responses[chat.calls-1], nil
}

type modelResponseAuditError struct {
	cause error
}

func (err modelResponseAuditError) Error() string { return err.cause.Error() }

func (err modelResponseAuditError) Unwrap() error { return err.cause }

func (modelResponseAuditError) ModelResponseAvailable() bool { return true }

type scriptedExecutor struct {
	results map[string]string
	err     error
	order   []string
}

func (executor *scriptedExecutor) Execute(_ context.Context, _ string, _ []string, call llm.ToolCall) (string, error) {
	executor.order = append(executor.order, call.Function.Name)
	return executor.results[call.Function.Name], executor.err
}

type sessionPrompt struct{}

func (sessionPrompt) Build(_ context.Context, current *session.Session, _ *profile.Profile, _ string) ([]llm.Message, error) {
	return current.Messages(), nil
}
