# Implementation Plan: LLM Provider Integration

**Branch**: `[001-provider-integration]` | **Date**: 2026-09-14 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/001-provider-integration/spec.md`

## Summary

Deliver lesson 16 as a thin, synchronous Provider implementation of the OryxOS-owned `llm.ChatModel` port. Process configuration declares DeepSeek and MiniMax names plus environment-backed API keys; credential-free Profiles select Provider/model/temperature. Explicit Eino-ext factories are keyed by Provider name and own endpoint policy: DeepSeek relies on its native connector's official default, while MiniMax fixes its OpenAI-compatible endpoint internally. Each Eino connector is wrapped by an adapter and independently constructed `llm.ChatModel` instances are keyed by Profile name. `ProviderService.Chat` passes OryxOS Tool definitions, makes exactly one model call, preserves assistant Tool calls, and persists one success or failure `llm_calls` record before returning.

## Technical Context

**Language/Version**: Go `1.26.0` (repository module; Go 1.26+ contract)

**Primary Dependencies**: Eino core `v0.9.19`; Eino-ext DeepSeek `v0.1.7`; Eino-ext OpenAI `v0.1.13`; GORM `v1.31.2`; `github.com/glebarez/sqlite v1.11.0`; existing `gopkg.in/yaml.v3 v3.0.1`; standard `log/slog`

**Storage**: SQLite at `.oryxos/sessions/oryxos.db` through the pure-Go `glebarez/sqlite` dialector; lesson 16 adds only the `llm_calls` core table using repository-owned SQL

**Testing**: Go `testing`; OryxOS `llm.ChatModel` fakes for service tests, Eino connector fakes for adapter tests, temporary SQLite integration tests, and opt-in `//go:build integration` live Provider smoke tests

**Target Platform**: Single-process Linux/macOS development and Linux release binary; `CGO_ENABLED=0` required

**Project Type**: Existing single Go CLI/service repository with internal packages

**Performance Goals**: Provider wrapper adds only one registry lookup, one schema conversion, timing/usage extraction, and one short audit insert around each external model call; no database transaction spans network I/O

**Constraints**: Exactly DeepSeek + MiniMax; MiniMax uses OpenAI compatibility; Provider endpoints are factory-owned and not user-configurable; synchronous calls; no Eino ADK; no automatic Tool execution; no fallback/retry/streaming API; secrets redacted; bad Profiles isolated; invalid process Provider declarations fail; no `AutoMigrate`; no fourth business table

**Scale/Scope**: One startup snapshot containing multiple Profiles, including multiple isolated instances of the same Provider; four required unit-test files plus one tagged smoke-test file; one audit table

## Constitution Check

*GATE: Passed before Phase 0 research and re-checked after Phase 1 design.*

| Gate | Result | Evidence |
|---|---|---|
| Requirements remain the source of truth | PASS | The approved two-layer configuration and audit outcome fields were synchronized to `docs/DemandAnalysis.md` before technical and governance artifacts. |
| OryxOS retains ReAct/Tool execution control | PASS | Provider binds schemas and returns Tool-call intentions only; no ADK or Tool execution is introduced. |
| OryxOS owns the runtime boundary | PASS | Runtime-facing service and tests target `llm.ChatModel`; Eino core/Eino-ext imports are confined to `internal/provider` adapters and factories. |
| Factories are explicit and models are Profile-isolated | PASS | Factory map uses Provider names and owns endpoint policy; DeepSeek uses its native default and MiniMax sets its compatible endpoint; model map uses `Profile.name`; duplicate registration cannot overwrite. |
| Agent remains Profile + Skill | PASS | Profile remains the per-Agent selection source and retains the full structural fields; Provider credentials stay process-level. |
| Call records exist from Day One | PASS | Every connector attempt has exactly one `llm_calls` insert attempt with outcome and sanitized failure. |
| Security and pure-Go binary | PASS | Environment-backed credentials, sanitization, `glebarez/sqlite`, and the CGO-disabled build gate are explicit. |
| Fixed scope and counts | PASS | Exactly two Providers and one of the already-approved three business tables; no new endpoint, CLI leaf, Demo, or workspace artifact. |
| Repository-maintained migrations | PASS | `llm_calls` is created by hand-maintained idempotent SQL; `AutoMigrate` is prohibited and tested. |
| Synchronous shared runtime direction | PASS | `context.Context` propagates through schema resolution, model call, and persistence; no asynchronous path is added. |

Post-design review also passes: [data-model.md](./data-model.md) adds no forbidden data category, [provider-service.md](./contracts/provider-service.md) preserves the OryxOS/Eino adapter boundary, and [llm_calls.sql](./contracts/llm_calls.sql) preserves the three-table contract.

## Project Structure

### Documentation (this feature)

```text
specs/001-provider-integration/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── configuration.md
│   ├── llm_calls.sql
│   └── provider-service.md
├── checklists/
│   └── requirements.md
└── tasks.md
```

### Source Code (repository root)

```text
go.mod
go.sum
cmd/oryxos/
├── workspace.go                         # remove credentials/base URL from default Profile
└── workspace_test.go                    # protect the credential-free template contract
internal/
├── llm/
│   ├── model.go                          # OryxOS ChatModel port
│   └── message.go                        # Message/Tool/Usage domain types
├── config/
│   ├── config.go                        # process Provider declarations
│   ├── load.go                          # strict Provider validation and ENV expansion
│   └── config_test.go
├── profile/
│   ├── profile.go                       # complete Profile data shape
│   ├── loader.go                        # strict per-file load and isolation diagnostics
│   ├── registry.go                      # immutable startup lookup by Profile.name
│   └── profile_loader_test.go
├── provider/
│   ├── config.go                        # merged ProviderConfig
│   ├── factory.go                       # DeepSeek and MiniMax/OpenAI-compatible factories
│   ├── registry.go                      # factories by Provider, models by Profile
│   ├── service.go                       # synchronous Chat and audit-before-return
│   ├── eino_adapter.go                  # OryxOS/Eino request-response conversion
│   ├── provider_service_test.go
│   ├── eino_adapter_test.go
│   └── provider_smoke_test.go            # integration build tag
└── store/
    ├── sqlite.go                        # pure-Go connection and migration execution
    ├── llm_call.go                      # GORM model
    ├── llm_call_repository.go
    ├── llm_call_repository_test.go
    └── migrations/
        └── 001_llm_calls.sql
```

**Structure Decision**: Extend the existing single Go module and its established package boundaries. OryxOS model ports and domain types belong to `internal/llm`; Profile parsing belongs to `internal/profile`; all Eino imports, connector factories, and conversion adapters belong to `internal/provider`; GORM and SQLite remain in `internal/store`; process YAML remains in `internal/config`.

## Design Sequence

### Phase 0: Dependency and behavior research

Completed in [research.md](./research.md): resolve latest stable versions, inspect their local source APIs, verify Eino interface compatibility, decide the two-layer configuration boundary, define isolated registry ownership, define no-execution Tool schema binding, and choose the exactly-once audit behavior.

### Phase 1: Contracts and data design

Completed in:

- [data-model.md](./data-model.md): configuration, Profile, registry, audit model, and state transitions.
- [configuration.md](./contracts/configuration.md): strict YAML and redaction contract.
- [provider-service.md](./contracts/provider-service.md): factory, registry, adapter, Chat, and error-ordering contract.
- [llm_calls.sql](./contracts/llm_calls.sql): authoritative table/index shape.
- [quickstart.md](./quickstart.md): deterministic and opt-in live verification.

### Phase 2: Task decomposition

Generate `tasks.md` with test-first ordering. Each behavioral group starts with a failing focused test, then the minimum implementation, then package/full-repository verification. Live smoke tests remain opt-in and must never be necessary for ordinary CI.

## Key Implementation Decisions

1. Extend the current strict process YAML loader instead of creating a second YAML engine. Provider definitions are validated before Profile loading and sensitive values use the existing sanitizer.
2. Parse Profile files independently and in lexical order. Return a registry plus diagnostics; filesystem-wide failures remain fatal, while per-file failures are logged and skipped.
3. Keep Provider factory registration explicit. DeepSeek uses `deepseek.ChatModelConfig` without `BaseURL`, relying on the native connector's official default; MiniMax uses `openai.ChatModelConfig` with a factory-owned OpenAI-compatible endpoint and a local `float32` copied to the required temperature pointer. Neither process nor Profile YAML accepts an endpoint override.
4. Construct one model per valid Profile and reject duplicate model bindings. Never cache a configured model only by Provider name.
5. Define Tool metadata as OryxOS `llm.ToolDefinition`. Lesson 20 can make `ToolRegistry` implement the metadata source without importing Eino or changing `ProviderService`; only the Eino adapter converts it to `schema.ToolInfo`.
6. Make the model call outside database transactions. Build the audit record after the call and perform one short insert before returning.
7. Keep the SQL migration embedded or otherwise repository-owned and run the same script in tests and production. Do not use `AutoMigrate`.
8. Update the existing default Profile template as the user-approved migration: remove legacy `api_key` and `base_url`, retain `name/model/temperature`, and do not alter the five-directory/six-file count. Process Provider declarations accept only `name/api_key`; the DeepSeek factory uses the native endpoint default and the MiniMax factory fixes its compatibility endpoint.

## Verification Strategy

### Default harness

- `profile_loader_test.go`: full structural parsing, strict unknown fields, missing/undeclared Provider, invalid file isolation, deterministic duplicate handling, and process credential expansion/redaction.
- `provider_service_test.go`: DeepSeek/MiniMax factory selection, same-vendor Profile isolation, one OryxOS Generate call, response/Tool-call preservation, success/failure audit, usage fallback to zero, persistence failure, and context cancellation.
- `eino_adapter_test.go`: ordered Tool-definition conversion, four message roles, Tool-call/result identity, usage/finish reason preservation, invalid schema errors, and zero Tool executions.
- `llm_call_repository_test.go`: execute the hand SQL, inspect columns/index, write/read success and failure rows, reject invalid required fields, and confirm no `AutoMigrate` dependency.
- existing workspace/config/application tests: protect foundation behavior and the credential-free Profile template.

### Opt-in integration harness

- `provider_smoke_test.go` has `//go:build integration`.
- DeepSeek and MiniMax are separate subtests and read only environment-backed credentials.
- Each subtest requires a non-empty response and an audit row; Tool Calling fixtures preserve call IDs and do not execute Tools.
- Missing credentials skip; supplied-but-invalid credentials fail and leave a sanitized failure audit row.

### Delivery gates

```bash
gofmt -w <each changed Go file>
go test ./...
go vet ./...
CGO_ENABLED=0 go build ./cmd/oryxos
```

The implementation phase must also run `go mod tidy`, confirm the five direct pins, and verify that the resolved graph contains no CGO SQLite driver.

## Complexity Tracking

No constitution violations require justification.
