// Package llm defines the OryxOS-owned model port and the domain values that
// cross the Agent runtime, Tool, and Provider boundaries.
package llm

import (
	"context"
	"encoding/json"
)

// ChatModel performs complete or incremental model generation.
type ChatModel interface {
	Generate(ctx context.Context, request Request) (Response, error)
	Stream(ctx context.Context, request Request) (ResponseStream, error)
}

// Request is one model generation request.
type Request struct {
	Messages []Message
	Tools    []ToolDefinition
}

// Response is one model generation result.
type Response struct {
	Message      Message
	Usage        Usage
	FinishReason string
}

// StreamEventKind identifies an incremental or terminal Stream event.
type StreamEventKind string

const (
	// StreamEventDelta carries one Provider response chunk.
	StreamEventDelta StreamEventKind = "delta"
	// StreamEventCompleted carries the one complete merged response.
	StreamEventCompleted StreamEventKind = "completed"
)

// StreamEvent carries either Delta or Response according to Kind.
type StreamEvent struct {
	Kind     StreamEventKind
	Delta    Message
	Response *Response
}

// ResponseStream is a single-consumer pull stream. A successful stream yields
// zero or more delta events, one completed event, then io.EOF. Close must be
// idempotent at the OryxOS boundary.
type ResponseStream interface {
	Recv() (StreamEvent, error)
	Close() error
}

// Role identifies the participant that produced a message.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message is the text and Tool-calling message shape used by the core runtime.
type Message struct {
	Role             Role
	Content          string
	Name             string
	ToolCalls        []ToolCall
	ToolCallID       string
	ToolName         string
	ReasoningContent string
	Extra            map[string]any
}

// FunctionCall describes one function-style Tool invocation.
type FunctionCall struct {
	Name      string
	Arguments string
}

// ToolCall is an assistant request to invoke a Tool.
type ToolCall struct {
	Index    *int
	ID       string
	Type     string
	Function FunctionCall
	Extra    map[string]any
}

// ToolDefinition is metadata exposed to a model. InputSchema contains a
// standard JSON Schema document and has no execution behavior.
type ToolDefinition struct {
	Name        string
	Description string
	InputSchema json.RawMessage
	Extra       map[string]any
}

// Usage contains the token counts made available by a Provider.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}
