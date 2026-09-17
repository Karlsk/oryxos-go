---

description: "Implementation tasks for lesson 16 LLM Provider integration"
---

# Tasks: LLM Provider Integration

**Input**: Design documents from `/specs/001-provider-integration/`

**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`, `contracts/`, `quickstart.md`

**Tests**: Required by the lesson-16 harness. For every behavior group, write the named tests first, run them, and confirm they fail for the intended missing behavior before implementing production code.

**Organization**: Tasks are grouped by user story so routing/isolation, configuration, and durable audit behavior can each be tested as an increment.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel because it changes different files and does not depend on another incomplete task in the same phase
- **[Story]**: Maps the task to a user story in `spec.md`
- Every task names its target file path

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Lock the user-selected stable dependency set without changing Provider behavior yet.

- [x] T001 Add direct pins for Eino `v0.9.19`, Eino-ext DeepSeek `v0.1.7`, Eino-ext OpenAI `v0.1.13`, GORM `v1.31.2`, and `glebarez/sqlite v1.11.0`, then run `go mod tidy` and confirm the selected modules in `go.mod` and `go.sum`
- [x] T002 Record the resolved production dependency closure and verify it contains `modernc.org/sqlite` but no production load/import of `mattn/go-sqlite3` or `github.com/cloudwego/eino/adk`, documenting upstream test-only module metadata separately, in `specs/001-provider-integration/quickstart.md`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Strengthen the shared secret-sanitization boundary and create deterministic Eino test seams used by all stories.

**⚠️ CRITICAL**: User story work starts only after this phase passes.

- [x] T003 [P] Add failing redaction tests for Provider API keys, authorization material, and secret-bearing URLs in `internal/config/redact_test.go`
- [x] T004 [P] Add reusable fake `model.ToolCallingChatModel`, factory capture, Tool metadata source, call recorder, and deterministic clock helpers in `internal/provider/provider_test.go`
- [x] T005 Make the Provider redaction cases pass without leaking matched values in `internal/config/redact.go`

**Checkpoint**: Provider errors and tests share safe deterministic boundaries.

---

## Phase 3: User Story 1 - Multiple Profiles Use the Intended Provider (Priority: P1) 🎯 MVP

**Goal**: Route DeepSeek and MiniMax explicitly, isolate model instances by Profile, and bind truthful Tool schemas without executing Tools.

**Independent Test**: Construct DeepSeek and MiniMax Profiles plus two Profiles sharing DeepSeek with different model/temperature values; bind them through capturing factories, call each Profile, and prove routing/settings never cross while returned Tool-call intentions are unchanged and no Tool executes.

### Tests for User Story 1

> **Write these tests first and confirm the intended failures before production implementation.**

- [x] T006 [P] [US1] Add failing routing tests for DeepSeek versus MiniMax, DeepSeek native-default/MiniMax fixed-endpoint construction, missing Provider/model bindings, duplicate registration, and two same-vendor Profiles with isolated construction settings in `internal/provider/provider_service_test.go`
- [x] T007 [P] [US1] Add failing tests for ordered Tool metadata translation, unknown Tool references, `WithTools` binding, response Tool-call ID preservation, and zero Tool executions in `internal/provider/tool_schema_adapter_test.go`

### Implementation for User Story 1

- [x] T008 [US1] Define the complete lesson-16 `Profile` structure and credential-free `ProfileProviderConfig` with Go/YAML field mappings in `internal/profile/profile.go`
- [x] T009 [US1] Define endpoint-free merged `ProviderConfig`, a DeepSeek factory using the native connector default, a MiniMax/OpenAI factory with a fixed official endpoint, factory-by-Provider/model-by-Profile registry behavior, duplicate rejection, and Profile binding in `internal/provider/config.go`, `internal/provider/registry.go`, and `internal/provider/factory.go`
- [x] T010 [P] [US1] Implement the metadata-only `ToolSchemaAdapter` and ordered `ToolInfoSource` seam without importing or executing concrete Tools in `internal/provider/tool_schema_adapter.go`
- [x] T011 [US1] Implement synchronous `ProviderService.Chat` lookup, optional immutable `WithTools` binding, one `Generate` call, context propagation, and unchanged assistant response return in `internal/provider/service.go`
- [x] T012 [US1] Run the User Story 1 tests and package dependency checks, fixing only routing/schema issues in `internal/provider/provider_service_test.go` and `internal/provider/tool_schema_adapter_test.go`

**Checkpoint**: Programmatically supplied Profiles route and isolate both Providers; schema binding cannot execute a Tool.

---

## Phase 4: User Story 2 - Configuration Changes Do Not Require Code Changes (Priority: P2)

**Goal**: Strictly load process Provider declarations and independent credential-free Profile files so valid Profiles survive malformed neighbors and configuration-only switches rebuild the intended model binding.

**Independent Test**: Load a process YAML declaring DeepSeek/MiniMax and a directory containing two valid Profiles plus one malformed Profile; verify both valid registrations, one sanitized diagnostic, configuration-only Provider/model switching after reload, and fatal rejection of invalid global declarations.

### Tests for User Story 2

> **Write these tests first and confirm the intended failures before production implementation.**

- [x] T013 [P] [US2] Add failing strict-loader tests for Provider environment expansion, duplicate/unsupported names, missing credentials, rejection of user-supplied `base_url` and other unknown fields, and sanitized fatal errors in `internal/config/config_test.go`
- [x] T014 [P] [US2] Add failing tests for full Profile YAML parsing, defaults, non-finite temperature rejection, unknown/legacy credential fields, missing or undeclared Providers, malformed-file isolation, lexical duplicate handling, and configuration-only reload switches in `internal/profile/profile_loader_test.go`
- [x] T015 [P] [US2] Add a failing regression assertion that the generated default Profile has only `name`, `model`, and `temperature` under `provider` while the workspace remains five directories and six files in `cmd/oryxos/workspace_test.go`

### Implementation for User Story 2

- [x] T016 [P] [US2] Extend process `ServerConfig` and its strict YAML shape/validation with `name/api_key`-only, redacted `ProviderDefinition` loading for exactly DeepSeek and MiniMax in `internal/config/config.go` and `internal/config/load.go`
- [x] T017 [US2] Implement lexical per-file `ProfileLoader` isolation diagnostics and immutable startup `ProfileRegistry` lookup without runtime `Register` in `internal/profile/loader.go` and `internal/profile/registry.go`
- [x] T018 [P] [US2] Remove `api_key` and `base_url` from the idempotently generated default Profile without adding a workspace artifact in `cmd/oryxos/workspace.go`
- [x] T019 [US2] Integrate loaded Provider declarations with valid Profiles so each reload rebuilds bindings from current configuration and bad Profiles cannot block valid ones in `internal/provider/registry.go`
- [x] T020 [US2] Run the User Story 2 configuration/Profile/workspace tests and verify all error output remains sanitized in `internal/config/config_test.go`, `internal/profile/profile_loader_test.go`, and `cmd/oryxos/workspace_test.go`

**Checkpoint**: Operators can change declared Provider/model choices and restart the loading boundary without code changes or credential-bearing Profiles.

---

## Phase 5: User Story 3 - Every Model Call Is Auditable (Priority: P3)

**Goal**: Persist exactly one Session-linked outcome row for every successful or failed connector call before returning control.

**Independent Test**: Execute one fake successful model call and one fake failed call against temporary SQLite created from the hand SQL; verify one row per attempt, token extraction/zero fallback, duration/outcome fields, sanitized failure text, and explicit persistence-error behavior.

### Tests for User Story 3

> **Write these tests first and confirm the intended failures before production implementation.**

- [x] T021 [P] [US3] Add failing migration/repository tests that execute the hand SQL, inspect required columns/index, round-trip success/failure records, preserve nullable errors, and reject invalid required fields in `internal/store/llm_call_repository_test.go`
- [x] T022 [P] [US3] Extend the failing Provider service tests with success audit, failure-audit-before-return, unavailable-usage zeros, duration, sanitization, insert failure, and combined connector/persistence failure cases in `internal/provider/provider_service_test.go`

### Implementation for User Story 3

- [x] T023 [P] [US3] Add the idempotent `llm_calls` table/index SQL and pure-Go SQLite opener/migration executor with WAL and `busy_timeout`, never `AutoMigrate`, in `internal/store/migrations/001_llm_calls.sql` and `internal/store/sqlite.go`
- [x] T024 [US3] Implement the `LlmCall` GORM mapping and context-aware `LlmCallRepository.Create` validation/insert boundary in `internal/store/llm_call.go` and `internal/store/llm_call_repository.go`
- [x] T025 [US3] Complete `ProviderService.Chat` timing, token extraction, exactly-one audit insert attempt, sanitized error ordering, and audit-before-return behavior in `internal/provider/service.go`
- [x] T026 [US3] Run the User Story 3 repository and Provider tests, including `CGO_ENABLED=0` package compilation, fixing only audit/storage behavior in `internal/store/llm_call_repository_test.go` and `internal/provider/provider_service_test.go`

**Checkpoint**: Successful and failed model attempts are durably queryable and persistence failures are explicit.

---

## Phase 6: Polish & Cross-Cutting Verification

**Purpose**: Finish the lesson harness, synchronize precise APIs, and enforce repository-wide gates.

- [x] T027 [P] Add `//go:build integration` DeepSeek and MiniMax OpenAI-compatible smoke subtests with environment-only credentials, non-empty responses, Tool-call preservation, and success/failure audit assertions in `internal/provider/provider_smoke_test.go`
- [x] T028 [P] Reconcile implemented names, locked versions, `ChatModelConfig` fields, DeepSeek native-default/MiniMax fixed-endpoint behavior, and migration path across `docs/TechnicalSolution.md`, `docs/require/第16节：Agent Provider 原理解析、实现与代码讲解.md`, and `specs/001-provider-integration/quickstart.md`
- [x] T029 Run `gofmt` on every changed Go file, then run `go test ./...`, `go vet ./...`, and `CGO_ENABLED=0 go build ./cmd/oryxos`, recording any environment-only live-smoke skip in `specs/001-provider-integration/quickstart.md`
- [x] T030 Audit the final diff for exactly two Providers, one model per Profile, `name/api_key`-only process Provider declarations, no user-configurable Provider endpoint, no credential-bearing Profile fields, no Eino ADK/automatic Tool execution, no fallback/retry, no `AutoMigrate`, no `mattn/go-sqlite3`, and no extra core table or workspace artifact in `specs/001-provider-integration/checklists/requirements.md`

---

## Phase 7: OryxOS-Owned LLM Boundary Amendment

**Purpose**: Supersede the original Eino-core runtime boundary after the approved architecture decision. Eino remains the Provider implementation, while Runtime and Tool consume OryxOS-owned types.

- [x] T031 Add failing contract tests for OryxOS `ChatModel`, message roles, Tool-call/result correlation, Tool definitions, usage, and finish reason in `internal/llm` and `internal/provider/eino_adapter_test.go`
- [x] T032 Add Eino-import architecture checks proving that production Eino imports exist only under `internal/provider`
- [x] T033 Implement `internal/llm` domain types and `ChatModel`; implement the bidirectional Eino adapter in `internal/provider/eino_adapter.go`
- [x] T034 Change Provider factories and registry storage from Eino `model.ToolCallingChatModel` to OryxOS `llm.ChatModel`, wrapping both DeepSeek and MiniMax connectors
- [x] T035 Change `ProviderService.Chat`, Tool metadata resolution, tests, and integration smoke tests to use OryxOS messages, responses, and Tool definitions without changing audit behavior
- [x] T036 Run formatting, package tests, `go test ./...`, `go vet ./...`, and `CGO_ENABLED=0 go build ./cmd/oryxos`; confirm no Eino imports outside `internal/provider`

---

## Phase 8: Provider Stream Amendment

**Purpose**: Add a transport-neutral OryxOS Stream port and exactly-once terminal audit while keeping ReAct, CLI, and Web/SSE outside lesson 16.

- [x] T037 Add failing `llm.ChatModel.Stream` and Eino adapter tests for ordered deltas, fragmented Tool-call concatenation, one completed response followed by `io.EOF`, empty streams, receive failures, and idempotent close in `internal/llm/model.go` and `internal/provider/eino_adapter_test.go`
- [x] T038 Add failing `ProviderService.ChatStream` tests for shared validation, successful terminal audit, initialization/receive/premature-close failures, persistence errors, cancellation-detached audit, and exactly-once behavior in `internal/provider/provider_service_test.go` and test fakes in `internal/provider/provider_test.go`
- [x] T039 Implement the OryxOS Stream domain contract and Eino Stream adapter without exposing Eino types or introducing goroutines/channels in `internal/llm/model.go` and `internal/provider/eino_adapter.go`
- [x] T040 Implement `ProviderService.ChatStream` and its terminal audit wrapper, reusing Chat validation and persistence rules, in `internal/provider/service.go` and `internal/provider/stream.go`
- [x] T041 Extend the tagged DeepSeek/MiniMax smoke harness with Stream completion and one-row-per-logical-call checks in `internal/provider/provider_smoke_test.go`
- [x] T042 Synchronize Stream boundaries and extension guidance across `docs/TechnicalSolution.md`, `AGENTS.md`, `docs/development/provider-development.md`, `docs/testing/provider-manual-testing.md`, the lesson-16 courseware, and Spec-Kit artifacts
- [x] T043 Run `gofmt` on changed Go files, targeted Provider tests, `go test ./...`, `go vet ./...`, `CGO_ENABLED=0 go build ./cmd/oryxos`, the credential-free tagged smoke harness, and the Eino-import architecture check

---

## Dependencies & Execution Order

### Phase dependencies

- **Phase 1 Setup**: Starts immediately; T002 depends on T001's resolved graph.
- **Phase 2 Foundational**: Starts after T001; T003 and T004 can proceed in parallel, then T005 completes the redaction boundary.
- **User Story 1**: Depends on Phase 2. It is the MVP and establishes the Profile type, registry, adapters, and Provider service seams.
- **User Story 2**: Depends on User Story 1's Profile and registry contracts. Its three failing-test tasks can run in parallel before loader/template implementation.
- **User Story 3**: Depends on User Story 1's service and recorder seam, but not on User Story 2's filesystem loader; it may proceed in parallel with User Story 2 after User Story 1.
- **Polish**: Depends on all selected user stories. T027 and T028 can run in parallel before the final sequential gates T029-T030.
- **Provider Stream amendment**: Depends on the OryxOS-owned boundary amendment. T037-T038 define failing contracts, T039-T040 implement them, T041-T042 complete integration/docs, and T043 runs final gates.

### User story dependency graph

```text
Setup -> Foundation -> US1 (routing/isolation MVP)
                              |-> US2 (configuration/profile loading)
                              |-> US3 (durable audit)
US2 + US3 -> Polish and full gates
```

### Within each user story

- Write the named tests and demonstrate the intended failure before production changes.
- Add data/config structures before loaders, registries, services, or repositories that consume them.
- Keep the external model call outside database transactions.
- Complete the story's targeted tests before advancing to its checkpoint.

### Parallel opportunities

- T003 and T004 affect independent foundational test files.
- T006 and T007 are independent US1 test suites; T010 can proceed once T007 defines the adapter contract.
- T013, T014, and T015 are independent US2 test files; T016 and T018 affect separate production boundaries.
- T021 and T022 establish independent US3 storage/service failures; T023 can proceed independently of Provider service edits.
- T027 and T028 touch separate integration-test and documentation files.

## Parallel Example: User Story 1

```text
Task T006: Define routing and Profile-isolation failures in internal/provider/provider_service_test.go
Task T007: Define metadata translation and no-execution failures in internal/provider/tool_schema_adapter_test.go
```

After the test contracts exist:

```text
Task T009: Implement factories/registry in internal/provider/config.go, registry.go, and factory.go
Task T010: Implement metadata-only conversion in internal/provider/tool_schema_adapter.go
```

## Parallel Example: User Story 2

```text
Task T013: Define process Provider configuration failures in internal/config/config_test.go
Task T014: Define Profile loading/isolation failures in internal/profile/profile_loader_test.go
Task T015: Define default Profile template failure in cmd/oryxos/workspace_test.go
```

## Parallel Example: User Story 3

```text
Task T021: Define SQL/repository failures in internal/store/llm_call_repository_test.go
Task T022: Define audit-ordering failures in internal/provider/provider_service_test.go
```

## Implementation Strategy

### MVP first: User Story 1

1. Complete dependency locking and shared redaction/test seams.
2. Write the US1 routing and Tool-schema tests and confirm their intended failures.
3. Implement the Profile shape, explicit factories, Profile-isolated registry, adapter, and thin Chat call.
4. Stop at the US1 checkpoint and verify the routing matrix independently.

### Incremental delivery

1. **US1**: Correct Provider routing, per-Profile isolation, no Tool execution.
2. **US2**: Operational two-layer configuration and bad-Profile isolation.
3. **US3**: Durable success/failure audit with pure-Go SQLite.
4. **Polish**: Tagged live harness, documentation reconciliation, and full repository gates.

### Scope controls

- Do not implement ReAct iteration, Tool execution, ReAct/CLI streaming, SSE/WebSocket, fallback, retry, circuit breakers, or cost dashboards. Provider-level Stream is explicitly in scope.
- Do not add a config file to `.oryxos init`, a CLI leaf command, REST endpoint, or database table.
- Do not expose any Eino core or Eino-ext type outside `internal/provider`.
- Do not run live Provider tests in default CI or persist real credentials.

## Notes

- `[P]` means file-level independence, not permission to violate test-first ordering.
- User Story 2 and User Story 3 can proceed concurrently only after User Story 1 establishes their shared contracts.
- Live smoke tests require user-supplied environment credentials; absence is a documented skip, not a reason to weaken deterministic tests.
- No implementation task includes committing, pushing, or changing worktrees.
