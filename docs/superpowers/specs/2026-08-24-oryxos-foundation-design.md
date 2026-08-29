# OryxOS Go Foundation Design

## Status

Approved in conversation on 2026-08-24. This design initializes the bounded Go foundation defined
by the repository's `oryxos-init` Skill. It does not implement the OryxOS business runtime.

## Governing Sources

Apply these sources in descending priority:

1. `docs/DemandAnalysis.md` for product scope and stage boundaries.
2. `docs/TechnicalSolution.md` for architecture and implementation direction.
3. `AGENTS.md` for engineering constraints.
4. `.agents/skills/oryxos-init/SKILL.md` and every reference routed by
   `.agents/skills/oryxos-init/references/foundation-modules.md` for the foundation contract.

Existing files describe repository state but do not override these sources. No Git operation is
permitted during this initialization.

## Preflight

| Parameter | Value |
| --- | --- |
| Go module | `github.com/Karlsk/oryxos-go` |
| Target executable | `./cmd/oryxos` |
| Service and binary name | `oryxos` |
| Default listen address | `127.0.0.1:8080` |
| Required Go version | Go 1.26+ |
| Observed local Go version | `go1.26.6 darwin/arm64` |

The repository has no `go.mod` or Go source. Foundation code, tests, quality automation, and the
two development documents are therefore classified `absent -> create`. Existing requirements,
technical documentation, skills, Spec-Kit files, IDE metadata, and `.gitignore` are outside the
candidate change set. No candidate is `identical`, `compatible`, or `conflict` at design time.
The implementation must reclassify every exact candidate immediately before editing it.

## Scope

The initialization creates the complete package skeleton from `project-layout.md`. It provides
real behavior only for:

- process-level server configuration;
- secret redaction and structured console/JSON logging;
- in-process readiness and HTTP observations;
- the generic HTTP response and pagination contracts;
- request-ID, request-size, access-observation, and recovery middleware;
- the bounded Gin server and `GET /api/v1/health` plus `GET /api/v1/info`;
- application assembly, signal handling, startup, and graceful reverse shutdown;
- the exact 12-leaf Cobra command tree;
- functional, non-destructive `init`, truthful `status`, and bounded `serve` commands;
- repository development guidance, linting, pre-commit checks, CI, and quality gates.

The other nine CLI leaves return a stable nonzero `not implemented` error. Each of the fourteen
business packages contains only its exact documented `doc.go` placeholder.

## Deferred Capabilities

The foundation does not implement business Agents, Profiles, Providers, ReAct, Tools, Memory,
Session persistence, MCP, Scheduler behavior, demos, or the remaining eight core REST endpoints.
It also excludes authentication, RBAC, multi-tenancy, SSE, WebSockets, Provider fallback,
transparent HTTP retries, circuit breakers, Prometheus integration or `/metrics`, OpenAPI or
Swagger, and all other extension-stage behavior.

HTTP client timeouts and pooling are permitted only as a consumer-neutral construction boundary.
One caller attempt performs one `Do` call. The later `ToolExecutor` owns policy-aware retries,
and may retry only an explicitly retryable operation that is idempotent or carries a reliable
idempotency key. Circuit breaking, Prometheus, and OpenAPI require later independent designs.

## Architecture and Implementation Order

Implementation follows the mandatory dependency order:

```text
config -> observability -> API contract -> middleware -> HTTP server
       -> lifecycle -> Cobra commands -> quality gates
```

The dependency direction remains:

```text
cmd -> app -> handler/channel/scheduler -> service/runtime
runtime -> Eino core interfaces + internal domain ports
provider/tool-mcp/store -> concrete external libraries
```

The foundation does not manufacture placeholder services to fill future dependency slots.
`internal/app` wires only dependencies that genuinely exist.

## Per-Phase Deviation Gate

Every implementation phase has a mandatory gate before work starts and another before the next
phase may begin. For each phase, perform these steps in order:

1. Re-read `.agents/skills/oryxos-init/SKILL.md`, the applicable row in
   `references/foundation-modules.md`, and the complete detailed reference for that phase.
2. Reclassify every exact candidate file as `absent`, `identical`, `compatible`, or `conflict`;
   stop for approval on a conflict.
3. Compare the implementation against that reference's scope, public contracts, dependency
   direction, required tests, explicit exclusions, and security rules.
4. Run the phase's focused named tests and any routed static inventory checks. Record the exact
   command and outcome; a failed or unavailable required check does not count as passing.
5. Reinspect changed files for business-capability leakage and fixed-count drift before marking
   the phase complete.

The phase gate fails on missing evidence, a test failure, an API or contract change, a forbidden
dependency or endpoint, invented business behavior, or any mismatch with a higher-priority source.
When a gate fails, stop that phase, diagnose the cause, and do not start the next phase until the
current implementation conforms or the user approves an explicit governing-source change.

## Components

### Command edge

`cmd/oryxos` owns Cobra construction and deliberate user-facing output. It registers exactly:

```text
init
status
chat [--profile <name>]
serve
gateway
profile list
profile create <name>
profile show <name>
profile delete <name>
provider list
tool list
session list
```

`profile`, `provider`, `tool`, and `session` are grouping commands, not leaves.

### Configuration

`internal/config` loads process-level server YAML only. It expands exact environment placeholders
once, strictly decodes one YAML document, applies defaults only to omitted fields, validates the
listen address and every positive duration, and returns stable field-path errors. It rejects
Profile keys so process configuration cannot become a second business configuration source.

The package also owns the shared sensitive-key and error-string sanitizers. A recognized secret is
fully replaced with `[REDACTED]`; partial masking is not used.

### Observability

`internal/observability` supplies standard-library `slog` JSON and colored console handlers. Both
are wrapped by a recursive redacting handler. Correlation values are immutable context data under
the exact keys `request_id`, `session_id`, `profile_name`, `channel`, and `schedule_id`; unknown
values are omitted.

The concurrency-safe in-process Observer tracks readiness and HTTP count plus total duration by
method, matched route template, and status. It has no exporter, global registry, or HTTP endpoint.

### API contract

`internal/web/api` is the only HTTP JSON response writer. It implements the exact generic
`Result[T]` and `PageResult[T]` structures and the `Success`, `Page`, and `Error` responder API.
Invalid descriptors, page metadata, or detail maps produce one fixed `500/internal` envelope.
Pagination is one-based, bounded, overflow-safe, and serializes empty items as `[]`.

### HTTP boundary

`internal/web/middleware` installs middleware in this order:

```text
RequestID -> RequestBodyLimit -> AccessObservation -> Recovery -> handler
```

Request IDs reuse only valid 1-128 character ASCII values or use a cryptographically random opaque
replacement. The body limit is one MiB. Access observation reads the final request context after
handler execution. Recovery maps uncommitted panics to the shared safe internal-error envelope.

`internal/web` uses `gin.New()`, registers exactly the health and info routes, provides Result
envelopes for 404 and 405, and copies all four validated nonzero timeouts into `http.Server`.
Health reflects Observer readiness; info exposes only version, foundation mode, and readiness.

### Application lifecycle

`internal/app` constructs configuration, logger, Observer, middleware/API dependencies, and the
HTTP server synchronously. It starts components in actual dependency order and sets readiness only
after successful startup. `Application.Run` owns SIGINT/SIGTERM handling and waits for cancellation
or a terminal component error.

Shutdown sets readiness false, uses a new positive timeout context derived from
`context.Background`, closes successfully started components in reverse order, waits for every
owned goroutine, and joins triggering and cleanup errors. `http.ErrServerClosed` caused by normal
shutdown is not treated as a terminal failure.

### Workspace initialization

`oryxos init` creates `.oryxos` with exactly five directories and six initial files:

```text
profiles/  sessions/  skills/  logs/  memory/
AGENTS.md  SOUL.md  USER.md  profiles/default.yaml
memory/MEMORY.md  mcp_servers.yaml
```

It reports every target as `created` or `skipped`, never overwrites an existing target, does not
create the database, and does not create `agents/` or a runtime `tools/` directory.

## Error and Safety Rules

- Expected failures return wrapped errors; panic is not expected control flow.
- All public HTTP responses have a real status, stable code/message, safe details, and matching
  body/header/log request IDs.
- Request and response bodies, headers, cookies, raw paths, query strings, credentials, expanded
  configuration, and credential-bearing URLs never enter logs or observations.
- No code outside `internal/web/api` directly serializes an HTTP JSON response.
- Context flows from CLI/HTTP through lifecycle calls and is never stored on long-lived structs.
- Every goroutine has an owner, cancellation source, terminal-error path, and close-time wait path.
- The foundation has no transparent retry loop, circuit breaker, detached worker, or global
  mutable registry.
- `init` is idempotent and non-destructive; existing user-authored runtime files are preserved.

## Testing Strategy

Implementation uses package-level TDD in the mandatory phase order. Each phase begins with the
exact named tests in `references/test-contracts.md`, observes their intended failure, adds the
smallest compliant implementation, and reruns the focused tests before advancing.

Tests use `httptest`, temporary directories, injected environment lookups, listeners, contexts,
clocks, and fake components. They do not call a real Provider, MCP server, external URL, or OS
signal, and they synchronize with channels or barriers rather than sleeps.

Acceptance proves:

- strict configuration defaults, validation, one-pass expansion, and redaction;
- JSON/console logging, recursive redaction, correlation, readiness, aggregation, and race safety;
- exact Result/PageResult shapes, request IDs, UTC timestamps, safe details, and overflow behavior;
- middleware order, final panic status observation, body limiting, and safe recovery;
- exact health/info route inventory and absence of all forbidden routes and imports;
- graceful startup, reverse shutdown, joined errors, shutdown deadlines, and goroutine ownership;
- exactly 12 CLI leaves, exact functional/deferred behavior, and init idempotence;
- the required import-family boundaries and pure placeholders;
- no Prometheus, OpenAPI, transparent retry, circuit-breaker, Eino ADK, or CGO SQLite surface.

## Quality and Automation

Create and maintain:

- `docs/DevelopmentGuide.md` and `docs/ApiConvention.md`;
- `.editorconfig`, `.golangci.yml`, and `.pre-commit-config.yaml`;
- `Makefile` with exactly `fmt`, `lint`, `test`, `test-race`, `security`, `build`, and `verify` as
  public targets;
- `.github/workflows/verify.yml` and `.github/pull_request_template.md`.

The Go module declares Go 1.26 and pins the required Go-distributed tools at
`golang.org/x/tools v0.49.0`, `golang.org/x/vuln v1.7.0`, and
`github.com/securego/gosec/v2 v2.28.0`, using Go 1.26 tool directives. The lint workflow uses
`golangci/golangci-lint-action@v9.3.0` with binary `v2.12.2`. Runtime libraries are selected from
their current official stable releases at implementation time, locked exactly in `go.mod`, and
recorded in the development guide. No tool is installed globally and no floating version is used.

## Verification and Completion

After formatting every changed Go file, run and record the exact outcome of:

```text
go mod tidy -diff
gofmt/goimports non-mutating checks
go vet ./...
golangci-lint run
go test ./...
go test -race ./...
go tool gosec ./cmd/... ./internal/...
go tool govulncheck ./cmd/... ./internal/...
CGO_ENABLED=0 go build -o <temporary-directory>/oryxos ./cmd/oryxos
```

The implementation is complete only when the current source proves every mandatory contract and
the final scope review finds no deferred behavior. An unavailable prerequisite or failed command
is reported exactly and prevents a claim of full completion. No Git operation, global tool
installation, generated repository binary, or automatic publication is part of this work.
