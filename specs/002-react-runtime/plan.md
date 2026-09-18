# Implementation Plan: ReAct Runtime Orchestration

**Branch**: `002-react-runtime` | **Date**: 2026-09-17 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/002-react-runtime/spec.md`

## Summary

Deliver lesson 17 as the OryxOS-owned synchronous Agent runtime: `AgentService.Invoke` resolves a
Profile and Session through narrow ports, `ReActLoop` repeatedly calls the existing audited
`ProviderService.Chat`, preserves complete assistant Tool calls, executes Tool calls serially through
`ToolExecutor`, appends matching Tool messages, and stops on a final response, cancellation,
unrecoverable failure, or the Profile iteration limit. `PromptBuilder`, `BootstrapLoader`, and
`SkillLoader` assemble bounded, provenance-labelled context from an immutable startup snapshot.
Tool outcomes are stored in the second approved core table through a hand-maintained migration.
When a model response succeeds but its `llm_calls` record cannot be persisted, the Provider returns
the response with a marked audit error; the loop retains that assistant message for diagnosis and
fails closed before executing any Tool call from the unaudited response.

Runtime and Tool packages use only OryxOS `internal/llm` domain types. They do not import Eino,
Gin, Cobra, or GORM. Session identity and persistence implementation remain lesson 18 work; lesson
17 introduces only the minimal Session aggregate and collaboration seam required by the approved
runtime contracts.

## Technical Context

**Language/Version**: Go `1.26.0` (repository module; Go 1.26+ contract)

**Primary Dependencies**: Standard library `context`, `encoding/json`, `errors`, `fmt`, `io`,
`log/slog`, `slices`, `strings`, `sync`, `time`, and `unicode/utf8`; existing OryxOS
`internal/llm`, `internal/profile`, and
`internal/provider` contracts; GORM `v1.31.2`; `github.com/glebarez/sqlite v1.11.0`; no new module
dependency

**Storage**: Existing pure-Go SQLite database at `.oryxos/sessions/oryxos.db`; add only the approved
`tool_invocations` table and `session_id` index through repository-owned SQL

**Testing**: Go `testing`; deterministic model, Session, Tool, clock, and filesystem doubles;
temporary SQLite repository tests; `go test -race ./internal/session`; no real network in the
default harness

**Target Platform**: Single-process Linux/macOS development and Linux release binary;
`CGO_ENABLED=0` is mandatory

**Project Type**: Existing single Go CLI/service repository with internal packages

**Performance Goals**: Synchronous orchestration adds bounded in-memory message assembly, ordered
Tool dispatch, and one short audit insert per logical Tool call; no database transaction spans an
LLM or Tool call

**Constraints**: Runtime owns the loop; OryxOS domain types only outside `internal/provider`;
default `max_iterations=10` and `max_history_turns=20`; Memory capped at 4,000 characters; an
application-supplied per-Profile input-rune budget trims only oldest complete historical turns; Tool
calls serial; cancellation propagated; Session state saved on success and failure; exactly one
final Tool audit row; no automatic Agent execution, goroutine orchestration, hot reload, summary
compression, Agent delegation, or Agent-level streaming

**Scale/Scope**: One immutable startup context snapshot per valid Profile; six required harness
test files; one additional approved core table; no endpoint, CLI command, Provider, Tool catalog,
Demo, or workspace-count changes

## Constitution Check

*GATE: Passed before Phase 0 research and re-checked after Phase 1 design.*

| Gate | Result | Evidence |
|---|---|---|
| Requirements remain the source of truth | PASS | Scope comes from `docs/DemandAnalysis.md`, `docs/TechnicalSolution.md`, and the unique lesson 17 requirement. No extension-stage capability is introduced. |
| OryxOS retains ReAct control | PASS | `ReActLoop` owns stopping, model calls, Tool dispatch, message accumulation, cancellation, and failure propagation; no ADK path exists. |
| OryxOS owns model and Tool boundaries | PASS | Runtime and Tool code use `internal/llm`; Eino remains confined to the existing Provider adapter. `InvokableTool` and `OryxTool` follow Constitution v5.0.0. |
| Providers remain explicit and Profile-isolated | PASS | The existing audited `ProviderService.Chat(ctx, sessionID, Profile, messages)` contract is consumed without changing factory or registry ownership. |
| Agent remains Profile plus Skill | PASS | Skill content is loaded only from Profile references and remains prompt context, never a Tool. No Agent directory or Markdown-derived Profile is added. |
| One controlled Tool path | PASS | `ToolExecutor` performs lookup, Profile filtering, argument validation, timeout, eligible retry, execution, result correlation, and final recording in serial order. |
| State and records exist from Day One | PASS | Model calls keep lesson 16 auditing; every logical Tool call gets one `tool_invocations` final record. Session persistence is invoked through the lesson 18 seam on success and failure. |
| Security and pure-Go binary | PASS | Errors are sanitized before storage, file loaders stay under the workspace boundary, concrete external IO remains behind the approved Sandbox seam, and SQLite stays pure Go. |
| One synchronous runtime | PASS | CLI, Web, and Scheduler request shapes converge on `AgentService.Invoke`; no streaming transport, parallel Tool call, or goroutine pipeline is added. |
| Fixed core contract | PASS | This feature adds only the already-approved second of three tables and changes none of the fixed Provider, Tool, endpoint, command, Demo, or workspace counts. |

Post-design re-check: PASS. The data model and contracts below add no table beyond
`tool_invocations`, no third-party runtime type, and no implementation assigned to later lessons.

## Project Structure

### Documentation (this feature)

```text
specs/002-react-runtime/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── runtime.md
│   ├── tool-execution.md
│   └── tool_invocations.sql
└── tasks.md
```

### Source Code (repository root)

```text
internal/
├── bootstrap/
│   ├── loader.go
│   └── loader_test.go
├── profile/
│   ├── loader.go                         # lesson 17 setting defaults/range validation
│   └── profile_loader_test.go
├── runtime/
│   ├── agent_service.go
│   ├── agent_service_test.go
│   ├── prompt_builder.go
│   ├── prompt_builder_test.go
│   ├── react_loop.go
│   ├── react_loop_test.go
│   └── types.go                          # documented Agent request/response contract
├── session/
│   └── session.go                        # minimal message aggregate; service/store stay lesson 18
├── skill/
│   ├── loader.go
│   └── loader_test.go
├── store/
│   ├── sqlite.go
│   ├── tool_invocation.go
│   ├── tool_invocation_repository.go
│   ├── tool_invocation_repository_test.go
│   └── migrations/
│       └── 002_tool_invocations.sql
└── tool/
    ├── executor.go
    ├── executor_test.go
    └── types.go                          # documented OryxOS InvokableTool/OryxTool contract
```

**Structure Decision**: Extend the established single Go module. Runtime orchestration remains in
`internal/runtime`; the minimal conversation aggregate lives in `internal/session` so lesson 18 can
add identity and persistence without a cycle; Tool ports and execution live in `internal/tool`;
filesystem context loaders remain separate; GORM and migration code remain in `internal/store`.
No Eino import crosses out of `internal/provider`.

## Design Sequence

### Phase 0: Boundary and behavior research

Completed in [research.md](./research.md): preserve the Provider contract, define the Session seam,
define complete Tool-call message accumulation, define whole-turn history truncation, define
immutable loader snapshots, preserve the OryxOS Tool boundary, constrain retries, and verify the
existing GORM/pure-Go SQLite APIs.

### Phase 1: Contracts and data design

Completed in:

- [data-model.md](./data-model.md): Agent request/response, Session messages, prompt snapshot, Tool
  execution metadata, and Tool invocation persistence.
- [runtime.md](./contracts/runtime.md): shared invocation and ReAct behavior.
- [tool-execution.md](./contracts/tool-execution.md): OryxOS Tool port, execution ordering, retries,
  Sandbox seam, and audit behavior.
- [tool_invocations.sql](./contracts/tool_invocations.sql): authoritative table/index shape.
- [quickstart.md](./quickstart.md): deterministic harness and remaining manual checks.

### Phase 2: Task decomposition

Generate `tasks.md` with test-first ordering. Every behavior task begins with a focused failing test,
then the minimum implementation, then package and repository verification. The fixed checkpoint
after task generation prevents implementation before user review.

## Key Implementation Decisions

1. Keep `ReActLoop` independent of concrete Provider implementations by depending on the narrow
   audited `Chat(ctx, sessionID, selectedProfile, messages)` behavior already implemented by
   `ProviderService`. The production service satisfies the seam without runtime importing Eino.
2. Introduce only the race-safe Session message aggregate needed by the loop. A per-Session mutex
   serializes reads and appends, and returned message slices are defensive copies. `AgentService` depends on a
   narrow Session collaboration seam; lesson 18 supplies `GetOrCreate`, explicit Session-ID and
   stateless resolution, database persistence, archive behavior, and identity generation.
   `AgentService` never concatenates a Session ID.
3. Append the user message once before the loop. On every iteration, append the complete assistant
   message before inspecting Tool calls; append one `llm.RoleTool` message carrying the matching
   `ToolCallID` and Tool name after each serial execution. Tool failure is represented in the Tool
   message and returned, preserving the observable failed state.
4. Treat one history turn as a user message plus every following assistant and Tool message up to
   the next user message. Truncation removes complete oldest turns so no assistant Tool call or Tool
   result becomes orphaned. `PromptBuilder` separates the current user-rooted turn from prior
   history, so a second ReAct iteration includes the user message exactly once together with its
   assistant Tool call and Tool observations. After applying `max_history_turns`, it enforces an
   application-supplied per-Profile rune budget by dropping only oldest complete historical turns.
   Protected system context and the complete current turn are never split; if they alone exceed the
   budget, prompt construction fails explicitly. This seam does not add a Provider or Profile YAML
   field and can later be populated from model metadata during application assembly.
5. Apply `max_iterations=10` and `max_history_turns=20` in Profile loading when omitted and reject
   non-positive explicit values. This is the lesson 17 change to the lesson 16 Profile parser;
   existing Profile contracts and fields remain unchanged.
6. Load Bootstrap and Skill content once while assembling the startup snapshot. Each Skill reference
   is a relative path beneath `.oryxos/skills/` that must resolve to a `SKILL.md` file; loaders never
   auto-discover directories, and duplicate references fail. Bootstrap defaults
   to `AGENTS.md`, `SOUL.md`, `USER.md`; explicitly configured files replace that list. Explicit
   missing references fail, while missing default templates contribute empty labelled sections.
   Loaders normalize and confine paths to the workspace root and retain a Sandbox-policy seam for
   lesson 24 rather than inventing a second whitelist system.
7. Keep Memory behind a narrow reader seam because lesson 22 supplies `MarkdownMemoryStore`.
   `PromptBuilder` caps the returned text at 4,000 characters and labels every source. A clock is
   injected for deterministic current-date/time tests.
8. Define the documented OryxOS `InvokableTool` and `OryxTool` types in `internal/tool` because
   `ToolExecutor` requires them. Do not implement `ToolRegistry`, built-in Tools, or MCP adapters;
   lesson 20 supplies those and satisfies the executor's package-private lookup seam.
9. Validate Tool arguments against the JSON object-schema subset required by the core Tool schemas
   using standard-library JSON decoding. Invalid JSON, missing required properties, unexpected
   properties when forbidden, and primitive type mismatches fail before invocation; unsupported
   schema constructs fail closed. This adds no dependency and does not leak Eino schema types.
10. A concrete Tool remains responsible for calling the appropriate approved Sandbox method before
    external IO. Lesson 17 keeps that dependency seam visible; lesson 24 supplies the real
    `ValidatePath`, `ValidateCommand`, and `ValidateURL` implementation. No speculative generic
    Sandbox API is introduced.
11. Apply the Tool timeout with `context.WithTimeout`. Retry only explicitly retryable failures when
    `OryxTool.Idempotent` is true or the call provides a reliable idempotency marker, using bounded
    exponential backoff and no more than three retries. The final logical outcome produces one
    database row; every physical attempt produces one structured log with correlation identifiers,
    Tool name, one-based attempt number, final-attempt status, success, duration, and sanitized
    failure text. Tool arguments and results are not logged.
12. Extend the existing SQLite opener to apply `002_tool_invocations.sql` after lesson 16's
    migration. Calls execute outside database transactions; final recording uses one short
    context-aware insert. Stored failure text goes through the existing sanitizer.
13. Preserve a successful model response when only `llm_calls` persistence fails by returning it
    with Provider `AuditPersistenceError`. Runtime recognizes only the narrow
    `ModelResponseAvailable() bool` marker, appends the assistant response, returns the error, and
    executes none of its Tool calls. Ordinary model failures do not append a fabricated response.

## Verification Strategy

### Default harness

- `react_loop_test.go`: no-Tool final response; one and multiple Tool calls; exact serial order;
  maximum-iteration stop; complete assistant Tool-call and matching Tool-result accumulation;
  cancellation and unrecoverable error propagation.
- `prompt_builder_test.go`: exact provenance order and precedence; whole-turn history and input
  budget truncation; current user turn appears exactly once across ReAct iterations; 4,000-character
  Memory cap; current date/time at the end.
- `executor_test.go`: lookup and Profile allow-list; argument rejection before invocation; serial
  behavior; a timeout that actually expires; eligible bounded retry; one sanitized correlated log
  per physical attempt; no retry for non-idempotent Tools; success/failure audit with redacted error
  and exactly one final record.
- `agent_service_test.go`: CLI/Web/Scheduler-shaped requests through one `Invoke`; explicit Profile
  isolation; no Session-ID construction; save on success and failure; joined run/save errors.
- bootstrap/skill `loader_test.go`: default Bootstrap order; explicit override order; relative
  `SKILL.md` references; duplicate and missing
  explicit reference failure; empty default file handling; labelled boundaries; immutable snapshot;
  workspace path confinement; no hot reload.
- `tool_invocation_repository_test.go`: hand migration columns/index; success/failure write and read
  by `session_id`; validation; no `AutoMigrate`; both approved audit tables remain available.
- `profile_loader_test.go`: setting defaults and rejection of non-positive explicit bounds while all
  lesson 16 Profile isolation tests remain green.

### Manual checks

- Run the weather conversation once with a real Provider and real `http_get` after lesson 20 and
  Sandbox wiring exist; verify the model issues a Tool call and returns a clothing recommendation.
- Run the same multi-turn Tool Calling flow through MiniMax/OpenAI compatibility and inspect complete
  assistant Tool-call IDs and matching Tool messages.
- Review imports to confirm Eino remains only in `internal/provider` and no ADK execution path exists.

### Delivery gates

```bash
gofmt -w <each changed Go file>
go test ./...
go test -race ./internal/session
go vet ./...
CGO_ENABLED=0 go build ./cmd/oryxos
```

Implementation must also run focused package tests after each red/green step, `git diff --check`,
the lesson 16 regression harness, and grep-based checks for `AutoMigrate`, CGO SQLite, Eino outside
Provider, skipped tests, and credential literals.

## Complexity Tracking

No constitution violations require justification.
