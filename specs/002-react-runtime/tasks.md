# Tasks: ReAct Runtime Orchestration

**Input**: Design documents from `/specs/002-react-runtime/`

**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`, `contracts/`,
`quickstart.md`

**Tests**: Every behavior task is test-first. Each test task must be run and observed failing for the
stated missing behavior before its paired implementation task begins. Existing assertions, bounds,
and predecessor tests must not be weakened or skipped.

**Organization**: Tasks are grouped by user story and preserve the lesson 17 delivery boundary.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel because it changes different files and has no incomplete dependency.
- **[Story]**: Maps the task to User Story 1, 2, or 3 from `spec.md`.

## Phase 1: Setup and Predecessor Gate

**Purpose**: Reconfirm lesson 16 contracts and the approved architecture before source changes.

- [X] T001 Run the clean baseline gates and verify lesson 16 `ProviderService.Chat`, OryxOS `llm` types, Profile registry, and `llm_calls` migration remain present using `go.mod`, `internal/llm/model.go`, `internal/provider/service.go`, `internal/profile/registry.go`, and `internal/store/migrations/001_llm_calls.sql`
- [X] T002 Verify locally that GORM `v1.31.2` exposes the repository APIs used by the plan and confirm no new dependency is needed, using `go.mod`, `go.sum`, and the local module cache before editing `internal/store/`

---

## Phase 2: Foundational Runtime Data and Audit

**Purpose**: Add the bounded Profile settings, minimal Session message aggregate, and Tool audit
storage required by all stories without implementing lesson 18 persistence or lesson 20 Registry.

**Critical**: No user-story implementation starts until this phase is green.

- [X] T003 [P] Write and run failing tests for omitted settings defaulting to 10/20 and explicit non-positive settings being isolated as invalid Profiles in `internal/profile/profile_loader_test.go`
- [X] T004 Implement lesson 17 runtime-setting defaults and positive-range validation without changing existing Profile fields or lesson 16 behavior in `internal/profile/loader.go` and `internal/profile/profile.go`
- [X] T005 [P] Write and run failing message-aggregate tests for serialized concurrent append/read behavior, race-safe defensive copies, complete user-rooted turn selection, and Tool-call/result group preservation in `internal/session/session_test.go`
- [X] T006 Implement the per-Session mutex, defensive message cloning, and in-memory message helpers required by lesson 17, leaving identity, store, archive, and lookup behavior for lesson 18, in `internal/session/session.go`
- [X] T007 [P] Write and run failing migration/repository tests for exact columns, `session_id` index, successful and failed rows, validation, and coexistence with `llm_calls` in `internal/store/tool_invocation_repository_test.go`
- [X] T008 Add the exact `ToolInvocation` GORM model and context-aware repository with sanitized failure validation in `internal/store/tool_invocation.go` and `internal/store/tool_invocation_repository.go`
- [X] T009 Add the idempotent hand-maintained table/index migration and execute it after lesson 16's migration without `AutoMigrate` in `internal/store/migrations/002_tool_invocations.sql` and `internal/store/sqlite.go`
- [X] T010 Run `go test ./internal/profile ./internal/session ./internal/store` and keep all lesson 16 Profile and audit regressions green

**Checkpoint**: Bounded settings, the in-memory conversation aggregate, and the second approved
audit table are available; Session persistence and Tool catalogs remain absent.

---

## Phase 3: User Story 1 - Complete a Tool-Assisted Request (Priority: P1) MVP

**Goal**: Execute deterministic reason-act-observe cycles, preserve complete Tool messages, and stop
correctly with one final Tool audit per logical call.

**Independent Test**: A deterministic model asks for zero, one, multiple, or endless Tool calls. The
runtime returns the expected final result or iteration error, preserves exact message correlation,
and invokes each Tool serially once unless an explicitly eligible retry applies.

### Tests for User Story 1

- [X] T011 [US1] Write and run failing executor tests for exact lookup, Profile allow-list rejection, malformed/schema-invalid arguments rejected before invocation, and matching Tool result identity in `internal/tool/executor_test.go`
- [X] T012 [US1] Extend and run the failing executor tests for per-attempt timeout, explicitly retryable idempotent calls capped at three retries, non-idempotent calls never retried, and exactly one redacted final audit outcome in `internal/tool/executor_test.go`
- [X] T013 [US1] Write and run failing ReAct tests for a one-call final response, one Tool round, multiple serial Tool calls, complete assistant/Tool message accumulation, cancellation, propagated failures, and exact max-iteration termination in `internal/runtime/react_loop_test.go`

### Implementation for User Story 1

- [X] T014 [US1] Add the documented OryxOS `InvokableTool` and `OryxTool` contracts plus package-private lookup/retry error seams, with no Eino imports and no ToolRegistry implementation, in `internal/tool/types.go`
- [X] T015 [US1] Implement fail-closed standard-library validation for the approved JSON object-schema subset in `internal/tool/schema.go`
- [X] T016 [US1] Implement `ToolExecutor` lookup, Profile filtering, timeout, eligible bounded retry, invocation, error joining/redaction, and exactly-one final recording in `internal/tool/executor.go`
- [X] T017 [US1] Implement `ReActLoop` with one user append, audited model calls through the lesson 16 Chat seam, complete assistant append, serial Tool dispatch, matching Tool messages, cancellation/error propagation, and `ErrMaxIterations` in `internal/runtime/react_loop.go`
- [X] T018 [US1] Run `go test ./internal/tool ./internal/runtime ./internal/provider ./internal/store` and confirm the P1 harness plus all lesson 16 Provider regressions are green

**Checkpoint**: The MVP loop completes deterministic Tool-assisted work without a real network,
parallel Tool execution, ADK, ToolRegistry, built-in Tool, MCP, or concrete Sandbox implementation.

---

## Phase 4: User Story 2 - Build Trustworthy Agent Context (Priority: P2)

**Goal**: Supply immutable, provenance-labelled, bounded context and Profile-specific Tool metadata.

**Independent Test**: Temporary workspace content and a fixed clock produce exact section order,
boundaries, whole-turn truncation, Memory cap, current time, and Profile Tool selection without a
real model.

### Tests for User Story 2

- [X] T019 [P] [US2] Write and run failing Bootstrap tests for default `AGENTS.md`/`SOUL.md`/`USER.md` order, explicit override order, empty missing defaults, explicit missing-reference failure, path confinement, copied output, and no hot reload in `internal/bootstrap/loader_test.go`
- [X] T020 [P] [US2] Write and run failing Skill tests for ordered Profile-only relative `SKILL.md` file references, explicit missing-reference failure, duplicate rejection, path confinement, copied output, no directory auto-discovery, and no Tool registration in `internal/skill/loader_test.go`
- [X] T021 [US2] Write and run failing prompt tests for exact precedence/labels, Profile identity plus persona, 4,000-character Memory cap, newest complete 20-turn default, custom history limit, intact Tool-call groups, current request retention, Profile-only Tool metadata, and fixed date/time at the end in `internal/runtime/prompt_builder_test.go`

### Implementation for User Story 2

- [X] T022 [P] [US2] Implement immutable workspace-confined Bootstrap loading with the documented defaults and explicit-reference behavior in `internal/bootstrap/loader.go`
- [X] T023 [P] [US2] Implement immutable Profile-referenced Skill loading and deterministic labelled sections without Tool registration in `internal/skill/loader.go`
- [X] T024 [US2] Implement `PromptBuilder` with narrow Memory and Tool-definition seams, exact labelled precedence, whole-turn truncation, Memory cap, and injected clock in `internal/runtime/prompt_builder.go`
- [X] T025 [US2] Run `go test ./internal/bootstrap ./internal/skill ./internal/runtime ./internal/profile` and confirm all P2 and predecessor tests are green

**Checkpoint**: Context is stable for an invocation and cannot be changed by file edits until a new
startup snapshot is built; Memory and Session remain service-backed mutable inputs.

---

## Phase 5: User Story 3 - Invoke One Shared Runtime Safely (Priority: P3)

**Goal**: Provide the one shared `AgentService.Invoke` boundary required by future CLI, Web, and
Scheduler callers while preserving Profile isolation and failed Session state.

**Independent Test**: CLI-, Web-, and Scheduler-shaped requests use fakes for the Session seam and
loop; every request resolves an explicit Profile, saves once after success or failure, and never
constructs its own Session identity.

### Tests for User Story 3

- [X] T026 [US3] Write and run failing tests for request validation, missing Profile, three caller shapes through one Invoke path, Profile isolation, explicit request forwarding to the Session seam, save after success/failure, cancellation-detached save, and joined run/save failures in `internal/runtime/agent_service_test.go`

### Implementation for User Story 3

- [X] T027 [US3] Add the documented `AgentRequest`, `AgentResponse`, and `AgentService` contract without adding entry-point commands or handlers in `internal/runtime/types.go`
- [X] T028 [US3] Implement `AgentService.Invoke` over the existing Profile registry, lesson 18 Session collaboration seam, and ReAct loop, without concatenating Session IDs or hiding Profile in context values, in `internal/runtime/agent_service.go`
- [X] T029 [US3] Run `go test ./internal/runtime ./internal/session ./internal/profile` and confirm all P3, P2, and P1 runtime tests are green

**Checkpoint**: Later CLI, Web, and Scheduler packages can share one invocation contract; lesson 18
still owns Session identity and persistence implementation.

---

## Phase 6: Cross-Cutting Harness and Delivery Gates

**Purpose**: Prove lesson completeness, protect predecessor contracts, and record deferred manual
checks without expanding scope.

- [X] T030 [P] Add or extend architecture assertions that reject Eino imports outside `internal/provider`, ADK imports, forbidden runtime framework/storage imports, and production `AutoMigrate` or CGO SQLite use in `internal/app/architecture_test.go`
- [X] T031 [P] Compare every production file, harness file, migration, default value, and manual check against the lesson 17 delivery list and record any discrepancy in `specs/002-react-runtime/quickstart.md`
- [X] T032 Format every changed Go file and run focused tests for each changed package plus `go test -race ./internal/session` using the commands in `specs/002-react-runtime/quickstart.md`
- [X] T033 Run `go test ./...`, `go vet ./...`, `CGO_ENABLED=0 go build ./cmd/oryxos`, and `git diff --check` from the repository root and retain exact output for the delivery report
- [X] T034 Run grep-based invariant checks for Eino boundary violations, automatic Agent/Tool execution, skipped tests, credential literals, `AutoMigrate`, forbidden extra tables, and speculative endpoint/CLI additions across `internal/`, `cmd/`, and `go.mod`
- [X] T035 Record the unexecuted real DeepSeek weather flow and MiniMax/OpenAI Tool-call compatibility checks as deferred dependency-bound manual validation in `specs/002-react-runtime/quickstart.md`

### Post-delivery reliability amendment

- [X] T036 Add failing Provider and ReAct tests proving that a successful model response survives
  `llm_calls` persistence failure while all Tool calls in that response remain unexecuted
- [X] T037 Add the marked `AuditPersistenceError` Provider contract and consume its narrow
  response-availability behavior in `ReActLoop` without importing `internal/provider`
- [X] T038 Document the fail-closed response-retention behavior and the deferred audit-store health
  circuit-breaker design, then rerun all delivery gates

---

## Dependencies & Execution Order

### Phase dependencies

- Phase 1 has no implementation dependency.
- Phase 2 depends on Phase 1 and blocks all user stories.
- User Story 1, User Story 2, and the fake-driven portion of User Story 3 can begin after Phase 2.
- User Story 3 production wiring depends on the ReAct behavior from User Story 1.
- Phase 6 depends on all selected user stories.

### User story dependencies

- **User Story 1 (P1)**: Depends only on Phase 2; this is the MVP.
- **User Story 2 (P2)**: Depends only on Phase 2 and can be developed beside P1 because it changes
  loader and prompt files rather than executor/loop files, except for coordinated runtime tests.
- **User Story 3 (P3)**: Tests can start after Phase 2 with a fake loop; final implementation and
  full validation depend on P1's ReAct loop.

### Test-first order inside each story

1. Add the smallest meaningful failing test.
2. Run it and verify the failure is caused by the missing target behavior.
3. Add the minimum implementation.
4. Run the target package tests immediately and restore green before continuing.
5. Never accumulate unrelated failing tests or alter predecessor assertions.

### Parallel opportunities

- T003, T005, and T007 change separate packages and can run in parallel after T002.
- T019 and T020 can run in parallel; T022 and T023 can follow in parallel.
- T011/T012, T013, and T021 touch separate production areas but their test helpers must not create
  conflicting public contracts.
- T030 and T031 can run in parallel after implementation is complete.

---

## Parallel Example: User Story 2

```text
Task T019: Bootstrap loader tests in internal/bootstrap/loader_test.go
Task T020: Skill loader tests in internal/skill/loader_test.go

After both expected failures are observed:
Task T022: Bootstrap loader in internal/bootstrap/loader.go
Task T023: Skill loader in internal/skill/loader.go
```

---

## Implementation Strategy

### MVP first

1. Complete the predecessor and foundational gates.
2. Complete User Story 1 through T018.
3. Stop and run the independent P1 harness.
4. Add User Story 2 context assembly.
5. Add User Story 3 shared invocation.
6. Run all cross-cutting delivery gates.

### Scope protections

- Do not implement Session ID generation, SQLite Session persistence, archive behavior, CLI chat,
  Web handlers, or Scheduler execution; those belong to later lessons.
- Do not implement `ToolRegistry`, any of the nine built-in Tools, MCP clients/adapters, or Tool-list
  CLI behavior; lesson 20 owns them.
- Do not implement the concrete Sandbox; retain only the approved per-operation dependency seam for
  lesson 24.
- Do not add streaming output, SSE, WebSocket, parallel Tool execution, Agent delegation, summary
  compression, hot reload, fallback, endpoints, commands, tables, or dependencies.
- Do not commit, push, package, or publish as part of implementation.

## Task Summary

- Total tasks: 38
- Setup/predecessor: 2
- Foundational: 8
- User Story 1: 8
- User Story 2: 7
- User Story 3: 4
- Cross-cutting delivery and reliability amendment: 9
- Suggested MVP: Phases 1-3 through T018

## Phase 7: Convergence

- [X] T039 [US1] CRITICAL add failing tests and implement one structured log event for every physical Tool execution attempt, including attempt number, final-attempt status, session ID, Tool name, duration, and sanitized error, per Constitution VI and plan decision 11 (missing)
- [X] T040 [US1] Add a failing second-iteration ReAct prompt test, then keep the current user-rooted turn exactly once while preserving its assistant Tool call and Tool result observations per FR-002, FR-006, and FR-012 (contradicts)
- [X] T041 [US2] Add failing context-budget tests and implement a PromptBuilder-local budget seam that drops only the oldest complete historical turns while preserving system context and the complete current turn per FR-013 and TechnicalSolution 4.4 (missing)
- [X] T042 [P] [US2] Replace conditional symlink skips with deterministic hard assertions in Bootstrap and Skill confinement harnesses, and make the quickstart invariant scan match the executable tests per the lesson delivery gate (contradicts)
- [X] T043 [P] [US1] Strengthen focused tests for real Tool timeout/cancellation propagation and Profile-only Tool schema binding without adding ToolRegistry, concrete Tool, Sandbox, or entry-point scope per SC-005 and the lesson 17 harness (partial)
- [X] T044 [P] Synchronize the optimization-one error contract and convergence behavior into plan/quickstart, and create `docs/testing/react-runtime-manual-testing.md` with dependency gates, exact commands, expected message/audit evidence, failure criteria, evidence template, and cleanup while keeping real Tool/Sandbox execution deferred to lessons 20 and 24 (partial)
- [X] T045 Run all focused harnesses, race detection, architecture checks, `go test ./...`, `go vet ./...`, `CGO_ENABLED=0 go build ./cmd/oryxos`, and `git diff --check`; then record only actually executed checks and leave dependency-bound manual checks explicitly unpassed (partial)

### Effective summary after convergence

- Original tasks: 38
- Appended convergence tasks: 7
- Effective total: 45
- Completed: 45
