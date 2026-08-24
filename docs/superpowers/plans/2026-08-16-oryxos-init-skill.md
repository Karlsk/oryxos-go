# OryxOS Init Skill Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development
> (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create a reusable `oryxos-init` Skill that directly installs the OryxOS Go engineering
foundation while preserving core-stage architecture and non-destructive behavior.

**Architecture:** `SKILL.md` orchestrates the workflow, focused references define subsystem
contracts, and a read-only Bash verifier checks initialized targets. The Skill creates the complete
package skeleton but implements only configuration, observability, lifecycle, HTTP/API, and CLI
foundations.

**Tech Stack:** Markdown, Bash, Go 1.26+, Cobra, Gin, `log/slog`, gofmt, goimports, go vet,
golangci-lint v2, gosec, govulncheck, pre-commit, GitHub Actions.

## Global Constraints

- Read the confirmed design, DemandAnalysis, TechnicalSolution, AGENTS, and constitution first.
- Use `skill-creator`, `superpowers:writing-skills`, and its required TDD background.
- Run a failing fresh-agent baseline before creating the Skill, then rerun it with the Skill.
- Create only `.agents/skills/oryxos-init/`; do not modify application source.
- Generated projects target Go 1.26+ and `CGO_ENABLED=0` release builds.
- Never overwrite files, initialize Git, or commit.
- Initialize only `/api/v1/health` and `/api/v1/info`.
- Prometheus/OpenAPI endpoints, transparent HTTP retries, and circuit breakers stay out of scope.
- Use `Result[T]` and `Result[PageResult[T]]`; create exactly 12 CLI leaf commands.
- Keep `SKILL.md` concise, aim for at most 500 words, and route details to one-level references.
- Add a table of contents to any reference longer than 100 lines.

## Target Files

```text
.agents/skills/oryxos-init/
├── SKILL.md
├── agents/
│   └── openai.yaml
├── references/
│   ├── project-layout.md
│   ├── foundation-modules.md
│   ├── configuration.md
│   ├── application-lifecycle.md
│   ├── observability.md
│   ├── http-server.md
│   ├── http-client-boundary.md
│   ├── api-contract.md
│   ├── test-contracts.md
│   ├── go-quality.md
│   └── ci-verification.md
└── scripts/
    └── verify.sh
```

---

### Task 1: Skill Entry Point

**Files:**
- Create: `.agents/skills/oryxos-init/SKILL.md`
- Create: `.agents/skills/oryxos-init/agents/openai.yaml`

**Interfaces:**
- Consumes: governance documents and all reference paths.
- Produces: triggers, preflight, routing, phase gates, conflict policy, and final report.

- [ ] **Step 1: Read authoring Skills**

Read `skill-creator/SKILL.md`, `superpowers:writing-skills/SKILL.md`, its required
`superpowers:test-driven-development` background, and skill-creator's `openai_yaml.md` completely.

- [ ] **Step 2: Run the RED baseline without the Skill**

Use fresh agents that cannot read the proposed Skill. Give them realistic empty-repository and
half-initialized-repository requests with combined time pressure, requests to overwrite conflicts,
and requests to add Prometheus/OpenAPI and auto-commit. Record the exact scope violations,
omissions, or rationalizations. The baseline must demonstrate at least one failure before authoring.

- [ ] **Step 3: Initialize the standard Skill package**

Run:

```bash
python3 /Users/gao/.codex/skills/.system/skill-creator/scripts/init_skill.py \
  oryxos-init \
  --path .agents/skills \
  --resources scripts,references \
  --interface display_name="OryxOS Init" \
  --interface short_description="Initialize the OryxOS Go engineering foundation" \
  --interface \
  default_prompt='Use $oryxos-init to initialize or incrementally complete the OryxOS Go foundation.'
```

Expected: `SKILL.md`, `agents/openai.yaml`, `references/`, and `scripts/` are created. Do not use
`--examples`.

- [ ] **Step 4: Replace the generated frontmatter**

```yaml
---
name: oryxos-init
description: >-
  Use when initializing or incrementally completing the OryxOS Go repository, especially
  for requests involving a Go project scaffold, foundation modules, structured logging,
  Gin HTTP/API conventions, CLI skeleton, CI, linting, or security checks.
---
```

- [ ] **Step 5: Write required workflow sections**

Use headings `Overview`, `Scope and Non-Goals`, `Sources of Truth`, `Preflight Parameters`,
`Non-Destructive Change Policy`, `Execution Workflow`, `Reference Routing`, `Phase Gates`,
`Quick Reference`, `Common Mistakes`, and `Final Report`. Keep all trigger conditions in the
frontmatter description. Require module path, target, service name, and port when not inferable.

Define:

```text
absent -> create
identical -> skip
compatible -> precise patch
conflict -> stop for approval
```

Prohibit Git operations, business features, extension endpoints, and unverified success claims.

- [ ] **Step 6: Validate the entry point and UI metadata**

```bash
test -s .agents/skills/oryxos-init/SKILL.md
test -s .agents/skills/oryxos-init/agents/openai.yaml
rg -n '^name: oryxos-init$|^  Use when' .agents/skills/oryxos-init/SKILL.md
rg -n '^## ' .agents/skills/oryxos-init/SKILL.md
rg -n 'display_name: "OryxOS Init"|default_prompt: "Use \$oryxos-init' \
  .agents/skills/oryxos-init/agents/openai.yaml
```

Expected: trigger-only metadata, all body sections, and synchronized UI metadata.

- [ ] **Step 7: Checkpoint**

Suggest `feat(skills): add oryxos init workflow entry point`; do not run Git.

---

### Task 2: Layout and Foundation Routing

**Files:**
- Create: `.agents/skills/oryxos-init/references/project-layout.md`
- Create: `.agents/skills/oryxos-init/references/foundation-modules.md`

**Interfaces:**
- Consumes: TechnicalSolution section 11.
- Produces: exact tree, placeholder rules, dependency direction, CLI tree, and phase order.

- [ ] **Step 1: Write `project-layout.md`**

Include every package from the design. Give each unimplemented package a distinct compile-safe
`doc.go` and forbid speculative interfaces and fake results. Use this pattern:

```go
// Package provider contains model-provider factories and adapters.
//
// The oryxos-init Skill creates this package as a compile-safe placeholder.
// Provider behavior is implemented by a later feature specification.
package provider
```

Include:

```text
cmd -> app -> handler/channel/scheduler -> service/runtime
runtime -> Eino core interfaces + internal domain ports
provider/tool-mcp/store -> concrete external libraries
```

List all 12 commands. `init`, `status`, and `serve` are functional; the other nine return stable
`not implemented` errors with non-zero status.

- [ ] **Step 2: Write `foundation-modules.md`**

```text
config -> observability -> API contract -> middleware -> HTTP server
       -> lifecycle -> Cobra commands -> quality gates
```

Route every subsystem to one detailed reference and state that this file is an index.

- [ ] **Step 3: Validate**

```bash
rg -n 'internal/(app|config|profile|provider|runtime|memory|session|sandbox|scheduler|store)' \
  .agents/skills/oryxos-init/references/project-layout.md
rg -n 'oryxos (init|status|chat|serve|gateway)' \
  .agents/skills/oryxos-init/references/project-layout.md
rg -n 'oryxos (profile list|profile create|profile show|profile delete|provider list|tool list|session list)' \
  .agents/skills/oryxos-init/references/project-layout.md
```

Expected: complete package families and 12 commands.

- [ ] **Step 4: Checkpoint**

Suggest `docs(skills): define oryxos Go project skeleton`.

---

### Task 3: Configuration, Lifecycle, and Observability

**Files:**
- Create: `.agents/skills/oryxos-init/references/configuration.md`
- Create: `.agents/skills/oryxos-init/references/application-lifecycle.md`
- Create: `.agents/skills/oryxos-init/references/observability.md`

**Interfaces:**
- Consumes: Task 2 ownership.
- Produces: contracts consumed by app, HTTP, and tests.

- [ ] **Step 1: Write `configuration.md`**

Require typed server config, `${ENV_VAR}` expansion, non-zero HTTP/shutdown timeouts, field-path
errors, strict YAML parsing, and secret redaction. Require table tests for defaults, invalid
durations, missing variables, and redaction. Do not redefine Profile business fields.

- [ ] **Step 2: Write `application-lifecycle.md`**

Specify construction order, signals, `context.Context`, bounded graceful shutdown, reverse cleanup,
and joined errors. Forbid panic for expected errors and goroutines without ownership/cancellation.

- [ ] **Step 3: Write `observability.md`**

Require `log/slog` JSON with:

```text
request_id session_id profile_name channel schedule_id
```

Define internal HTTP duration/status and readiness metrics without Prometheus. Require tests proving
secrets do not enter logs.

- [ ] **Step 4: Validate**

```bash
rg -n 'timeout|context.Context|graceful|reverse|log/slog|request_id|schedule_id|Prometheus|redact' \
  .agents/skills/oryxos-init/references/{configuration,application-lifecycle,observability}.md
```

Expected: all cross-cutting contracts.

- [ ] **Step 5: Checkpoint**

Suggest `docs(skills): define OryxOS lifecycle contracts`.

---

### Task 4: HTTP and Frontend API Contracts

**Files:**
- Create: `.agents/skills/oryxos-init/references/http-server.md`
- Create: `.agents/skills/oryxos-init/references/http-client-boundary.md`
- Create: `.agents/skills/oryxos-init/references/api-contract.md`

**Interfaces:**
- Consumes: Task 3 contracts.
- Produces: server behavior, client boundaries, and JSON schemas.

- [ ] **Step 1: Write `http-server.md`**

Require Gin, all four `http.Server` timeouts, graceful shutdown, request ID, recovery, JSON access
logs, and request observation. Register only health/info. Require tests proving metrics, OpenAPI,
Swagger, SSE, and WebSocket routes are absent.

- [ ] **Step 2: Write `http-client-boundary.md`**

Allow shared Transport pooling/timeouts and require caller context. Prohibit provider/tool clients
and transparent retries. ToolExecutor owns retry; eligible retries require retryable and
idempotent/idempotency-key conditions, exponential backoff, and at most three retries. Circuit
breakers remain extension-stage.

- [ ] **Step 3: Write exact `api-contract.md` types**

```go
type Result[T any] struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Data      *T             `json:"data,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
	RequestID string         `json:"request_id"`
}

type PageResult[T any] struct {
	Items      []T   `json:"items"`
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	Total      int64 `json:"total"`
	TotalPages int64 `json:"total_pages"`
}
```

Define `api.Success`, `api.Page`, and `api.Error`; real HTTP status codes; stable string codes;
one-based bounded paging; RFC 3339 UTC; empty `items: []`; request ID in header/body/log; and no
direct `gin.Context.JSON` outside the responder.

- [ ] **Step 4: Validate**

```bash
rg -n 'Result\[T any\]|PageResult\[T any\]|items.*\[\]|request_id|gin.Context.JSON' \
  .agents/skills/oryxos-init/references/api-contract.md
rg -n 'transparent retr|at most three retries|circuit breaker' \
  .agents/skills/oryxos-init/references/http-client-boundary.md
```

Expected: schemas and boundaries.

- [ ] **Step 5: Checkpoint**

Suggest `docs(skills): define OryxOS HTTP and API contracts`.

---

### Task 5: Go Quality, CI, and Tests

**Files:**
- Create: `.agents/skills/oryxos-init/references/go-quality.md`
- Create: `.agents/skills/oryxos-init/references/ci-verification.md`
- Create: `.agents/skills/oryxos-init/references/test-contracts.md`

**Interfaces:**
- Consumes: Tasks 2-4.
- Produces: project standards, tool gates, and exact test matrix.

- [ ] **Step 1: Write `go-quality.md`**

Require generated `DevelopmentGuide.md`, `ApiConvention.md`, `.editorconfig`, and PR template.
Use Effective Go, Google Go Style, and Code Review Comments. Require gofmt/goimports,
consumer-owned interfaces, `%w`, context-first signatures, owned goroutines, bounded concurrency,
Go doc, and table tests.

Pin goimports, golangci-lint v2, gosec, and govulncheck. No floating `@latest`. Use Go tool
directives where suitable and the official pinned golangci-lint binary/Action.

- [ ] **Step 2: Write `ci-verification.md`**

Require Make targets `fmt`, `lint`, `test`, `test-race`, `security`, `build`, and `verify`.
Define pre-commit and CI around:

```bash
go mod tidy -diff
go vet ./...
go test ./...
go test -race ./...
golangci-lint run
go tool gosec ./...
go tool govulncheck ./...
CGO_ENABLED=0 go build -o "$tmpdir/oryxos" ./cmd/oryxos
```

- [ ] **Step 3: Write `test-contracts.md`**

Give exact test names, inputs, and assertions for Config, Observability, Result, PageResult,
Middleware, Server, forbidden endpoints, CLI, init idempotency, architecture imports, and builds.
Assert exactly 12 leaves, only health/info, `items: []`, and no forbidden runtime imports.

- [ ] **Step 4: Validate**

```bash
rg -n 'Effective Go|Google Go Style|Code Review Comments|golangci-lint|gosec|govulncheck' \
  .agents/skills/oryxos-init/references/go-quality.md
rg -n 'go test -race|CGO_ENABLED=0|pull_request_template' \
  .agents/skills/oryxos-init/references/ci-verification.md
rg -n 'Config|Observability|PageResult|Middleware|12|health/info|Architecture' \
  .agents/skills/oryxos-init/references/test-contracts.md
```

- [ ] **Step 5: Checkpoint**

Suggest `docs(skills): define Go quality and security gates`.

---

### Task 6: Read-Only Verifier and Skill Evaluation

**Files:**
- Create: `.agents/skills/oryxos-init/scripts/verify.sh`
- Modify if necessary: files under `.agents/skills/oryxos-init/`

**Interfaces:**
- Consumes: initialized target root as argument 1.
- Produces: exit 0 with concise PASS output or non-zero named failures.

- [ ] **Step 1: Implement strict setup**

```bash
#!/usr/bin/env bash
set -euo pipefail

target_root="${1:-.}"
target_root="$(cd "$target_root" && pwd)"
tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT
```

Add `require_file`, `require_dir`, `require_pattern`, and `reject_pattern`. Check layout, quality
files, forbidden routes, likely secrets, forbidden runtime imports, `mattn/go-sqlite3`, and direct
Gin JSON outside the responder.

Mark the completed script executable with `chmod 0755`; do not perform write operations against
the target project.

- [ ] **Step 2: Add read-only gates**

```bash
test -z "$(gofmt -l .)"
test -z "$(go tool goimports -l .)"
go mod tidy -diff
go vet ./...
go test ./...
go test -race ./...
golangci-lint run
go tool gosec ./...
go tool govulncheck ./...
CGO_ENABLED=0 go build -o "$tmpdir/oryxos" ./cmd/oryxos
```

Never write formatting, run plain `go mod tidy`, or build into the target. Missing tools report
project-local installation commands and are not installed globally.

- [ ] **Step 3: Test shell and failure behavior**

```bash
bash -n .agents/skills/oryxos-init/scripts/verify.sh
fixture="$(mktemp -d)"
if .agents/skills/oryxos-init/scripts/verify.sh "$fixture"; then
  echo "expected empty target verification to fail" >&2
  exit 1
fi
rm -rf "$fixture"
```

Expected: syntax passes; empty target fails with missing `go.mod`.

- [ ] **Step 4: Validate inventory, links, and placeholders**

Expect `SKILL.md`, `agents/openai.yaml`, eleven references, and `verify.sh`. Check every routed
path exists. Reject unresolved template markers and incomplete instructions. Every
Prometheus/OpenAPI mention must explicitly prohibit core registration or identify an extension
boundary.

- [ ] **Step 5: Run GREEN and REFACTOR skill evaluations**

Give fresh agents the Skill path and rerun the same RED prompts, then test:

1. empty repository;
2. compatible half-initialized repository;
3. conflicting module or responder;
4. request for core Prometheus/OpenAPI;
5. request for automatic Git initialization/commit.

Expected: cases 1-2 proceed, case 3 pauses safely, cases 4-5 preserve scope, and no business
feature is implemented. Record any new rationalization, make the smallest instruction change that
closes it, and rerun the affected scenario until it passes.

- [ ] **Step 6: Run the official Skill validator**

```bash
python3 /Users/gao/.codex/skills/.system/skill-creator/scripts/quick_validate.py \
  .agents/skills/oryxos-init
```

Expected: validation succeeds with no frontmatter, naming, or structure errors.

- [ ] **Step 7: Final coverage review**

Map design sections 4-13 to concrete files. Fix missing contracts, inconsistent type names, broken
paths, duplicate ownership, and unsupported completion claims.

- [ ] **Step 8: Delivery checkpoint**

Report created files and commands actually run. Suggest:

```text
feat(skills): add Go OryxOS project initialization skill
```

Do not run Git.
