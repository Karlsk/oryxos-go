package provider

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/Karlsk/oryxos-go/internal/llm"
	"github.com/cloudwego/eino/schema"
)

func TestEinoAdapterPreservesMessagesToolsAndResponseMetadata(t *testing.T) {
	response := &schema.Message{
		Role:             schema.Assistant,
		Content:          "use the tool",
		Name:             "assistant-name",
		ReasoningContent: "reasoning",
		ToolCalls: []schema.ToolCall{{
			ID:   "call-2",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "http_get",
				Arguments: `{"url":"https://example.com"}`,
			},
		}},
		ResponseMeta: &schema.ResponseMeta{
			FinishReason: "tool_calls",
			Usage: &schema.TokenUsage{
				PromptTokens:     11,
				CompletionTokens: 7,
				TotalTokens:      18,
			},
		},
	}
	connector, state := newFakeModel(response, nil)
	adapter, err := newEinoChatModelAdapter(connector)
	if err != nil {
		t.Fatalf("newEinoChatModelAdapter() error = %v", err)
	}

	got, err := adapter.Generate(context.Background(), llm.Request{
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: "system"},
			{Role: llm.RoleUser, Content: "user", Name: "user-name"},
			{
				Role:    llm.RoleAssistant,
				Content: "calling",
				ToolCalls: []llm.ToolCall{{
					ID: "call-1", Type: "function",
					Function: llm.FunctionCall{Name: "http_get", Arguments: `{"url":"https://example.com"}`},
				}},
			},
			{Role: llm.RoleTool, Content: "ok", ToolCallID: "call-1", ToolName: "http_get"},
		},
		Tools: []llm.ToolDefinition{{
			Name:        "http_get",
			Description: "fetch a URL",
			InputSchema: []byte(`{"type":"object","properties":{"url":{"type":"string"}},"required":["url"]}`),
		}},
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if state.generateCalls != 1 || state.withToolsCalls != 1 || len(state.boundTools) != 1 {
		t.Fatalf("connector calls = generate:%d withTools:%d tools:%d", state.generateCalls, state.withToolsCalls, len(state.boundTools))
	}
	if len(state.inputs) != 1 || len(state.inputs[0]) != 4 {
		t.Fatalf("connector messages = %#v", state.inputs)
	}
	input := state.inputs[0]
	if input[0].Role != schema.System || input[1].Role != schema.User || input[1].Name != "user-name" {
		t.Fatalf("system/user conversion = %#v", input[:2])
	}
	if len(input[2].ToolCalls) != 1 || input[2].ToolCalls[0].ID != "call-1" || input[2].ToolCalls[0].Function.Arguments != `{"url":"https://example.com"}` {
		t.Fatalf("assistant Tool call conversion = %#v", input[2])
	}
	if input[3].Role != schema.Tool || input[3].ToolCallID != "call-1" || input[3].ToolName != "http_get" {
		t.Fatalf("Tool result conversion = %#v", input[3])
	}
	toolSchema, err := state.boundTools[0].ParamsOneOf.ToJSONSchema()
	if err != nil || toolSchema == nil || toolSchema.Type != "object" {
		t.Fatalf("Tool schema = %#v, %v", toolSchema, err)
	}
	if got.Message.Role != llm.RoleAssistant || got.Message.Content != "use the tool" || got.Message.Name != "assistant-name" || got.Message.ReasoningContent != "reasoning" {
		t.Fatalf("response message = %#v", got.Message)
	}
	if len(got.Message.ToolCalls) != 1 || got.Message.ToolCalls[0].ID != "call-2" || got.Message.ToolCalls[0].Function.Name != "http_get" {
		t.Fatalf("response Tool calls = %#v", got.Message.ToolCalls)
	}
	if got.FinishReason != "tool_calls" || got.Usage.PromptTokens != 11 || got.Usage.CompletionTokens != 7 || got.Usage.TotalTokens != 18 {
		t.Fatalf("response metadata = %#v", got)
	}
}

func TestEinoAdapterStreamPreservesDeltasAndCompletes(t *testing.T) {
	toolIndex := 0
	chunks := []*schema.Message{
		{
			Role:    schema.Assistant,
			Content: "hel",
			ToolCalls: []schema.ToolCall{{
				Index:    &toolIndex,
				ID:       "call-1",
				Type:     "function",
				Function: schema.FunctionCall{Name: "echo", Arguments: `{"text":"`},
			}},
		},
		{
			Role:    schema.Assistant,
			Content: "lo",
			ToolCalls: []schema.ToolCall{{
				Index:    &toolIndex,
				Function: schema.FunctionCall{Arguments: `hello"}`},
			}},
			ResponseMeta: &schema.ResponseMeta{
				FinishReason: "tool_calls",
				Usage:        &schema.TokenUsage{PromptTokens: 4, CompletionTokens: 5, TotalTokens: 9},
			},
		},
	}
	connector, state := newFakeModel(nil, nil)
	state.stream = schema.StreamReaderFromArray(chunks)
	adapter, err := newEinoChatModelAdapter(connector)
	if err != nil {
		t.Fatalf("newEinoChatModelAdapter() error = %v", err)
	}

	stream, err := adapter.Stream(context.Background(), llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hello"}},
		Tools:    []llm.ToolDefinition{{Name: "echo", InputSchema: []byte(`{"type":"object"}`)}},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	defer stream.Close()

	first, err := stream.Recv()
	if err != nil || first.Kind != llm.StreamEventDelta || first.Delta.Content != "hel" || first.Response != nil {
		t.Fatalf("first Recv() = %#v, %v", first, err)
	}
	second, err := stream.Recv()
	if err != nil || second.Kind != llm.StreamEventDelta || second.Delta.Content != "lo" || second.Response != nil {
		t.Fatalf("second Recv() = %#v, %v", second, err)
	}
	completed, err := stream.Recv()
	if err != nil || completed.Kind != llm.StreamEventCompleted || completed.Response == nil {
		t.Fatalf("completed Recv() = %#v, %v", completed, err)
	}
	response := completed.Response
	if response.Message.Content != "hello" || len(response.Message.ToolCalls) != 1 || response.Message.ToolCalls[0].ID != "call-1" || response.Message.ToolCalls[0].Function.Arguments != `{"text":"hello"}` {
		t.Fatalf("completed response = %#v", response)
	}
	if response.FinishReason != "tool_calls" || response.Usage.TotalTokens != 9 {
		t.Fatalf("completed metadata = %#v", response)
	}
	if _, err := stream.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("Recv() after completion error = %v, want io.EOF", err)
	}
	if state.streamCalls != 1 || state.generateCalls != 0 || state.withToolsCalls != 1 {
		t.Fatalf("connector calls = stream:%d generate:%d withTools:%d", state.streamCalls, state.generateCalls, state.withToolsCalls)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func TestEinoAdapterStreamRejectsInvalidAndIncompleteStreams(t *testing.T) {
	t.Run("initialization_error", func(t *testing.T) {
		upstreamErr := errors.New("stream initialization failed")
		connector, state := newFakeModel(nil, nil)
		state.streamErr = upstreamErr
		adapter, _ := newEinoChatModelAdapter(connector)
		if _, err := adapter.Stream(context.Background(), llm.Request{}); !errors.Is(err, upstreamErr) {
			t.Fatalf("Stream() error = %v, want upstream error", err)
		}
	})

	t.Run("nil_reader", func(t *testing.T) {
		connector, _ := newFakeModel(nil, nil)
		adapter, _ := newEinoChatModelAdapter(connector)
		if _, err := adapter.Stream(context.Background(), llm.Request{}); err == nil {
			t.Fatal("Stream() error = nil")
		}
	})

	t.Run("invalid_input_before_connector", func(t *testing.T) {
		connector, state := newFakeModel(nil, nil)
		adapter, _ := newEinoChatModelAdapter(connector)
		if _, err := adapter.Stream(context.Background(), llm.Request{Messages: []llm.Message{{Role: "unknown"}}}); err == nil {
			t.Fatal("Stream() error = nil")
		}
		if state.streamCalls != 0 {
			t.Fatalf("stream calls = %d, want zero", state.streamCalls)
		}
	})

	t.Run("empty_stream", func(t *testing.T) {
		connector, state := newFakeModel(nil, nil)
		state.stream = schema.StreamReaderFromArray([]*schema.Message{})
		adapter, _ := newEinoChatModelAdapter(connector)
		stream, err := adapter.Stream(context.Background(), llm.Request{})
		if err != nil {
			t.Fatalf("Stream() error = %v", err)
		}
		defer stream.Close()
		if _, err := stream.Recv(); err == nil || errors.Is(err, io.EOF) {
			t.Fatalf("Recv() error = %v, want incomplete stream failure", err)
		}
	})

	t.Run("nil_chunk", func(t *testing.T) {
		connector, state := newFakeModel(nil, nil)
		state.stream = schema.StreamReaderFromArray([]*schema.Message{nil})
		adapter, _ := newEinoChatModelAdapter(connector)
		stream, err := adapter.Stream(context.Background(), llm.Request{})
		if err != nil {
			t.Fatalf("Stream() error = %v", err)
		}
		defer stream.Close()
		if _, err := stream.Recv(); err == nil {
			t.Fatal("Recv() error = nil")
		}
	})

	t.Run("connector_receive_error", func(t *testing.T) {
		reader, writer := schema.Pipe[*schema.Message](1)
		upstreamErr := errors.New("upstream stream failed")
		writer.Send(nil, upstreamErr)
		writer.Close()
		connector, state := newFakeModel(nil, nil)
		state.stream = reader
		adapter, _ := newEinoChatModelAdapter(connector)
		stream, err := adapter.Stream(context.Background(), llm.Request{})
		if err != nil {
			t.Fatalf("Stream() error = %v", err)
		}
		defer stream.Close()
		if _, err := stream.Recv(); !errors.Is(err, upstreamErr) {
			t.Fatalf("Recv() error = %v, want upstream failure", err)
		}
	})

	t.Run("early_close_is_idempotent", func(t *testing.T) {
		connector, state := newFakeModel(nil, nil)
		state.stream = schema.StreamReaderFromArray([]*schema.Message{{Role: schema.Assistant, Content: "unused"}})
		adapter, _ := newEinoChatModelAdapter(connector)
		stream, err := adapter.Stream(context.Background(), llm.Request{})
		if err != nil {
			t.Fatalf("Stream() error = %v", err)
		}
		if err := stream.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
		if err := stream.Close(); err != nil {
			t.Fatalf("second Close() error = %v", err)
		}
		if _, err := stream.Recv(); !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("Recv() after Close error = %v, want io.ErrClosedPipe", err)
		}
	})
}

func TestEinoAdapterRejectsInvalidOryxOSInputBeforeConnectorCall(t *testing.T) {
	connector, state := newFakeModel(&schema.Message{Role: schema.Assistant}, nil)
	adapter, err := newEinoChatModelAdapter(connector)
	if err != nil {
		t.Fatalf("newEinoChatModelAdapter() error = %v", err)
	}

	cases := []struct {
		name    string
		request llm.Request
	}{
		{name: "unknown_role", request: llm.Request{Messages: []llm.Message{{Role: "unknown"}}}},
		{name: "invalid_tool_schema", request: llm.Request{Tools: []llm.ToolDefinition{{Name: "bad", InputSchema: []byte(`{"type":`)}}}},
		{name: "empty_tool_name", request: llm.Request{Tools: []llm.ToolDefinition{{Description: "missing name"}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := adapter.Generate(context.Background(), tc.request); err == nil {
				t.Fatal("Generate() error = nil")
			}
		})
	}
	if state.generateCalls != 0 || state.withToolsCalls != 0 {
		t.Fatalf("connector calls = generate:%d withTools:%d, want zero", state.generateCalls, state.withToolsCalls)
	}
}

func TestEinoAdapterRejectsNilConnector(t *testing.T) {
	if _, err := newEinoChatModelAdapter(nil); err == nil {
		t.Fatal("newEinoChatModelAdapter(nil) error = nil")
	}
}
