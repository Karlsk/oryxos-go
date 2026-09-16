# Phase 0 Research: LLM Provider Integration

## Decision 1: Lock the latest stable dependency releases

**Decision**: Pin these direct dependencies when implementation begins:

| Module | Locked version | Purpose |
|---|---:|---|
| `github.com/cloudwego/eino` | `v0.9.19` | Provider-internal connector and schema adaptation |
| `github.com/cloudwego/eino-ext/components/model/deepseek` | `v0.1.7` | Native DeepSeek connector |
| `github.com/cloudwego/eino-ext/components/model/openai` | `v0.1.13` | MiniMax through its OpenAI-compatible endpoint |
| `gorm.io/gorm` | `v1.31.2` | `LlmCall` persistence |
| `github.com/glebarez/sqlite` | `v1.11.0` | Pure-Go SQLite GORM dialector |

**Rationale**: On 2026-09-14, `go list -m <module>@latest` resolved these as the latest non-prerelease module versions. The user explicitly selected the latest stable policy.

**Local API verification**:

- Eino core `v0.9.19` defines `model.ToolCallingChatModel.WithTools([]*schema.ToolInfo)` and `Generate(context.Context, []*schema.Message, ...model.Option)` through its backward-compatible `BaseChatModel` alias.
- DeepSeek `v0.1.7` exposes `deepseek.NewChatModel(context.Context, *deepseek.ChatModelConfig)`; `Temperature` is `float32`.
- OpenAI `v0.1.13` exposes `openai.NewChatModel(context.Context, *openai.ChatModelConfig)`; `BaseURL` is a string and `Temperature` is `*float32`.
- Both constructors return concrete models implementing `model.ToolCallingChatModel` and both connectors expose immutable `WithTools` binding.
- `schema.ResponseMeta.Usage` contains `PromptTokens`, `CompletionTokens`, and `TotalTokens` integer fields.
- `glebarez/sqlite v1.11.0` exposes `sqlite.Open(dsn) gorm.Dialector` and imports `modernc.org/sqlite`, not `mattn/go-sqlite3`; GORM `v1.31.2` exposes `gorm.Open`, `DB.Exec`, and `DB.Create`.
- MiniMax's current China-platform OpenAI-compatible documentation uses `https://api.minimax.cn/v1`, lists `MiniMax-M3` as the latest supported model, and requires complete assistant responses to be preserved in multi-turn Tool Calling: <https://platform.minimax.cn/docs/api-reference/text-openai-api>.

Both connector modules declare Eino `v0.7.13` as their minimum. A local source diff from Eino `v0.7.13` to `v0.9.19` confirmed that the production interfaces used here remain source-compatible: `BaseChatModel` is retained as an alias, `ToolCallingChatModel.WithTools` is unchanged, and the message/stream helpers used by the connectors remain available. A complete temporary binary download was interrupted by repeated external proxy timeouts, so implementation must re-run module resolution and the full build gates after adding the pins.

**Alternatives considered**:

- Pin Eino core to the connectors' older `v0.7.13`: rejected because the selected policy is latest stable and the used interface is source-compatible.
- Use a native MiniMax connector: rejected because lesson 16 deliberately validates the OpenAI-compatible path.
- Import `modernc.org/sqlite` directly: rejected because the application uses the required `glebarez/sqlite` GORM dialector; `modernc.org/sqlite` remains transitive unless production code directly needs it.

## Decision 2: Separate process connection declarations from Profile choices

**Decision**: Extend the existing process YAML loader with `providers`, each containing only `name` and `api_key`. Profile YAML contains `provider.name`, `provider.model`, and optional `provider.temperature` only. Each explicit Provider factory owns its connector, protocol adaptation, and endpoint policy.

**Rationale**: Credentials belong to the process boundary and can be expanded from environment placeholders once. Profiles remain portable Agent choices and do not duplicate secrets.

**Validation policy**:

- Unknown process fields, duplicate Provider names, unsupported names, empty credentials, and missing environment variables are fatal configuration errors.
- Only `deepseek` and `minimax` are accepted.
- The DeepSeek factory omits `BaseURL`, allowing the native connector to use its documented official default `https://api.deepseek.com/`. The MiniMax factory sets the China-platform OpenAI-compatible `https://api.minimax.cn/v1` on the OpenAI connector. Neither endpoint is accepted from process or Profile YAML.
- A future custom/self-hosted OpenAI-compatible service requires a separate explicit Provider design; it must not overload the `minimax` identity.
- Profile files are decoded strictly and independently. Invalid YAML, missing name/model/provider, undeclared Provider references, and duplicate Profile names are diagnostics for that Profile; other valid Profiles still load.
- Profile files are processed in lexical path order so duplicate-name handling is deterministic; the first valid Profile remains registered and later duplicates are skipped.

**Alternatives considered**:

- Keep API keys in every Profile: rejected because it duplicates credentials and risks cross-Profile leakage.
- Fail the entire process for one malformed Profile: rejected by the approved lesson-16 isolation behavior.
- Add `global.yaml` to `.oryxos init`: rejected because the core workspace contract is fixed at five directories and six initialization files.

## Decision 3: Own the runtime port and isolate Eino in Provider

**Decision**: Define `ChatModel`, `Request`, `Response`, `Message`, `ToolDefinition`, `ToolCall`, and `Usage` in `internal/llm`. The core-stage message contract preserves text roles, names, reasoning content, complete assistant Tool calls, Tool-result correlation IDs, JSON arguments, JSON Schema, token usage, and finish reason. An adapter in `internal/provider` performs all Eino conversions.

**Rationale**: Eino remains valuable for connector protocol maintenance, but its 0.x API and broad schema types should not determine Runtime, Tool, Handler, or Scheduler contracts. The narrow port makes those packages independently testable and confines a future connector replacement to Provider.

**Alternatives considered**:

- Use Eino core as the Runtime boundary: rejected because `schema.Message` and `schema.ToolInfo` would spread into later ReAct and Tool lessons.
- Reimplement vendor HTTP protocols: rejected because OryxOS would inherit authentication, Function Calling, error-shape, and compatibility maintenance without adding core Agent OS value.

## Decision 4: Register factories by vendor and instances by Profile

**Decision**: `ProviderRegistry` owns two maps:

- `factories[provider.name]ModelFactory`
- `models[profile.name]llm.ChatModel`

For each valid Profile, merge its choice with the declared connection and call the matching factory once. Reject duplicates instead of overwriting.

**Rationale**: Explicit factories make the supported vendor set and endpoint ownership reviewable, while per-Profile instances prevent model, temperature, or future Tool binding from crossing Profile boundaries.

**Alternatives considered**:

- Cache one model by Provider name: rejected because two Profiles using the same vendor can select different models and temperatures.
- Dynamically discover connectors: rejected because the core supports exactly two Providers and must not silently route.

## Decision 5: Bind Tool schemas without executing Tools

**Decision**: `ProviderService` resolves ordered Profile Tool names to OryxOS `[]llm.ToolDefinition` and calls an OryxOS `llm.ChatModel`. The Provider-internal Eino adapter converts definitions to `[]*schema.ToolInfo`, calls `WithTools` only when definitions are present, invokes `Generate`, and converts the complete response back to `llm.Response`.

**Rationale**: This supplies Function Calling metadata while keeping execution authority in the future `ReActLoop + ToolExecutor`. Lesson 20's `ToolRegistry` can provide OryxOS metadata without importing Eino, and connector replacement affects only `internal/provider`.

**Alternatives considered**:

- Use Eino ADK automatic Tool execution: prohibited by the constitution.
- Invent incomplete name-only schemas from `Profile.Tools`: rejected because the model needs truthful parameter schemas.
- Put Eino core or Eino-ext types into runtime, Tool, or Profile packages: rejected because all Eino types are restricted to the Provider adapter edge.

## Decision 6: Persist exactly one audit record for every call attempt

**Decision**: `ProviderService.Chat` measures the connector call, extracts available usage, sanitizes any failure, and asks `LlmCallRepository` to insert one record before returning. Missing usage becomes zero. A connector failure is returned only after its failed audit row is stored. A persistence failure is never swallowed and prevents a successful result from being reported as fully handled.

**Rationale**: Success-only logging omits the incidents that most need investigation. One repository call per logical model attempt gives a testable exactly-once boundary.

**Error precedence**:

- Connector succeeds, insert succeeds: return the original response.
- Connector fails, insert succeeds: return the sanitized connector error.
- Connector succeeds, insert fails: return a persistence error.
- Connector and insert both fail: return a persistence error that preserves the sanitized connector failure as joined context without exposing secrets.

**Alternatives considered**:

- Log only: rejected because call records are a Day One requirement.
- Insert before calling the model and update later: rejected because it creates partial state and a second write path.
- Wrap the external model call in a database transaction: rejected because external latency must not hold SQLite locks.

## Decision 7: Use hand-maintained idempotent SQL

**Decision**: Add a repository-owned SQL migration for `llm_calls` using `CREATE TABLE IF NOT EXISTS` and `CREATE INDEX IF NOT EXISTS`. Execute it with `DB.Exec`; do not call `AutoMigrate` and do not add a fourth migration-history business table.

**Rationale**: The production schema remains reviewable and identical to the test schema while preserving the fixed three-table core contract. WAL and `busy_timeout` are connection setup concerns and remain compatible with the pure-Go dialector.

**Alternatives considered**:

- GORM `AutoMigrate`: rejected by the approved architecture.
- A schema-migrations table in lesson 16: rejected because it would violate the fixed core-table count unless later governance explicitly distinguishes infrastructure metadata.

## Decision 8: Test through deterministic seams and opt-in live smoke tests

**Decision**: Default service tests use fake OryxOS `llm.ChatModel` implementations; adapter tests use a fake Eino `ToolCallingChatModel`; repository tests use temporary SQLite. Live tests carry `//go:build integration`, require the relevant credentials, and separately exercise DeepSeek and MiniMax.

**Rationale**: Unit tests must be fast, deterministic, and free of paid external calls. Live smoke tests verify credentials, endpoint behavior, connector compatibility, non-empty responses, and successful audit insertion without destabilizing default CI.

**Alternatives considered**:

- Run live Provider calls in ordinary `go test ./...`: rejected because it couples CI reliability and cost to external APIs.
- Expose Eino core as the runtime mocking boundary: rejected because OryxOS now owns the runtime port; Eino fakes belong only in Provider adapter tests.
