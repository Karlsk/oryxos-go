<!--
Sync Impact Report
- Version change: 1.0.0 -> 1.1.0
- Modified principles:
  - II. ReAct Control Remains in OryxOS: added prompt provenance and precedence
  - VI. All Tools Use One Controlled Execution Path: defined retry limits and attempt logging
  - VII. State and Call Records Exist from Day One: clarified logical call records and state safety
  - VIII. Security and the Pure-Go Binary Are Non-Negotiable: added secret and network boundaries
- Modified sections:
  - Runtime Workspace and Configuration: added non-destructive init and immutable snapshots
  - Delivery Gates: added performance verification and clean-Linux smoke testing
- Added sections: none
- Removed sections: none
- Follow-up TODOs: none
-->

# OryxOS Constitution

## Core Principles

### I. Requirements Are the Source of Truth and Scope Is Phased

`docs/DemandAnalysis.md` MUST remain the unique source for product scope, phase boundaries,
counts, and acceptance criteria. `docs/TechnicalSolution.md` MUST describe implementation
decisions without adding requirements. `AGENTS.md` MUST guide implementation without
overriding either document.

A scope change MUST be made in the demand document first, reflected in the technical
solution second, and synchronized to the project guide and this constitution last.
Core-stage work MUST NOT silently absorb extension-stage capabilities. This ordering keeps
What, How, and engineering governance independently reviewable.

### II. ReAct Control Remains in OryxOS

`ReActLoop` MUST be implemented by OryxOS. It MUST control prompt assembly, model calls,
tool-call parsing, tool execution, result accumulation, iteration limits, cancellation,
timeouts, and persistence of Session and call records.

Prompt content MUST remain isolated by provenance. `PromptBuilder` MUST preserve separate
boundaries for runtime rules, project rules, Agent identity, Skills, user preferences,
long-term Memory, Session history, and the current user message. When governed sources
conflict, precedence MUST be runtime safety rules, then `AGENTS.md`, then Profile identity
and Skills, then `SOUL.md`, then `USER.md`, and finally `MEMORY.md`. `MEMORY.md` MUST
never override instructions. Missing explicitly referenced Skill or Bootstrap files MUST
fail during Profile loading.

Eino ADK or any automatic Agent/ReAct abstraction MUST NOT replace this loop. Automatic
tool execution MUST be disabled so that each tool call is executed exactly once by
`ToolExecutor`. The rationale is to preserve deterministic behavior and leave the core
runtime adaptable without surrendering control to a framework.

### III. Eino Core Is the Runtime Boundary

Runtime code MUST depend on Eino core's `model.ToolCallingChatModel` and Tool interfaces.
Eino-ext concrete connectors MUST be constructed only inside `internal/provider` factories.
Handlers, Scheduler, Runtime, and Tool packages MUST NOT retain, inspect, or construct
Eino-ext concrete types.

DeepSeek MUST use the Eino-ext DeepSeek connector. MiniMax MUST use the Eino-ext OpenAI
connector with an explicit MiniMax-compatible base URL. Core HTTP APIs MUST remain
synchronous JSON; connector streaming support MUST NOT be presented as core SSE support.
This boundary isolates vendor and connector churn from the runtime.

### IV. Providers Are Explicitly Mapped and Profile-Isolated

Provider factories MUST be registered explicitly by `provider.name`. Constructed
`ToolCallingChatModel` instances MUST be stored by `Profile.name`, not only by provider
name. Two Profiles using the same vendor MUST be able to use different models, API keys,
base URLs, and temperatures without sharing mutable configuration.

The core release MUST support exactly DeepSeek and MiniMax. Provider fallback, hedge
racing, circuit breaking, and adaptive routing MUST remain extension-stage work. Explicit
mapping prevents ambiguous discovery and credential/configuration leakage between Profiles.

### V. Agent Equals Profile Plus Skill

A runnable business Agent MUST be defined as Profile plus Skill:

- `.oryxos/profiles/<name>.yaml` defines how the Agent runs.
- `.oryxos/skills/**/SKILL.md` defines what the Agent does and when.
- `Profile.name` MUST be the unique runtime identifier.
- `identity.agent_name` MUST remain display-only.
- `identity.prompt` MUST describe identity and baseline persona, not replace the business Skill.

`.oryxos/agents/` and singular `AGENT.md` MUST NOT be created. Profile configuration
MUST NOT be derived from Markdown frontmatter. Skill, Bootstrap, and Memory are prompt
context and MUST NOT be registered as Tools. One configuration source prevents divergent
runtime identities and duplicated lifecycle mechanisms.

### VI. All Tools Use One Controlled Execution Path

Built-in Tools, MCP Tools, and Go Tools compiled into the binary MUST enter
`ToolRegistry` and execute through `ToolExecutor`. Eino `tool.BaseTool` supplies
metadata only; executable Tools MUST implement `tool.InvokableTool` and
`InvokableRun`. OryxOS runtime metadata MUST be carried by `OryxTool`.

`ToolExecutor` MUST perform schema validation, Profile allow-list filtering, Sandbox
validation, timeout handling, execution, result accumulation, and call recording. Tool
calls returned in one model response MUST execute serially in model order.

Automatic retry MUST occur only when an error is explicitly retryable and the operation is
idempotent or has a reliable idempotency key. Eligible retries MUST use exponential backoff
and MUST stop after at most three retries. Each physical execution attempt MUST be recorded
in structured logs. Side-effecting calls such as `write_file`, `shell`, `http_post`,
`notify`, and `save_memory` MUST NOT retry by default.

### VII. State and Call Records Exist from Day One

Session history MUST be persisted in SQLite. Every model call attempt MUST write
`llm_calls`. Every logical Tool call MUST write exactly one `tool_invocations` record for
its final outcome, including unsuccessful outcomes. Retry attempts MUST NOT introduce
additional core tables or columns. Failure details MUST also appear in structured logs with
request, session, profile, and channel correlation fields.

Long-term Memory MUST use only `.oryxos/memory/MEMORY.md` in the core stage.
`save_memory` MUST append to it, `recall_memory` MUST use keyword matching, and prompt
injection MUST be capped at 4000 characters using simple truncation. Memory backends,
Memory tables, scope partitions, semantic search, and automatic extraction MUST remain
extension-stage work.

Mutable state updates MUST be race-safe. Session message updates MUST be serialized per
Session and committed with short transactions. LLM, MCP, and Tool calls MUST execute
outside database transactions. `MEMORY.md` appends MUST be serialized and atomic.
Persistence failures MUST be returned explicitly and MUST NOT be silently swallowed.
Database migration failure MUST prevent the service from becoming ready.

This principle establishes recoverable state and traceable calls without claiming the
complete audit and governance capabilities reserved for later phases.

### VIII. Security and the Pure-Go Binary Are Non-Negotiable

Real credentials MUST NOT be committed to version control. Generated configuration
templates MUST use placeholders. Runtime secrets MUST be supplied through environment
expansion or separate uncommitted local configuration and MUST remain redacted from logs
and errors.

Because the core HTTP API has no authentication, it MUST only be deployed inside a trusted
network and MUST NOT be exposed directly to the public Internet. HTTPS termination MUST be
provided by a reverse proxy or ingress gateway.

Every Tool MUST pass the application-level Sandbox before execution:

- file paths MUST be normalized and resolved against allowed roots;
- symlink and parent-directory escapes MUST be rejected;
- commands MUST execute as an executable plus argv, never as concatenated shell text;
- HTTP scheme and host MUST be checked, including every redirect target;
- Tool input, output, and execution time MUST be bounded.

The core Sandbox MUST be described as an application-level control, not container-grade
isolation. Docker, K8s Pod, and WASM isolation belong to the extension stage.

SQLite MUST use `github.com/glebarez/sqlite` backed by `modernc.org/sqlite`.
`mattn/go-sqlite3` and any other CGO dependency that breaks the single-binary promise
MUST NOT be introduced. `CGO_ENABLED=0 go build ./cmd/oryxos` is a release gate.

### IX. All Entry Points Share One Synchronous Runtime

CLI, Web Service, and AgentScheduler MUST translate input to `AgentRequest` and call the
same `AgentService.Invoke` path. None may bypass Profile loading, Session management,
Memory, ReAct, Tool validation, or call recording.

The core runtime MUST use synchronous request/response semantics and serial Tool execution.
`context.Context` MUST propagate through CLI, Web, Scheduler, LLM, MCP, and Tool calls.
SSE, WebSocket, parallel Tool execution, and complex asynchronous orchestration MUST remain
outside the core stage.

Scheduler definitions MUST come only from Profile `schedules`. A single schedule MUST skip
overlapping runs, isolate failures, and reuse a stable scheduler Session. It MUST NOT create
core task-management APIs or `scheduled_tasks`/`task_executions` tables.

## Technical and Scope Constraints

### Mandatory Stack and Package Boundaries

The implementation MUST use Go 1.26+, Gin, Cobra, `gopkg.in/yaml.v3`, GORM,
`github.com/glebarez/sqlite`, Eino core/Eino-ext, the official MCP Go SDK,
`robfig/cron/v3`, and `log/slog` JSON logging.

The dependency direction MUST remain:

```text
cmd -> app -> handler/channel/scheduler -> service/runtime
runtime -> Eino core interfaces + internal domain ports
provider/tool-mcp/store -> concrete external libraries
```

`internal/runtime` MUST NOT import Gin, Cobra, GORM, or Eino-ext. `internal/web` MUST
NOT access concrete Provider connectors or Store implementations directly. Circular package
dependencies are prohibited.

### Fixed Core Contract

The core stage MUST contain exactly:

- 2 Providers: DeepSeek and MiniMax;
- 9 built-in Tools;
- 10 REST endpoints under `/api/v1`;
- 12 CLI leaf commands;
- 3 SQLite business tables;
- 2 acceptance Demos;
- 5 workspace subdirectories and 6 initialization files;
- 4 weeks at 3 hours per week, totaling 12 hours.

The nine built-in Tools are `read_file`, `write_file`, `list_dir`, `shell`,
`http_get`, `http_post`, `save_memory`, `recall_memory`, and `notify`.

The three tables are `sessions`, `tool_invocations`, and `llm_calls`. Scheduler,
Memory, Profile, and configuration MUST NOT introduce additional core tables.

The two Demos are Daily Weather and Daily Tech Digest. Both MUST support scheduled and
manual triggering through the same AgentService path. Daily Weather validates built-in
HTTP and notification Tools. Daily Tech Digest validates SKILL.md + MCP and use of user
preferences from MEMORY.md.

### Runtime Workspace and Configuration

`oryxos init` MUST create `profiles/`, `sessions/`, `skills/`, `logs/`, and
`memory/` plus `profiles/default.yaml`, `memory/MEMORY.md`,
`mcp_servers.yaml`, `AGENTS.md`, `SOUL.md`, and `USER.md`. The database MUST
reside at `.oryxos/sessions/oryxos.db`.

`oryxos init` MUST be idempotent and non-destructive. It MUST create missing artifacts,
MUST NOT overwrite existing targets, and MUST report each target as created or skipped.

Profile MUST remain the only runtime configuration source. Its top-level fields are
`name`, `description`, `identity`, `provider`, `tools`, `skills`,
`mcp_servers`, `notify_channels`, `schedules`, `channels`, `bootstrap`,
and `settings`. Provider fields are `name`, `model`, `api_key`, optional
`base_url`, and optional `temperature`.

`ProfileRuntime` MUST be constructed as an immutable startup snapshot. Core-stage
configuration changes MUST take effect after restart; file watching and hot reload MUST
remain outside the core stage.

Users MUST configure the initial API key and model by editing
`.oryxos/profiles/default.yaml`. `ConfigLoader` MUST expand `${ENV_VAR}`,
strictly reject invalid or unknown critical fields, fail fast on missing references, and
redact API keys, webhook URLs, and MCP credentials.

Stateful Session lookup MUST use `channel + user_id + profile.name`. Stateless invoke MUST
use `channel=http_invoke` with a unique `request_id` as `user_id`. Scheduler MUST use
`channel=scheduler` and `schedule.id` as `user_id`. The Session lookup index MUST be the
non-unique composite `(channel, user_id, profile_name, status)` and active lookup MUST
filter `status=active`.

### Extension Boundary

The following capabilities MUST NOT be treated as core-stage deliverables: Provider
fallback, multi-IM Channels, dedicated IM notification adapters, complete SKILL.md standard
support, Skills Marketplace, vector or episodic Memory, OryxOS as an MCP server, task
management APIs, scheduled-task state/history tables, SSE, WebSocket, OpenAPI endpoint,
Prometheus endpoint, authentication, RBAC, SSO, multi-tenancy, complete audit reports,
cluster high availability, and client SDKs.

## Development Workflow and Quality Gates

### Specification and Planning

Every feature MUST begin by checking `docs/DemandAnalysis.md` for scope and acceptance
criteria, then `docs/TechnicalSolution.md` for architecture. Specs, plans, and tasks MUST
cite the relevant constraints and MUST NOT invent a second source of truth.

Any intentional change to core counts, Profile fields, Provider boundaries, storage schema,
or phase ownership MUST update the demand document first and include a constitution
amendment when it changes a non-negotiable principle.

### Implementation and Review

Changes MUST preserve package boundaries and use Go naming conventions. Source changes MUST
include focused tests for the behavior being introduced or changed. Reviews MUST explicitly
check:

- Provider factory selection and per-Profile model isolation;
- ReAct behavior with no Tool, one Tool, multiple rounds, cancellation, and iteration limits;
- complete assistant Tool Call messages and matching Tool result messages;
- Profile and environment-variable validation;
- Sandbox path, redirect, command, timeout, and output limits;
- Tool retry idempotency rules and notify channel selection;
- Session identity, archive behavior, scheduler isolation, and overlap skipping;
- exact API, CLI, Tool, table, Demo, and workspace counts.

### Delivery Gates

Every Go change MUST be formatted. Before declaring implementation complete, the following
commands MUST pass from the repository root:

```bash
go test ./...
go vet ./...
CGO_ENABLED=0 go build ./cmd/oryxos
```

Release verification MUST benchmark the current `docs/DemandAnalysis.md` targets for
single-node Agent capacity, concurrent Sessions, Session-creation P99, and internal
forwarding overhead. LLM latency MUST be excluded from internal forwarding measurements.

The CGO-disabled release artifact MUST be smoke-tested in a clean Linux environment,
including startup, three-table migration, and basic SQLite read/write recovery.

Changed Go files MUST pass `gofmt`. Provider integrations MUST include DeepSeek and MiniMax
Tool Calling regressions. MCP integration MUST cover at least one stdio and one remote
connection. SQLite integration MUST cover migrations, restart recovery, and concurrent
Session writes. Web integration MUST exercise all 10 core endpoints.

If `go.mod` does not yet exist, only documentation consistency can be verified. No agent or
reviewer may claim that code builds or tests pass until the commands above have actually
run successfully.

### Documentation Synchronization

A requirement change MUST update `docs/DemandAnalysis.md` first. An architecture change MUST
update `docs/TechnicalSolution.md`. `AGENTS.md` and this constitution MUST then be
synchronized. Demo names MUST remain “每日天气” and “每日科技日报” in all artifacts.

## Governance

This constitution governs OryxOS engineering decisions, specifications, plans, tasks,
implementation, and review. It MUST NOT invent product scope that conflicts with
`docs/DemandAnalysis.md`; such a conflict requires amending the demand document and then
this constitution before implementation.

Amendments require:

1. an explicit description of the changed principle or governance rule;
2. user approval;
3. a Sync Impact Report at the top of this file;
4. a semantic version change;
5. a migration or compatibility plan for backward-incompatible governance changes;
6. synchronization of affected project guidance.

Versioning follows semantic versioning:

- MAJOR: removal or incompatible redefinition of a principle or governance contract;
- MINOR: addition of a principle/section or material expansion of mandatory guidance;
- PATCH: clarification, typo correction, or non-semantic refinement.

Every specification, plan, task list, and code review MUST include a constitution compliance
check. Deviations MUST be documented and explicitly approved; silent exceptions are
prohibited. `AGENTS.md` provides operational implementation guidance, while
`docs/TechnicalSolution.md` provides architecture details.

**Version**: 1.1.0 | **Ratified**: 2026-08-15 | **Last Amended**: 2026-08-15
