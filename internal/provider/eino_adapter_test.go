package provider

import (
	"context"
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
