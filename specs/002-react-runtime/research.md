# Phase 0 Research: ReAct Runtime Orchestration

## Decision 1: Consume the audited Provider contract through a narrow runtime seam

**Decision**: `ReActLoop` depends on the behavior of the existing
`ProviderService.Chat(ctx, sessionID, Profile, messages)` method, expressed through a narrow local
interface. Production assembly passes `internal/provider.Service`; runtime code imports only
OryxOS `llm` and Profile domain types and never imports Eino.

**Rationale**: Lesson 16 already owns Provider selection, Tool-definition binding, model invocation,
and `llm_calls` recording. Calling `llm.ChatModel` directly from the loop would bypass that audit
boundary; importing the concrete Provider package would couple runtime to an edge adapter.

When a model response succeeds but recording it fails, Provider returns the complete response plus
an audit-persistence error marked with `ModelResponseAvailable`. `ReActLoop` consumes only that
narrow behavioral marker, retains the assistant message for diagnosis, returns the failure, and
does not execute Tool calls. A model failure never carries the marker, even if recording that
failure also fails.

**Alternatives considered**:

- Call `llm.ChatModel.Generate` directly: rejected because it bypasses the approved Provider audit
  service.
- Import Eino or a connector in runtime: prohibited by the architecture and Constitution v5.0.0.

## Decision 2: Preserve complete message groups throughout the loop

**Decision**: Append the current user message once before iteration. Append each complete assistant
message before deciding whether to stop. For every Tool call, append one Tool-role message with the
original call ID, Tool name, and textual result before the next model call. Tool calls from the same
assistant message execute serially in slice order.

**Rationale**: OpenAI-compatible multi-turn Tool Calling requires the assistant Tool-call message and
each correlated Tool result to remain adjacent and identifiable. Saving only assistant text breaks
the next model turn and post-run inspection.

**Alternatives considered**:

- Save only final text: rejected because Tool-call identity and arguments are lost.
- Execute one response's Tool calls concurrently: rejected because ordering and context become
  nondeterministic.

## Decision 3: Truncate complete user-rooted turns

**Decision**: A history turn begins with a user message and includes subsequent assistant and Tool
messages until the next user message. Keep the newest configured number of complete turns and never
split an assistant Tool-call message from its Tool results. The current user turn is always retained.

**Rationale**: The requirements specify recent conversation turns rather than an arbitrary message
count. Whole-turn truncation is the only interpretation that also preserves the mandatory Tool-call
correlation invariant.

**Alternatives considered**:

- Count every message as one turn: rejected because one Tool-assisted exchange can contain several
  messages and may be cut into an invalid sequence.
- Summarize old turns: explicitly deferred to the extension stage.

## Decision 4: Build immutable prompt-source snapshots at startup

**Decision**: `BootstrapLoader` and `SkillLoader` normalize workspace-relative references, reject
escapes, load approved content once, and return copied labelled sections. Each Profile Skill entry
is a relative path beneath `.oryxos/skills/` and must resolve to a file named `SKILL.md`; duplicate
references fail and no directory auto-discovery occurs. Bootstrap defaults to
`AGENTS.md`, `SOUL.md`, and `USER.md`; an explicit list replaces the default. Missing explicit
references fail, while absent default templates yield empty sections. Runtime retains no file watcher.

**Rationale**: One invocation must not observe two different configurations across ReAct rounds.
Labels preserve provenance and make instruction precedence visible.

**Alternatives considered**:

- Read files on every model iteration: rejected because it adds IO and permits mid-request drift.
- Concatenate raw files without boundaries: rejected because the model cannot distinguish source
  authority.
- Implement hot reload: explicitly out of scope.

## Decision 5: Add only the minimal Session aggregate before lesson 18

**Decision**: Add the ordered in-memory Session message aggregate needed by `ReActLoop`, protect all
message reads and appends with a per-Session mutex, return defensive copies, and expose a narrow
runtime collaboration seam for obtain/save behavior. Do not implement Session identity generation,
SQLite persistence, active lookup, archive state, or stateless invoke rules in lesson 17.

**Rationale**: The documented loop and shared `AgentService` require a Session value now, while the
course assigns the Session service/store and CLI behavior to lesson 18. Keeping identity generation
behind the seam prevents `AgentService` from constructing IDs prematurely.

**Alternatives considered**:

- Implement the full Session service now: rejected as lesson 18 scope.
- Store messages directly inside runtime without a Session aggregate: rejected because it creates a
  throwaway contract that lesson 18 would have to replace.

## Decision 6: Establish the OryxOS Tool execution port without delivering lesson 20

**Decision**: Add the documented `InvokableTool` and `OryxTool` contracts required by
`ToolExecutor`. Executor lookup is a package-private seam that lesson 20's `ToolRegistry` will
satisfy. Built-in Tools, MCP adapters, registry enumeration, and Tool-list CLI behavior remain
absent.

**Rationale**: `ToolExecutor` cannot be implemented or tested without an invokable Tool contract,
but implementing the catalog or registry would cross the lesson boundary.

**Alternatives considered**:

- Use Eino `tool.InvokableTool`: rejected because Tool packages cannot import Eino.
- Implement `ToolRegistry` early: rejected because it is a lesson 20 delivery.

## Decision 7: Validate, time, retry, and record in one logical execution path

**Decision**: Executor performs exact lookup, Profile allow-list validation, standard-library JSON
schema-subset validation, timeout derivation, eligible retry, invocation, and one final record. An
unsupported schema keyword fails closed. A retry requires an explicitly retryable error plus an
idempotent Tool or reliable idempotency marker; attempts never exceed three retries. Side-effecting
Tools are non-idempotent by default.

**Rationale**: Centralized execution prevents alternate unrecorded paths while preserving the
requirement that concrete Tools perform operation-specific Sandbox checks immediately before IO.

**Alternatives considered**:

- Add a new JSON Schema dependency: rejected because the approved plan adds no dependency and the
  core Tool schemas need a bounded subset.
- Retry every transient-looking failure: rejected because it can repeat writes, commands, posts,
  notifications, or Memory appends.
- Add a speculative generic Sandbox method: rejected because the authoritative Sandbox contract is
  operation-specific and implemented in lesson 24.

## Decision 8: Extend the existing repository-owned SQLite migration chain

**Decision**: Add `002_tool_invocations.sql`, embed it alongside `001_llm_calls.sql`, and execute it
with the existing GORM database handle. Implement a context-aware repository with validation and
short inserts. GORM `v1.31.2` locally confirms `(*gorm.DB).Create(value) *gorm.DB`; the existing
`glebarez/sqlite v1.11.0` path remains backed by pure-Go modernc SQLite.

**Rationale**: This preserves the fixed three-table design, migration reviewability, WAL and busy
timeout settings, and the CGO-disabled binary contract.

**Alternatives considered**:

- `AutoMigrate`: prohibited by the architecture.
- A migration-history business table: rejected because the core business-table count is fixed.
- Hold a transaction during Tool execution: rejected because external work must not hold SQLite
  locks.

## Decision 9: Keep verification deterministic and local

**Decision**: Default tests use model, Tool, Session, clock, Memory, and filesystem doubles plus
temporary SQLite. Real Provider and weather flows remain manual until the dependent built-in Tool
and Sandbox lessons are complete.

**Rationale**: The lesson harness must be fast, repeatable, credential-free, and able to force every
failure branch.

**Alternatives considered**:

- Make real Provider calls part of `go test ./...`: rejected because CI would depend on network,
  credentials, cost, and vendor availability.
