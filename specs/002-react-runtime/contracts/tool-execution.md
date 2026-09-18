# Tool Execution Contract

## OryxOS Tool port

```go
type InvokableTool interface {
	Info(ctx context.Context) (llm.ToolDefinition, error)
	Invoke(ctx context.Context, arguments string) (string, error)
}

type OryxTool struct {
	Tool       InvokableTool
	Retryable  bool
	Idempotent bool
	Timeout    time.Duration
}
```

These types contain no Eino value. `Info` is metadata-only and cannot execute a Tool.

## Executor order

For each `llm.ToolCall`, `ToolExecutor` performs:

1. Verify the call name is non-empty and appears in the selected Profile's Tool list.
2. Resolve the exact Tool from the package-private lookup seam.
3. Read its OryxOS metadata and validate the raw JSON arguments against the supported schema subset.
4. Derive a timeout child context.
5. Invoke the Tool. The concrete Tool must use the applicable shared Sandbox method immediately
   before any filesystem, command, or HTTP side effect; lesson 24 supplies that implementation.
6. Retry only when the error is explicitly retryable and execution is idempotent or has a reliable
   idempotency marker, with at most three retries and bounded exponential backoff.
7. Store exactly one final `tool_invocations` outcome with sanitized failure text.
8. Return text suitable for an `llm.RoleTool` message carrying the original Tool-call ID and name.

The ReAct loop invokes this method serially for every call in one assistant response. Executor does
not spawn goroutines or reorder calls.

## Schema validation subset

The lesson 17 validator accepts the core object schema vocabulary needed by built-in Tools:

- `type` for object, array, string, number, integer, boolean, and null;
- `properties` and `required`;
- `additionalProperties: false`;
- nested object and array `items`;
- `enum`.

Malformed JSON, type mismatches, missing required values, forbidden additional properties, or an
unsupported schema keyword fail closed before `Invoke`. Lesson 20 must keep built-in schemas inside
this supported contract or explicitly extend it through an approved design change.

## Error and audit precedence

- Tool succeeds and recording succeeds: return the Tool result.
- Tool fails and recording succeeds: return the sanitized Tool failure after retaining the result
  text used for the matching Tool message.
- Tool succeeds and recording fails: return the recording failure; do not claim a fully handled Tool
  call.
- Tool and recording both fail: return an error preserving both sanitized causes.

Physical retry attempts are structured-log events. Only the final logical outcome is a database row.
