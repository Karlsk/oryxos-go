# Provider Service Contract

## Core types

```go
type ModelFactory func(
	ctx context.Context,
	cfg ProviderConfig,
) (model.ToolCallingChatModel, error)

type LlmCallRecorder interface {
	Create(ctx context.Context, call *LlmCall) error
}

type ToolSchemaAdapter interface {
	ToEinoToolInfos(ctx context.Context, names []string) ([]*schema.ToolInfo, error)
}
```

The concrete Eino-ext connector imports are confined to factory construction in `internal/provider`.

## Factory registration

```go
const miniMaxOpenAIBaseURL = "https://api.minimax.cn/v1"

factories["deepseek"] = func(ctx context.Context, cfg ProviderConfig) (model.ToolCallingChatModel, error) {
	return deepseek.NewChatModel(ctx, &deepseek.ChatModelConfig{
		APIKey:      cfg.APIKey,
		Model:       cfg.Model,
		Temperature: cfg.Temperature,
	})
}

factories["minimax"] = func(ctx context.Context, cfg ProviderConfig) (model.ToolCallingChatModel, error) {
	temperature := cfg.Temperature
	return openai.NewChatModel(ctx, &openai.ChatModelConfig{
		APIKey:      cfg.APIKey,
		BaseURL:     miniMaxOpenAIBaseURL,
		Model:       cfg.Model,
		Temperature: &temperature,
	})
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
	messages []*schema.Message,
) (*schema.Message, error)
```

Preconditions:

- `ctx`, `sessionID`, `profile`, and the Profile's model binding are valid.
- The Profile Tool names can be resolved by the injected metadata-only adapter.

Behavior:

1. Look up the model by `profile.Name`.
2. Resolve ordered Tool metadata from `profile.Tools`.
3. If Tool metadata is non-empty, derive a request model with `WithTools`; never execute Tools.
4. Call `Generate` once with the supplied message slice and the Profile's model/temperature options required by the selected construction policy.
5. Measure duration and extract available token usage from `ResponseMeta.Usage`; use zero when unavailable.
6. Insert exactly one `llm_calls` record before returning.
7. Return the connector's assistant message unchanged, including Tool-call IDs and arguments.

Failure behavior:

- Provider errors are sanitized, recorded with `success=false`, and returned without retry or fallback.
- Tool metadata conversion or `WithTools` failure occurs before a Provider call and therefore does not create an LLM attempt row.
- Audit insertion failure is returned explicitly and is never hidden by an otherwise successful Provider response.
- Cancellation and deadlines flow through the caller's context.

Non-goals: Tool execution, ReAct iteration, streaming API exposure, fallback, retry, circuit breaking, cost reporting, and vendor-specific model types outside `internal/provider`.
