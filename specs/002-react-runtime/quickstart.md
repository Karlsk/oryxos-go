# Quickstart: ReAct Runtime Orchestration

## Prerequisites

- Go 1.26+
- Lesson 16 Provider implementation and tests present
- No real Provider, MCP, Webhook, or weather credentials are needed for default verification

## Test-first implementation order

1. Add Profile runtime-setting default/rejection tests and implement only those parser changes.
2. Add the Session message aggregate tests needed to preserve Tool-call groups.
3. Add Bootstrap and Skill loader failure/order tests, then implement immutable loaders.
4. Add PromptBuilder order, whole-turn/history-budget truncation, current-turn de-duplication,
   Memory-cap, and clock tests, then implement it.
5. Add Tool executor validation, real timeout, retry, per-attempt structured log, ordering, and audit
   tests, then implement it.
6. Add ReAct loop final/Tool/max-iteration/cancellation tests, then implement it.
7. Add AgentService shared-entry/save-on-failure tests, then implement it.
8. Add migration/repository tests, then the hand SQL and repository.

Each test must first fail for the intended missing behavior. Do not weaken an existing lesson 16
assertion to make the new work pass.

## Focused harness

```bash
go test ./internal/profile
go test -race ./internal/session
go test ./internal/bootstrap ./internal/skill
go test ./internal/tool ./internal/store
go test ./internal/runtime
```

Expected results:

- Non-converging model doubles stop after exactly the configured call count.
- Complete assistant Tool calls and matching Tool messages remain in order.
- Multiple Tool calls execute serially.
- Prompt sections, Memory cap, whole-turn history, current time, and Profile Tool selection match the
  contract.
- A second ReAct model request contains the current user-rooted turn exactly once; input-budget
  pressure removes only the oldest complete historical turns.
- Every physical Tool attempt has one sanitized structured log; one logical Tool call still has
  exactly one final `tool_invocations` row.
- Success and failure Tool calls each produce one final audit row.
- A successful model response whose `llm_calls` write fails is retained in Session, while its Tool
  calls remain unexecuted and the audit failure is returned.
- Shared invocation saves accumulated Session state after success and failure.

## Repository gates

```bash
gofmt -w <each changed Go file>
go test ./...
go test -race ./internal/session
go vet ./...
CGO_ENABLED=0 go build ./cmd/oryxos
git diff --check
```

Also verify boundary invariants:

```bash
rg 'github.com/cloudwego/eino' internal --glob '*.go' --glob '!**/*_test.go'
rg 'AutoMigrate|mattn/go-sqlite3' internal --glob '*.go' --glob '!**/*_test.go'
rg 't\.Skip' internal/bootstrap internal/skill internal/runtime internal/tool --glob '*_test.go'
```

Expected boundary result: Eino imports appear only under `internal/provider`; production migration
code contains no `AutoMigrate` or CGO SQLite driver; the deterministic lesson 17 harness has no
conditional skip. The separately tagged real-Provider smoke test may still skip a Provider whose
credential is absent.

## Manual checks deferred by dependencies

After lessons 20 and 24 provide the built-in HTTP Tool, Registry, and Sandbox:

1. Configure a real DeepSeek Profile and ask for weather-based clothing advice.
2. Confirm one `http_get` Tool call, its matching Tool message, and the final recommendation.
3. Repeat through MiniMax/OpenAI compatibility and inspect Tool-call ID preservation.
4. Confirm `llm_calls` and `tool_invocations` contain the corresponding successful or failed rows.

These are manual checks and must not be reported as passing during lesson 17 implementation alone.

The executable steps, expected evidence, failure criteria, and cleanup procedure are expanded in
[`docs/testing/react-runtime-manual-testing.md`](../../docs/testing/react-runtime-manual-testing.md).

## Delivery comparison

The implementation was compared against the lesson 17 delivery list on 2026-09-19. All planned
production files, focused harness files, the `002_tool_invocations.sql` migration, Profile defaults,
and shared invocation contracts are present. The convergence pass additionally fixed current-turn
duplication, added whole-turn input-budget enforcement, added per-physical-attempt Tool logs,
strengthened actual timeout and Profile Tool-selection assertions, removed conditional skips from
the deterministic loader harness, and documented the marked audit-persistence failure behavior.
Tool schema binding remains intentionally owned by the lesson 16 audited `ProviderService`;
`PromptBuilder` receives only the selected Profile and does not introduce a second Tool catalog or
Eino boundary.

The real DeepSeek weather flow and MiniMax/OpenAI Tool-call compatibility flow were not executed.
They remain dependency-bound manual validation because the built-in HTTP Tool, ToolRegistry, and
concrete Sandbox are assigned to lessons 20 and 24.

## Verification record

Executed from the repository root on 2026-09-19:

- Focused package tests: all passed.
- `go test -race ./internal/session` and `go test -race ./internal/runtime ./internal/tool`: passed.
- `go test ./...`: all packages passed; packages without tests reported `[no test files]`.
- `go vet ./...`: exited successfully with no findings.
- `CGO_ENABLED=0 go build ./cmd/oryxos`: exited successfully with no output.
- `git diff --check`: exited successfully with no whitespace errors.
- Invariant scans: Eino imports occur only in `internal/provider` production code; no production
  ADK import, `AutoMigrate`, CGO SQLite driver, deterministic lesson 17 skipped test, new endpoint,
  new CLI command, or third migration was found. Credential-pattern matches were confined to
  redaction test fixtures.

The convergence run included the new prompt-budget/current-turn, physical-attempt log, real timeout,
Profile Tool-selection, audit fail-closed, and correlation assertions. The full test suite required
running outside the restricted sandbox because existing CLI/Web tests bind a loopback listener; it
then passed. Dependency-bound real Provider/Tool checks remain explicitly unpassed.
