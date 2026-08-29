# OryxOS Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development
> (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build and verify the truthful Go 1.26 OryxOS foundation with exact workspace
initialization, 12 Cobra leaves, and only the health/info HTTP surface.

**Architecture:** Implement the routed foundation in the mandatory order: configuration,
observability, API contract, middleware, HTTP server, lifecycle, Cobra, then quality gates. Each
task closes with a deviation gate that re-reads its `oryxos-init` reference and blocks the next
task on any scope, contract, test, dependency, or fixed-count mismatch.

**Tech Stack:** Go 1.26, Gin v1.12.0, Cobra v1.10.2, `gopkg.in/yaml.v3` v3.0.1,
standard-library `log/slog` and `net/http`, golangci-lint v2.12.2, goimports v0.49.0,
gosec v2.28.0, and govulncheck v1.7.0.

**Spec:** `docs/superpowers/specs/2026-08-24-oryxos-foundation-design.md`

## Global Constraints

- Module path is exactly `github.com/Karlsk/oryxos-go`; target is `./cmd/oryxos`; service name is
  `oryxos`; default listen address is `127.0.0.1:8080`.
- Use Go 1.26+ and keep the release binary CGO-free.
- Register exactly 12 Cobra leaves; only `init`, `status`, and `serve` are functional.
- Register exactly `GET /api/v1/health` and `GET /api/v1/info` during foundation initialization.
- Keep all fourteen business packages as the exact import-free `doc.go` placeholders from
  `project-layout.md`.
- Do not add Prometheus, OpenAPI, Swagger, transparent HTTP retry, circuit breakers, Provider
  fallback, Agent behavior, business endpoints, authentication, SSE, or WebSockets.
- Do not install tools globally, perform Git operations, write a repository binary, or suppress a
  failed check. Writing-plans normally suggests commits, but the invoked `oryxos-init` Skill
  explicitly prohibits Git operations, so this plan contains no commit steps.
- Use `apply_patch` for source and configuration edits; formatting tools may perform mechanical
  rewrites.

## File Map

| Path | Responsibility |
| --- | --- |
| `go.mod`, `go.sum` | Exact module, Go version, runtime dependencies, and pinned tool directives. |
| `internal/config/*.go` | Server config, one-pass expansion, strict YAML, validation, and redaction. |
| `internal/observability/*.go` | JSON/console slog, recursive redaction, correlation, and Observer. |
| `internal/web/api/*.go` | Result/PageResult, responder validation, request IDs, time and pagination. |
| `internal/web/middleware/*.go` | Request ID, body limit, access observation, and panic recovery. |
| `internal/web/*.go` | Two-route Gin router and owned HTTP serving component. |
| `internal/app/*.go` | Foundation assembly, lifecycle, signal seam, reverse shutdown, error joining. |
| `cmd/oryxos/*.go` | Main, exact Cobra tree, workspace init/status, and serve command. |
| Fourteen business `doc.go` files | Exact compile-safe placeholders, with no imports or behavior. |
| `docs/DevelopmentGuide.md`, `docs/ApiConvention.md` | Local workflow and public API conventions. |
| `.editorconfig`, `.golangci.yml`, `.pre-commit-config.yaml`, `Makefile` | Local quality automation. |
| `.github/workflows/verify.yml`, `.github/pull_request_template.md` | Pinned CI and review evidence. |

---

### Task 1: Module and Honest Package Skeleton

**References:** `.agents/skills/oryxos-init/SKILL.md`,
`.agents/skills/oryxos-init/references/foundation-modules.md`, and
`.agents/skills/oryxos-init/references/project-layout.md`.

**Files:**
- Create: `go.mod`
- Create: `internal/app/doc.go`
- Create: `internal/app/architecture_test.go`
- Create: the fourteen exact placeholder files under `internal/profile`, `skill`, `bootstrap`,
  `provider`, `runtime`, `memory`, `session`, `tool`, `tool/builtin`, `tool/mcp`, `sandbox`,
  `scheduler`, `channel/cli`, and `store`

**Interfaces:**
- Produces module imports rooted at `github.com/Karlsk/oryxos-go`.
- Produces the exact package paths consumed by every following task.

- [ ] **Step 1: Run the pre-task deviation gate**

Re-read the three references above. Confirm `go.mod` and every listed source target is still
`absent`; if any exists, classify it before editing. Record the candidate classification in the
execution notes.

- [ ] **Step 2: Create the module declaration and foundation test package**

Create `go.mod` with this direct dependency/tool block; `go mod tidy` may add indirect
requirements but must preserve these exact direct pins:

```go
module github.com/Karlsk/oryxos-go

go 1.26.0

require (
	github.com/gin-gonic/gin v1.12.0
	github.com/securego/gosec/v2 v2.28.0
	github.com/spf13/cobra v1.10.2
	golang.org/x/tools v0.49.0
	golang.org/x/vuln v1.7.0
	gopkg.in/yaml.v3 v3.0.1
)

tool (
	github.com/securego/gosec/v2/cmd/gosec
	golang.org/x/tools/cmd/goimports
	golang.org/x/vuln/cmd/govulncheck
)
```

Create `internal/app/doc.go` with a truthful package comment, without claiming runtime behavior.

- [ ] **Step 3: Write the failing exact-placeholder test**

In `internal/app/architecture_test.go`, define a table of all fourteen paths and complete expected
file bytes copied verbatim from `project-layout.md`. Resolve the repository root from the test
file, read each path, and compare exact bytes:

```go
func TestArchitectureBusinessPlaceholdersExact(t *testing.T) {
	for _, tc := range placeholderCases {
		t.Run(tc.path, func(t *testing.T) {
			got, err := os.ReadFile(filepath.Join(repositoryRoot(t), tc.path))
			if err != nil {
				t.Fatalf("read placeholder: %v", err)
			}
			if string(got) != tc.want {
				t.Fatalf("placeholder mismatch\nwant:\n%s\ngot:\n%s", tc.want, got)
			}
		})
	}
}
```

- [ ] **Step 4: Run the test and observe the intended failure**

Run: `go test ./internal/app -run TestArchitectureBusinessPlaceholdersExact -count=1`

Expected: FAIL because the first required business placeholder does not exist.

- [ ] **Step 5: Create all fourteen exact placeholders**

Copy each documented package comment and declaration verbatim. Each file contains only its comment
and `package` declaration; it has no imports, type, constructor, variable, unfinished marker, or fake
return value.

- [ ] **Step 6: Verify the skeleton**

Run:

```bash
go test ./internal/app -run TestArchitectureBusinessPlaceholdersExact -count=1
go test ./...
rg -n 'internal/(app|config|profile|provider|runtime|memory|session|sandbox|scheduler|store)' .agents/skills/oryxos-init/references/project-layout.md
```

Expected: both Go commands PASS; the reference inventory command finds every documented package.

- [ ] **Step 7: Run the post-task deviation gate**

Re-read the placeholder and dependency sections of `project-layout.md`. Confirm exactly fourteen
business placeholders exist, placeholder imports are absent, no `.oryxos` workspace was created,
and no business behavior or deferred dependency entered the module.

---

### Task 2: Strict Process-Level Server Configuration

**References:** `.agents/skills/oryxos-init/SKILL.md`, the configuration row in
`foundation-modules.md`, `configuration.md`, and the configuration cases in `test-contracts.md`.

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/load.go`
- Create: `internal/config/expand.go`
- Create: `internal/config/redact.go`
- Create: `internal/config/config_test.go`
- Create: `internal/config/redact_test.go`

**Interfaces:**
- Produces: `ServerConfig`, `LogFormat`, `LoadServerYAML([]byte, func(string) (string, bool))`.
- Produces: `RedactValue(key string, value any) any`,
  `SanitizeErrorString(text string) string`, and `IsSensitiveKey(path []string, key string) bool`.
- Consumed by: observability, HTTP construction, application assembly, and Cobra serve.

- [ ] **Step 1: Run the pre-task deviation gate**

Re-read `configuration.md` completely. Confirm all six candidate files are absent and that this
package will contain no Profile, Provider, Tool, Skill, MCP, schedule, or credential model.

- [ ] **Step 2: Write the exact configuration contract tests**

Implement all exact test names from `test-contracts.md`:

```text
TestLoadServerYAMLDefaults
TestLoadServerYAMLPartialDefaults
TestLoadServerYAMLExpansion
TestLoadServerYAMLMissingVariable
TestLoadServerYAMLInvalidDuration
TestLoadServerYAMLZeroOrNegativeTimeouts
TestLoadServerYAMLStrictUnknownFields
TestLoadServerYAMLInvalidAddress
TestLoadServerYAMLLogFormat
TestLoadServerYAMLRedactsSecrets
```

Use table rows for every `0s` and `-1s` timeout, duplicate keys, trailing YAML documents,
malformed placeholders, and set-but-empty expansion. Assert exact defaults and prove secret
literals are absent from errors.

- [ ] **Step 3: Run the tests and observe the intended compile failure**

Run: `go test ./internal/config -count=1`

Expected: FAIL to compile because `ServerConfig`, `LogFormat`, and `LoadServerYAML` are undefined.

- [ ] **Step 4: Implement runtime types and exact defaults**

Define the public types exactly:

```go
type ServerConfig struct {
	ListenAddress     string
	LogFormat         LogFormat
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
}

type LogFormat string

const (
	LogFormatConsole LogFormat = "console"
	LogFormatJSON    LogFormat = "json"
)
```

Keep raw YAML structs unexported with string duration fields so omission and explicit zero remain
distinguishable.

- [ ] **Step 5: Implement one-pass scalar expansion and strict decoding**

Decode exactly one document into a `yaml.Node`, reject duplicate mapping keys, recursively visit
scalar values while carrying their YAML field path, and expand exact `${NAME}` tokens once. Do not
expand environment-returned text recursively. Re-encode the expanded node and decode it through
`yaml.Decoder.KnownFields(true)` into the raw struct; require EOF after the one strict document.

Use the exact name grammar `[A-Za-z_][A-Za-z0-9_]*`. Missing variables return a sanitized error
that names the variable and containing path but no YAML bytes or expanded value.

- [ ] **Step 6: Implement defaulting and validation**

Apply defaults only when fields were omitted:

```text
listen_address=127.0.0.1:8080
log_format=console
read_header_timeout=5s
read_timeout=30s
write_timeout=5m
idle_timeout=60s
shutdown_timeout=30s
```

Validate host/port through `net.SplitHostPort`, port range 1-65535, exact log formats, and every
duration as a positive `time.ParseDuration` result. Every error begins with `config <field_path>:`.

- [ ] **Step 7: Implement shared redaction**

Treat keys case-insensitively. Redact exact sensitive names and keys containing `apikey`,
`credential`, `mcp_auth`, or `webhook`. Return `[REDACTED]` for error strings containing a
sensitive key/value pattern or credential-bearing URL; retain only strings proven not to contain
those forms.

- [ ] **Step 8: Run focused and package tests**

Run:

```bash
gofmt -w internal/config/*.go
go test ./internal/config -count=1
go vet ./internal/config
```

Expected: all commands PASS.

- [ ] **Step 9: Run the post-task deviation gate**

Compare every public type, default, validation row, error path, and secret rule against
`configuration.md`. Run `rg -n 'provider|identity|tools|skills|mcp_servers|notify_channels|schedules' internal/config` and confirm matches occur only in rejection tests or explanatory comments, never
as accepted server fields.

---

### Task 3: Correlated Structured Observability

**References:** `.agents/skills/oryxos-init/SKILL.md`, the observability row in
`foundation-modules.md`, `observability.md`, and its cases in `test-contracts.md`.

**Files:**
- Create: `internal/observability/logger.go`
- Create: `internal/observability/console_handler.go`
- Create: `internal/observability/redacting_handler.go`
- Create: `internal/observability/correlation.go`
- Create: `internal/observability/observer.go`
- Create: `internal/observability/observability_test.go`

**Interfaces:**
- Consumes: `config.LogFormat`, `config.RedactValue`, `config.SanitizeErrorString`.
- Produces: `NewLogger`, `NewConsoleLogger`, `NewRedactingHandler`, `WithCorrelation`,
  `CorrelationFromContext`, `Logger`, `Observer`, `Snapshot`, and `NewObserver`.
- Consumed by: API responder, middleware, HTTP handlers, lifecycle, and commands.

- [ ] **Step 1: Run the pre-task deviation gate**

Re-read `observability.md` completely. Confirm package candidates are absent and that no
Prometheus dependency, exporter, route, global logger, or metric label API will be added.

- [ ] **Step 2: Write the exact observability tests**

Implement every exact test name from `test-contracts.md`, including console/JSON redaction,
pre-attached and grouped attributes, route templates, readiness transitions, and barrier-based
concurrent access. Parse JSON logs structurally and check every captured byte sequence for secret
absence.

```text
TestLoggerJSONAndCorrelation
TestLoggerPartialCorrelation
TestObserverHTTPDurationAndStatus
TestObserverUsesRouteTemplate
TestObserverReadiness
TestLoggerSanitizesError
TestLoggerSanitizesDirectAndAttachedAttributes
TestLoggerSanitizesGroupedAttributes
TestLoggerConsoleFormatAndCorrelation
TestLoggerConsoleModeRedacts
TestObserverConcurrentAccess
TestNoPrometheusSurface
```

- [ ] **Step 3: Run the tests and observe the intended compile failure**

Run: `go test ./internal/observability -count=1`

Expected: FAIL to compile because logger, correlation, and Observer APIs are undefined.

- [ ] **Step 4: Implement correlation and logger constructors**

Define:

```go
type Correlation struct {
	RequestID   string
	SessionID   string
	ProfileName string
	Channel     string
	ScheduleID  string
}

func WithCorrelation(ctx context.Context, c Correlation) context.Context
func CorrelationFromContext(ctx context.Context) Correlation
func Logger(ctx context.Context, base *slog.Logger) *slog.Logger
```

Use a private typed context key. `Logger` attaches only non-empty correlation fields under their
exact snake_case names.

- [ ] **Step 5: Implement recursive redacting handlers**

`NewLogger` wraps `slog.NewJSONHandler`; `NewConsoleLogger` wraps the standard-library-only console
handler. Both pass their sink to `NewRedactingHandler`. Preserve time, level, message, and PC in a
new record; sanitize record attributes, `WithAttrs`, active `WithGroup` paths, nested groups, and
all `error` attributes before delegating.

- [ ] **Step 6: Implement the in-process Observer**

Define the exact contract:

```go
type Observer interface {
	ObserveHTTP(ctx context.Context, method, route string, status int, duration time.Duration)
	SetReady(ready bool)
	Snapshot() Snapshot
}
```

Use a mutex-protected map keyed only by method, matched route template, and status. `Snapshot`
returns copied and deterministically sorted aggregates; initial readiness is false.

- [ ] **Step 7: Run focused, vet, and race tests**

Run:

```bash
gofmt -w internal/observability/*.go
go test ./internal/observability -count=1
go test -race ./internal/observability -count=1
go vet ./internal/observability
```

Expected: all commands PASS with no race.

- [ ] **Step 8: Run the post-task deviation gate**

Compare constructors, correlation fields, Handler method behavior, Observer cardinality, and each
sensitive-data exclusion against `observability.md`. Confirm `rg -n 'prometheus|/metrics' go.mod internal` finds no Prometheus import, registration, or route.

---
### Task 4: Canonical Result and Pagination API

**References:** `.agents/skills/oryxos-init/SKILL.md`, the API-contract row in
`foundation-modules.md`, `api-contract.md`, and its Result/PageResult cases in
`test-contracts.md`.

**Files:**
- Create: `internal/web/api/result.go`
- Create: `internal/web/api/pagination.go`
- Create: `internal/web/api/time.go`
- Create: `internal/web/api/result_test.go`
- Create: `internal/web/api/pagination_test.go`

**Interfaces:**
- Consumes: `observability.CorrelationFromContext` and `observability.WithCorrelation`.
- Produces: exact `Result[T]`, `PageResult[T]`, `Success`, `Page`, `Error`, and
  `FormatTimeUTC(time.Time) string`.
- Produces an internal pagination parser used by package tests and subsequent handlers.

- [ ] **Step 1: Run the pre-task deviation gate**

Re-read `api-contract.md` completely. Confirm all candidate files are absent and that no package
outside `internal/web/api` will write API JSON.

- [ ] **Step 2: Write the exact Result and PageResult tests**

Implement every exact Result/PageResult test from `test-contracts.md`. Use `gin.New()` test
handlers, decode envelope JSON, and assert exact descriptor validation, one-write fallback,
safe details, emergency request-ID propagation, UTC formatting, nil items as `[]`, strict query
bounds, duplicate rejection, and `math.MaxInt64` total-page arithmetic.

```text
TestResultSuccessEnvelope
TestResultErrorEnvelope
TestResultInvalidDescriptorFallback
TestResultInvalidNestedDetailsFallback
TestResultEmergencyRequestID
TestResultTimeUTC
TestPageResultEmptyItemsArray
TestPageResultDefaultsAndBounds
TestPageResultMaxInt64TotalPages
TestPageResultInvalidFallback
```

- [ ] **Step 3: Run tests and observe the intended compile failure**

Run: `go test ./internal/web/api -count=1`

Expected: FAIL to compile because the generic types and responders are undefined.

- [ ] **Step 4: Implement the exact generic contracts**

Copy these contracts verbatim:

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

- [ ] **Step 5: Implement validated responders and emergency request IDs**

Allow only the exact success and error descriptor table from `api-contract.md`. Validate details
as exactly `{field, rule}` with the documented field regex and rule allowlist. Before writing,
return when `c.Writer.Written()` is true. On invalid input write one direct fixed
`500/internal/internal server error` envelope without recursive validation.

Use 16 random bytes from `crypto/rand`, encoded as lowercase hex with a `req_` prefix, for an
emergency ID. Store it in a derived request context and set `X-Request-ID` before serializing.
Only the unexported writer invokes `c.JSON`.

- [ ] **Step 6: Implement canonical pagination and UTC formatting**

Normalize nil `Items` to an empty allocated slice. Ignore supplied `TotalPages` and calculate it
without `n+s-1` overflow. Parse `page` and `page_size` as one unsigned decimal value each, with
defaults 1/20 and bounds 1..10000 and 1..100. Reject duplicate, signed, decimal, overflow, empty,
zero, negative, and out-of-range values.

`FormatTimeUTC` returns `value.UTC().Format(time.RFC3339)`.

- [ ] **Step 7: Run focused tests and static response checks**

Run:

```bash
gofmt -w internal/web/api/*.go
go test ./internal/web/api -count=1
go vet ./internal/web/api
rg -n '\.JSON\(|AbortWithStatusJSON' internal --glob '*.go'
```

Expected: Go commands PASS; static output identifies direct JSON writing only in
`internal/web/api/result.go` and test fixtures.

- [ ] **Step 8: Run the post-task deviation gate**

Compare every field/tag, descriptor, safe-detail rule, request-ID path, pagination bound, and UTC
rule against `api-contract.md`. Confirm no bare response, framework recovery, HTML, SSE, or
alternate serialization path exists.

---

### Task 5: Request Boundary Middleware

**References:** `.agents/skills/oryxos-init/SKILL.md`, the middleware row in
`foundation-modules.md`, middleware sections of `http-server.md`, observability's HTTP boundary,
and middleware cases in `test-contracts.md`.

**Files:**
- Create: `internal/web/middleware/request_id.go`
- Create: `internal/web/middleware/body_limit.go`
- Create: `internal/web/middleware/access.go`
- Create: `internal/web/middleware/recovery.go`
- Create: `internal/web/middleware/middleware_test.go`

**Interfaces:**
- Consumes: API responders, Observer, correlation helpers, and base `*slog.Logger`.
- Produces: `RequestID() gin.HandlerFunc`, `RequestBodyLimit(limit int64) gin.HandlerFunc`,
  `AccessObservation(observer observability.Observer, logger *slog.Logger) gin.HandlerFunc`, and
  `Recovery(logger *slog.Logger) gin.HandlerFunc`.

- [ ] **Step 1: Run the pre-task deviation gate**

Re-read all middleware and request-ID sections. Confirm candidate files are absent and preserve
the registration order `RequestID`, `RequestBodyLimit`, `AccessObservation`, `Recovery`.

- [ ] **Step 2: Write exact middleware tests**

Implement the exact five middleware test names from `test-contracts.md`. Use real API responders,
an in-memory Observer, JSON logger, a panic fixture, and a decoder fixture that maps
`*http.MaxBytesError` to `413/payload_too_large`. Never use Gin default middleware.

```text
TestRequestIDReuseAndGeneration
TestMiddlewareOrderAndFinalObservation
TestRequestBodyLimit
TestRecoveryUsesSafeEnvelope
TestAccessObservationPostHandlerContext
```

- [ ] **Step 3: Run tests and observe the intended compile failure**

Run: `go test ./internal/web/middleware -count=1`

Expected: FAIL to compile because middleware constructors are undefined.

- [ ] **Step 4: Implement request IDs and body bounds**

Accept incoming `X-Request-ID` only when it is 1-128 bytes and every byte is ASCII alphanumeric,
dot, underscore, or hyphen. Otherwise generate the same opaque form used by the API emergency
path. Derive and install `observability.Correlation{RequestID: id}` before `c.Next()` and set the
response header first.

Wrap non-nil request bodies with `http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)`; the
constructor rejects nonpositive limits at wiring time rather than silently disabling the bound.

- [ ] **Step 5: Implement final-state access observation and recovery**

Access middleware records time immediately before `c.Next()`, then reads
`c.Request.Context()`, final status, and `c.FullPath()` afterward. Use the matched template; use
the fixed safe string `unmatched` when Gin reports no template. Log only method, route, status, and
duration_ms through the correlated logger.

Recovery defers around `c.Next()`, logs a sanitized `http.recovered_panic`, aborts, and calls the
shared `500/internal` responder only when the writer is not committed. It never includes panic
text or stack bytes in a response or log attribute.

- [ ] **Step 6: Run focused and race tests**

Run:

```bash
gofmt -w internal/web/middleware/*.go
go test ./internal/web/middleware -count=1
go test -race ./internal/web/middleware -count=1
go vet ./internal/web/middleware
```

Expected: all commands PASS and panic requests are observed as final 500s.

- [ ] **Step 7: Run the post-task deviation gate**

Reconfirm middleware order, one MiB limit, exact request-ID grammar, post-handler context read,
safe unmatched-route label, and absence of raw request path/query/header/body logging.

---

### Task 6: Bounded Gin Router and Owned HTTP Component

**References:** `.agents/skills/oryxos-init/SKILL.md`, the HTTP-server row in
`foundation-modules.md`, `http-server.md`, and server/forbidden-route cases in
`test-contracts.md`.

**Files:**
- Create: `internal/web/server.go`
- Create: `internal/web/handlers.go`
- Create: `internal/web/component.go`
- Create: `internal/web/server_test.go`
- Create: `internal/web/component_test.go`

**Interfaces:**
- Consumes: validated `config.ServerConfig`, Observer, base logger, API, and middleware.
- Produces: `NewServer`, `Server.Handler() http.Handler`, `Server.Routes() gin.RoutesInfo`,
  `Server.Start(context.Context) error`, `Server.Close(context.Context) error`, and
  `Server.Errors() <-chan error`.
- Produces: `ListenerFactory func(network, address string) (net.Listener, error)` as an injectable
  test seam; production uses `net.Listen`.

- [ ] **Step 1: Run the pre-task deviation gate**

Re-read `http-server.md` completely. Confirm candidate files are absent and write down the exact
two-route inventory and all forbidden paths before coding.

- [ ] **Step 2: Write exact server and route-boundary tests**

Implement exact tests for server timeouts, ready/not-ready health, info, unknown route, method not
allowed, route inventory, forbidden routes, request-ID consistency, panic recovery, and graceful
shutdown. Inject a pre-bound loopback listener and synchronize with a channel, not sleep.

```text
TestServerTimeouts
TestHealthReadyEnvelope
TestHealthNotReadyEnvelope
TestInfoEnvelope
TestUnknownRouteEnvelope
TestMethodNotAllowedEnvelope
TestServerGracefulShutdown
TestRouteInventory
TestForbiddenFoundationRoutesAbsent
```

- [ ] **Step 3: Run tests and observe the intended compile failure**

Run: `go test ./internal/web -count=1`

Expected: FAIL to compile because server constructors and component methods are undefined.

- [ ] **Step 4: Construct the exact router and handlers**

Use `gin.New()`, set `HandleMethodNotAllowed = true`, install middleware in the required order,
register one `/api/v1` group, and add only:

```go
v1.GET("/health", healthHandler(observer))
v1.GET("/info", infoHandler(observer, version))
```

Health returns `200/ok` with `{status:"ready"}` only when ready, otherwise `503/not_ready` with
no data. Info returns only version, `mode:"foundation"`, and ready. NoRoute and NoMethod use the
shared 404/405 responders.

- [ ] **Step 5: Construct the exact `http.Server` and owned serve loop**

Copy all four validated timeouts into `http.Server`. `Start` binds synchronously through the
listener factory, then launches one owned goroutine running `Serve`. Report only non-normal serve
errors to a buffered terminal channel and close a done channel when the goroutine exits.

`Close(ctx)` calls `Shutdown(ctx)`, calls `Close()` only when needed after shutdown failure or
deadline, and waits for the owned serve goroutine. Repeated Close calls are safe and never close a
channel twice.

- [ ] **Step 6: Run focused, race, route, and static checks**

Run:

```bash
gofmt -w internal/web/*.go
go test ./internal/web -count=1
go test -race ./internal/web -count=1
go vet ./internal/web
rg -n 'GET\("|POST\("|PUT\("|DELETE\("|PATCH\("|/metrics|openapi|swagger|websocket|sse' internal/web --glob '*.go'
```

Expected: tests and vet PASS; production route registration contains only health/info and no
forbidden surface.

- [ ] **Step 7: Run the post-task deviation gate**

Inspect `router.Routes()` output against exactly two entries. Reconfirm four nonzero timeouts,
safe Result envelopes for all known paths, one owned goroutine, no Gin default middleware, and no
business handler or external protocol.

---

### Task 7: Application Assembly and Lifecycle

**References:** `.agents/skills/oryxos-init/SKILL.md`, the lifecycle row in
`foundation-modules.md`, `application-lifecycle.md`, and lifecycle cases in `test-contracts.md`.

**Files:**
- Create: `internal/app/application.go`
- Create: `internal/app/foundation.go`
- Create: `internal/app/application_test.go`
- Modify: `internal/app/doc.go`

**Interfaces:**
- Consumes: config loader, logger constructors, Observer, and web Server component.
- Produces: `Component`, `Application`, `NewApplication`, `Application.Run`, and
  `NewFoundation(FoundationOptions) (*Application, error)`.
- Produces `FoundationOptions` seams for YAML bytes, environment lookup, log writer, version,
  listener factory, and a controlled signal-context factory used only by tests.

- [ ] **Step 1: Run the pre-task deviation gate**

Re-read `application-lifecycle.md` completely. Confirm exact candidates are absent/compatible and
that the foundation has no fake Profile, Provider, Store, Tool, Scheduler, or Agent component.

- [ ] **Step 2: Write exact lifecycle tests with fake components**

Implement every exact lifecycle test name from `test-contracts.md`. The fake records Start/Close
order, injected errors, deadline presence, and owned-worker exit. Use channels for all coordination
and `errors.Is` for wrapped/joined failures.

```text
TestApplicationNormalCancellation
TestApplicationStartFailure
TestApplicationServeFailure
TestApplicationJoinsCloseErrors
TestApplicationShutdownDeadline
TestApplicationOwnedGoroutineExits
TestApplicationServerClosedIsNormal
TestApplicationShutdownIsIdempotent
```

- [ ] **Step 3: Run tests and observe the intended compile failure**

Run: `go test ./internal/app -run 'TestApplication' -count=1`

Expected: FAIL to compile because Application lifecycle APIs are undefined.

- [ ] **Step 4: Implement lifecycle contracts and single shutdown**

Define:

```go
type Component interface {
	Start(context.Context) error
	Close(context.Context) error
}

type TerminalSource interface {
	Errors() <-chan error
}
```

`Run` obtains one signal-aware root context, starts components in registered order, and stops at
the first Start failure. After successful starts it sets readiness true and waits for root
cancellation or a terminal error. Treat cancellation as normal; preserve a terminal error.

Shutdown uses `sync.Once`, sets readiness false, creates
`context.WithTimeout(context.Background(), shutdownTimeout)`, closes only the successful prefix in
reverse order, and joins trigger, close, and deadline errors without dropping any sentinel.

- [ ] **Step 5: Implement synchronous foundation assembly**

`NewFoundation` performs exactly:

```text
LoadServerYAML -> mode-selected logger -> NewObserver -> web.NewServer -> NewApplication
```

Default missing seams to `os.LookupEnv`, `os.Stderr`, and `net.Listen` at this application edge.
Do not create an outbound HTTP client because the foundation has no consumer. Do not create
business placeholder services.

- [ ] **Step 6: Run focused, race, and vet checks**

Run:

```bash
gofmt -w internal/app/*.go
go test ./internal/app -count=1
go test -race ./internal/app -count=1
go vet ./internal/app
```

Expected: all commands PASS; every fake closes at most once and every worker exits.

- [ ] **Step 7: Run the post-task deviation gate**

Compare construction/start/close order, signal ownership, timeout context origin, terminal error
handling, `errors.Join`, and goroutine ownership to `application-lifecycle.md`. Confirm app imports
only real foundation packages and no deferred business adapter.

---

### Task 8: Exact Cobra Tree and Workspace Commands

**References:** `.agents/skills/oryxos-init/SKILL.md`, CLI and workspace sections of
`project-layout.md`, CLI/init cases in `test-contracts.md`, and Technical Solution sections 8.1,
8.2, 9.3, and 12.1.

**Files:**
- Create: `cmd/oryxos/main.go`
- Create: `cmd/oryxos/root.go`
- Create: `cmd/oryxos/commands.go`
- Create: `cmd/oryxos/workspace.go`
- Create: `cmd/oryxos/root_test.go`
- Create: `cmd/oryxos/workspace_test.go`
- Create: `cmd/oryxos/architecture_test.go`

**Interfaces:**
- Consumes: `app.NewFoundation` and `Application.Run`.
- Produces: `NewRootCommand(CommandDependencies) *cobra.Command` for deterministic tests.
- Produces: `InitializeWorkspace(root string, out io.Writer) error` and
  `WorkspaceStatus(root string) WorkspaceState`.

- [ ] **Step 1: Run the pre-task deviation gate**

Re-read the exact command tree and workspace contracts. Confirm every candidate is absent and
write down all 12 leaf use lines, five directories, six files, and nine deferred leaves.

- [ ] **Step 2: Write exact CLI, init, and architecture tests**

Implement:

```text
TestRootCommandHasExactlyTwelveLeaves
TestFoundationCLIBehavior
TestInitCreatesExactWorkspace
TestInitIsIdempotentAndNonDestructive
TestArchitectureForbiddenRuntimeImports
TestArchitectureDependencyFamiliesAtEdges
TestArchitectureNoForbiddenRuntimeImports
TestNoDirectJSONOutsideResponder
TestFoundationRoutesAreOnlyHealthInfo
```

Walk Cobra leaves and compare sorted use lines exactly. For serve, inject a pre-bound listener,
wait for readiness via channel, request health/info, cancel the command context, and require owned
goroutine exit before a test-owned deadline. Static tests parse Go imports rather than use string
substrings where package-family evidence is required.

- [ ] **Step 3: Run tests and observe the intended compile failure**

Run: `go test ./cmd/oryxos -count=1`

Expected: FAIL to compile because root and workspace APIs are undefined.

- [ ] **Step 4: Implement non-destructive workspace initialization**

Use these exact target groups:

```go
var workspaceDirectories = []string{"profiles", "sessions", "skills", "logs", "memory"}

var workspaceFiles = map[string]string{
	"AGENTS.md":            "# Project Instructions\n",
	"SOUL.md":              "# Agent Personality\n",
	"USER.md":              "# User Preferences\n",
	"memory/MEMORY.md":     "",
	"mcp_servers.yaml":     "servers: []\n",
	"profiles/default.yaml": defaultProfileYAML,
}
```

Set `defaultProfileYAML` to the exact Profile in Technical Solution section 8.2. Create `.oryxos`
and missing directories with `0750`, and missing files with exclusive create plus `0600`. Report
each of the eleven targets as `created <path>` or `skipped <path>` in deterministic sorted order.
Never truncate an existing path. Do not create `oryxos.db`, `agents/`, or `tools/`.

- [ ] **Step 5: Implement exact command behavior**

Construct the exact leaves and grouping commands. `init` calls workspace initialization. `status`
prints only:

```text
mode: foundation
workspace: initialized
```

or `workspace: not_initialized`; it prints no configuration bytes or capability claim. `serve`
builds the foundation with empty server YAML so validated defaults apply, then calls
`Application.Run(cmd.Context())`.

Define one sentinel `ErrNotImplemented = errors.New("not implemented")`; all other nine leaves
return it without fabricated data. `main` alone maps root execution failure to nonzero process
exit; no lower package calls `os.Exit`.

- [ ] **Step 6: Run focused CLI and integration tests**

Run:

```bash
gofmt -w cmd/oryxos/*.go
go test ./cmd/oryxos -count=1
go test -race ./cmd/oryxos -count=1
go vet ./cmd/oryxos
```

Expected: all commands PASS.

- [ ] **Step 7: Run exact inventory and boundary checks**

Run:

```bash
rg -n 'oryxos (init|status|chat|serve|gateway)' .agents/skills/oryxos-init/references/project-layout.md
rg -n 'oryxos (profile list|profile create|profile show|profile delete|provider list|tool list|session list)' .agents/skills/oryxos-init/references/project-layout.md
rg -n '^oryxos chat \[--profile <name>\]$|^oryxos profile (create|show|delete) <name>$' .agents/skills/oryxos-init/references/project-layout.md
go test ./cmd/oryxos -run 'TestRootCommandHasExactlyTwelveLeaves|TestFoundationRoutesAreOnlyHealthInfo|TestArchitecture' -count=1
```

Expected: static references are found and contract tests PASS.

- [ ] **Step 8: Run the post-task deviation gate**

Reconfirm exactly 12 leaves, only three functional commands, exact init counts, idempotence, no
database/agents/tools workspace path, only health/info routes, and no forbidden dependency family.

---

### Task 9: Development Standards, CI, Security, and Final Verification

**References:** `.agents/skills/oryxos-init/SKILL.md`, quality/CI/test rows in
`foundation-modules.md`, `go-quality.md`, `ci-verification.md`, and full `test-contracts.md`.

**Files:**
- Create: `docs/DevelopmentGuide.md`
- Create: `docs/ApiConvention.md`
- Create: `.editorconfig`
- Create: `.golangci.yml`
- Create: `.pre-commit-config.yaml`
- Create: `Makefile`
- Create: `.github/workflows/verify.yml`
- Create: `.github/pull_request_template.md`
- Create/update through Go tooling: `go.sum` and indirect requirements in `go.mod`
- Modify only if formatting requires it: all created Go files

**Interfaces:**
- Consumes every foundation package and test contract.
- Produces the seven public Make targets and pinned CI workflow.
- Produces exact final evidence for the `oryxos-init` report.

- [ ] **Step 1: Run the pre-task deviation gate**

Re-read `go-quality.md`, `ci-verification.md`, and the complete final execution matrix. Reclassify
all automation/docs candidates. Confirm no existing CI or Makefile conflict before editing.

- [ ] **Step 2: Write the quality-pin contract test first**

Add `TestQualityToolPins` under `cmd/oryxos/architecture_test.go`. It reads `go.mod`,
`.golangci.yml`, and `.github/workflows/verify.yml`; assert the three exact Go tool module pins,
lint config version 2, action v9.3.0, binary v2.12.2, exact checkout/setup-go refs, and absence of
`@latest`, `@main`, `@master`, or major-only action refs.

- [ ] **Step 3: Run the pin test and observe the intended failure**

Run: `go test ./cmd/oryxos -run TestQualityToolPins -count=1`

Expected: FAIL because the lint configuration and workflow do not exist.

- [ ] **Step 4: Create development and API documentation**

`DevelopmentGuide.md` records Go 1.26+, the exact Gin/Cobra/YAML/tool pins, no-global-install rule,
all Make targets, the test/race/security/CGO workflow, and links to Effective Go, Google Go Style,
and Go Code Review Comments. `ApiConvention.md` records exact Result/PageResult structs,
snake_case, real status plus stable code, `X-Request-ID`, RFC3339 UTC, safe two-key details, and
empty `items: []`.

- [ ] **Step 5: Create editor, lint, pre-commit, and Make contracts**

Copy the Make recipes from `ci-verification.md` verbatim, including private `fmt-check`; expose no
additional public targets. Configure `.golangci.yml` with `version: "2"` and all required linters.
Configure forbidigo only for non-test `internal/**` production files so user-facing Cobra output
remains allowed. Pre-commit runs only deterministic local checks and performs no download or
automatic staging.

- [ ] **Step 6: Create pinned CI and PR template**

Use exact action refs:

```yaml
- uses: actions/checkout@v6.0.2
- uses: actions/setup-go@v7.0.0
  with:
    go-version-file: go.mod
- uses: golangci/golangci-lint-action@v9.3.0
  with:
    version: v2.12.2
```

Set `permissions: contents: read`. Run format diffs and then the full ordered gate from
`ci-verification.md`, building only into `mktemp -d`. The PR template includes scope/deferral,
test/vet/race/security/CGO evidence, API/architecture impact, and secret/generated-noise checks.

- [ ] **Step 7: Resolve module sums without floating versions**

Run: `go mod tidy`

Expected: downloads only exact selected runtime/tool modules, writes `go.sum`, and preserves every
direct pin and tool directive. If sandbox network restrictions block it, request approval for this
exact dependency download rather than changing versions or skipping the gate.

- [ ] **Step 8: Format and run the full verification sequence**

Run in this exact order, stopping at the first failure:

```bash
make fmt
go mod tidy -diff
make fmt-check
go vet ./...
golangci-lint run
go test ./...
go test -race ./...
go tool gosec ./cmd/... ./internal/...
go tool govulncheck ./cmd/... ./internal/...
tmpdir="$(mktemp -d)"; CGO_ENABLED=0 go build -o "$tmpdir/oryxos" ./cmd/oryxos
```

Remove only the validated temporary build directory after confirming the binary exists. The
repository must contain no generated `oryxos` binary. Because global tool installation is
forbidden, use the already available pinned binary or an official v2.12.2 binary unpacked into a
temporary directory; report unavailable tooling if neither can be used.

- [ ] **Step 9: Run routed skill text checks**

Run:

```bash
rg -n 'Effective Go|Google Go Style|Code Review Comments|golangci-lint|gosec|govulncheck' .agents/skills/oryxos-init/references/go-quality.md
rg -n 'go test -race|CGO_ENABLED=0|pull_request_template' .agents/skills/oryxos-init/references/ci-verification.md
rg -n 'Config|Observability|PageResult|Middleware|12|health/info|Architecture' .agents/skills/oryxos-init/references/test-contracts.md
```

Expected: every required phrase is present; these text checks supplement rather than replace the
Go, race, security, lint, and build gates.

- [ ] **Step 10: Run the final post-task and whole-foundation deviation gate**

Re-read `oryxos-init/SKILL.md`, all rows of `foundation-modules.md`, and each detailed reference.
Inspect current source and report:

```text
2 HTTP routes
12 Cobra leaves
14 import-free business placeholders
5 runtime workspace directories
6 runtime initialization files
0 Prometheus/OpenAPI/retry/circuit-breaker/business surfaces
```

Confirm exact outcomes for every command, list created/patched/skipped/conflict files, and
self-review sources, scope, safety, dependency direction, and fixed counts. Only then may the
foundation be reported complete. Suggest—but do not run—the commit message
`feat(init): establish OryxOS Go foundation`.

---

## Execution Notes

Use the plan checkboxes to track work. At the end of every task, append the focused command and
outcome to the execution commentary before starting the next task. Any missing evidence, failed
test, contract change, or newly conflicting file pauses execution at that task's deviation gate.
