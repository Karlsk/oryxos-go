package tool

import (
	"context"
	"time"

	"github.com/Karlsk/oryxos-go/internal/llm"
	"github.com/Karlsk/oryxos-go/internal/store"
)

// InvokableTool is the OryxOS-owned executable Tool boundary.
type InvokableTool interface {
	Info(ctx context.Context) (llm.ToolDefinition, error)
	Invoke(ctx context.Context, arguments string) (string, error)
}

// OryxTool combines execution behavior with runtime policy metadata.
type OryxTool struct {
	Tool       InvokableTool
	Retryable  bool
	Idempotent bool
	Timeout    time.Duration
}

type toolLookup interface {
	lookup(name string) (OryxTool, bool)
}

type invocationRecorder interface {
	Create(ctx context.Context, invocation *store.ToolInvocation) error
}

type retryableError interface {
	Retryable() bool
}
