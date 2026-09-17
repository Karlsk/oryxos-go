# Provider Service Contract

## Core types

```go
type ModelFactory func(
	ctx context.Context,
	cfg ProviderConfig,
) (llm.ChatModel, error)

type LlmCallRecorder interface {
	Create(ctx context.Context, call *LlmCall) error
}

type ToolSchemaResolver interface {
	Resolve(ctx context.Context, names []string) ([]llm.ToolDefinition, error)
}
```

Eino core and Eino-ext imports are confined to adapters and factory construction in `internal/provider`. Runtime and Tool packages use only `internal/llm`.

The OryxOS model port exposes both complete and incremental calls:

```go
type ChatModel interface {
	Generate(context.Context, Request) (Response, error)
	Stream(context.Context, Request) (ResponseStream, error)
}

type ResponseStream interface {
	Recv() (StreamEvent, error)
	Close() error
}
```

`Recv` yields zero or more delta events, exactly one completed event containing the full merged `Response`, and then `io.EOF`. Receive failures are returned as errors, not encoded as events. The stream is single-consumer and `Close` is idempotent.

## Factory registration

```go
const miniMaxOpenAIBaseURL = "https://api.minimax.cn/v1"

factories["deepseek"] = func(ctx context.Context, cfg ProviderConfig) (llm.ChatModel, error) {
	connector, err := deepseek.NewChatModel(ctx, &deepseek.ChatModelConfig{
		APIKey:      cfg.APIKey,
		Model:       cfg.Model,
		Temperature: cfg.Temperature,
	})
	return newEinoChatModelAdapter(connector, err)
}

factories["minimax"] = func(ctx context.Context, cfg ProviderConfig) (llm.ChatModel, error) {
	temperature := cfg.Temperature
	connector, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		APIKey:      cfg.APIKey,
		BaseURL:     miniMaxOpenAIBaseURL,
		Model:       cfg.Model,
		Temperature: &temperature,
	})
	return newEinoChatModelAdapter(connector, err)
}
```

These fields match the locally inspected stable connector versions DeepSeek `v0.1.7` and OpenAI `v0.1.13`. The DeepSeek connector uses its official default endpoint when `BaseURL` is empty, so the DeepSeek factory intentionally omits that field; only the MiniMax/OpenAI adapter needs a fixed URL.

## Registry behavior

- Factories are addressed by Provider name.
- Constructed models are addressed by Profile name.
- Registering a duplicate factory, Profile, or model binding returns an error and never overwrites.
- A missing Profile model binding returns an explicit error; no fallback is attempted.
- Model constructors receive a fresh merged `ProviderConfig` for each Profile.
- `ProviderConfig` contains no endpoint. Each explicit factory owns endpoint policy: DeepSeek deliberately uses the native connector default, while MiniMax sets the compatibility endpoint. Users cannot override either through startup or Profile YAML.

## `ProviderService.Chat`

```go
func (service *ProviderService) Chat(
	ctx context.Context,
	sessionID string,
	profile *profile.Profile,
	messages []llm.Message,
) (llm.Response, error)
```

Preconditions:

- `ctx`, `sessionID`, `profile`, and the Profile's model binding are valid.
- The Profile Tool names can be resolved by the injected metadata-only adapter.

Behavior:

1. Look up the model by `profile.Name`.
2. Resolve ordered Tool metadata from `profile.Tools`.
3. Pass OryxOS messages and ordered Tool definitions to the Profile-bound `llm.ChatModel`; never execute Tools.
4. The Eino adapter converts the request, binds `schema.ToolInfo` when present, and calls the connector's `Generate` exactly once.
5. Measure duration and extract usage from the OryxOS response; use zero when unavailable.
6. Insert exactly one `llm_calls` record before returning.
7. Return the OryxOS response with assistant content, reasoning content, Tool-call IDs/arguments, usage, and finish reason preserved.

Failure behavior:

- Provider errors are sanitized, recorded with `success=false`, and returned without retry or fallback.
- Tool metadata resolution or Eino schema binding failure occurs before a Provider call and therefore does not create an LLM attempt row.
- Audit insertion failure is returned explicitly and is never hidden by an otherwise successful Provider response.
- Cancellation and deadlines flow through the caller's context.

## `ProviderService.ChatStream`

```go
func (service *ProviderService) ChatStream(
	ctx context.Context,
	sessionID string,
	profile *profile.Profile,
	messages []llm.Message,
) (llm.ResponseStream, error)
```

It shares Chat's validation, Profile lookup, ordered Tool metadata resolution, and no-execution rule. The Eino adapter calls connector `Stream` once, converts each chunk to an OryxOS delta, merges all chunks through `schema.ConcatMessages`, returns one completed event, and then returns `io.EOF`.

The service wrapper persists exactly once before exposing a terminal outcome:

- completed response: one successful row using terminal usage;
- initialization or receive failure, premature EOF, or close before completion: one failed row with a sanitized reason;
- close after completion/failure and repeated close: no additional row;
- audit insertion failure: return that failure instead of reporting the terminal outcome as fully handled.

The audit insert uses `context.WithoutCancel` so a cancellation that ends a connector stream cannot prevent its required call record.

Non-goals: Tool execution, ReAct iteration, ReAct/CLI streaming, SSE/WebSocket transport, fallback, retry, circuit breaking, cost reporting, and any Eino type outside `internal/provider`.
