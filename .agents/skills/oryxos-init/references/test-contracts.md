# OryxOS foundation test contracts

## Contents

- [Purpose and discipline](#purpose-and-discipline)
- [Harness rules](#harness-rules)
- [Configuration](#configuration)
- [Observability](#observability)
- [Result](#result)
- [PageResult](#pageresult)
- [Middleware](#middleware)
- [Server](#server)
- [Forbidden endpoints](#forbidden-endpoints)
- [Lifecycle](#lifecycle)
- [CLI](#cli)
- [Init idempotency](#init-idempotency)
- [Architecture imports](#architecture-imports)
- [Build and quality gates](#build-and-quality-gates)
- [Minimum execution matrix](#minimum-execution-matrix)

## Purpose and discipline

This is the exact foundation test matrix consumed after Tasks 2–4. Tests live beside their owning
package and exercise a public/narrow boundary. They supplement the contracts in
`configuration.md`, `observability.md`, `api-contract.md`, `http-server.md`,
`http-client-boundary.md`, and `application-lifecycle.md`.

Use every exact Go test name below. Table subtests use the exact quoted case name. Extra tests are
welcome; no implementation may rename, weaken, delete, or skip the named contracts. Use
`httptest`, temporary directories, injected lookup/listener/clock, JSON buffers, and fake
components. Never call a real Provider, MCP server, external URL, or OS signal.

## Harness rules

- Decode API/log JSON before assertions; do not rely on a substring when the contract needs
  structural evidence. Console-mode logger rows instead assert on complete human-readable text
  lines.
- Use `errors.Is`/`errors.As` for wrapped failures and prove secret literals are absent from every
  captured log/response byte sequence.
- Use `gin.New()` with real responder/middleware; do not use `gin.Default()`.
- Synchronize concurrent tests with barriers/channels, not `time.Sleep`; run them with `-race`.
- Keep literal inputs and test names stable so contract changes receive review.

## Configuration

| Exact test name | Input/setup | Required assertions |
| --- | --- | --- |
| `TestLoadServerYAMLDefaults` | Empty YAML; lookup unset. | Exact defaults: `127.0.0.1:8080`, `LogFormatConsole`, `5s`, `30s`, `5m`, `60s`, `30s`; all timeouts nonzero. |
| `TestLoadServerYAMLPartialDefaults` | `listen_address: 127.0.0.1:9090`. | Address changes; all timeouts retain exact defaults. |
| `TestLoadServerYAMLExpansion` | `listen_address: ${ORYXOS_LISTEN_ADDRESS}`; lookup returns `127.0.0.1:9090`. | Success; no placeholder remains in typed config. |
| `TestLoadServerYAMLMissingVariable` | Same YAML; variable unset. | Error names `listen_address` and `ORYXOS_LISTEN_ADDRESS`, not YAML/unrelated environment content. |
| `TestLoadServerYAMLInvalidDuration` | `http:\n  write_timeout: nope`. | Error names exactly `http.write_timeout`; no default/zero substitution. |
| `TestLoadServerYAMLZeroOrNegativeTimeouts` | Table rows for every timeout field with `0s` and `-1s`. | Each error names its field path and says greater than zero. |
| `TestLoadServerYAMLStrictUnknownFields` | Rows `http.write_timout: 5m` and top-level `provider: {}`. | First names `http`/`write_timout`; second rejects `provider`; neither is accepted. |
| `TestLoadServerYAMLInvalidAddress` | `listen_address: localhost`. | Failure names `listen_address`. |
| `TestLoadServerYAMLLogFormat` | Rows `log_format: json`, `log_format: xml`, `log_format: JSON`, and omitted. | `json` yields `LogFormatJSON`; each invalid row fails naming `log_format`; omission yields `LogFormatConsole`. |
| `TestLoadServerYAMLRedactsSecrets` | Rejected `webhook_url: https://example.invalid/hook/very-secret-token`; config/log error includes `api_key: top-secret`. | Captured output has appropriate field/[REDACTED], never either secret literal. |

## Observability

| Exact test name | Input/setup | Required assertions |
| --- | --- | --- |
| `TestLoggerJSONAndCorrelation` | JSON buffer; all five correlation fields; one event. | Parsed object has exact `request_id`, `session_id`, `profile_name`, `channel`, `schedule_id`, and event. |
| `TestLoggerPartialCorrelation` | Only `request_id`. | Request ID present; other four fields absent. |
| `TestObserverHTTPDurationAndStatus` | Observe `GET`, `/api/v1/health`, 200 twice with known durations. | One aggregate has count 2, status 200, summed duration. |
| `TestObserverUsesRouteTemplate` | Observe raw `/api/v1/sessions/secret-session?token=nope` as `/api/v1/sessions/:id`. | Snapshot/log use template and lack both secret path/query values. |
| `TestObserverReadiness` | Snapshot, SetReady true, SetReady false. | Ready sequence false, true, false. |
| `TestLoggerSanitizesError` | error attributes contain `api_key=top-secret` and credential URL. | All parsed lines are safe/redacted; neither secret appears. |
| `TestLoggerSanitizesDirectAndAttachedAttributes` | Direct sensitive attrs and `With(slog.String("api_key", "top-secret"))`. | Values omitted or `[REDACTED]`; secret absent. |
| `TestLoggerSanitizesGroupedAttributes` | `WithGroup("provider").With("api_key", "top-secret")`, nested group, `WithGroup("secret").With("value", "top-secret")`. | Every group path redacts; group state cannot bypass handler. |
| `TestLoggerConsoleFormatAndCorrelation` | `NewConsoleLogger` buffer; all five correlation fields; one event. | One human-readable line has the level name, message, and all five values; the line is not a JSON object. |
| `TestLoggerConsoleModeRedacts` | The `TestLoggerSanitizesError` and direct/attached sensitive-attribute setups through `NewConsoleLogger`. | No secret literal appears in any captured console line; sensitive values are `[REDACTED]` or omitted. |
| `TestObserverConcurrentAccess` | Barrier-synchronized concurrent ObserveHTTP/SetReady/Snapshot. | No panic/map race; must pass under `-race`. |
| `TestNoPrometheusSurface` | Router/import inspection. | No `/metrics`, Prometheus import, registry, or exporter. |

## Result

| Exact test name | Input/setup | Required assertions |
| --- | --- | --- |
| `TestResultSuccessEnvelope` | `api.Success` with non-nil payload and valid correlation. | Real success status; code/message/data/request_id; header/body IDs equal. |
| `TestResultErrorEnvelope` | `api.Error(400, "invalid_argument", "invalid argument", {field: "page", rule: "out_of_range"})`. | 400; no data; safe two-key details; matching ID. |
| `TestResultInvalidDescriptorFallback` | Each responder receives unknown status/code/message triple. | Exactly one 500/internal envelope; no data/details or second write. |
| `TestResultInvalidNestedDetailsFallback` | Nested map, extra/missing key, invalid field/rule. | One 500/internal; none of supplied nested/secret values serialize. |
| `TestResultEmergencyRequestID` | No correlation; responder inside access observation, invoked twice. | One installed ID; header/body/log agree; second call writes nothing. |
| `TestResultTimeUTC` | Non-UTC `time.Time`. | Public timestamp is RFC 3339 and ends `Z`. |
| `TestNoDirectJSONOutsideResponder` | Static source scan outside `internal/web/api`. | No `gin.Context.JSON`, `c.JSON`, or `AbortWithStatusJSON`. |

## PageResult

| Exact test name | Input/setup | Required assertions |
| --- | --- | --- |
| `TestPageResultEmptyItemsArray` | `api.Page` with nil Items, page 1, size 20, total 0. | JSON has `"items": []`, never null/omitted; all page fields present. |
| `TestPageResultDefaultsAndBounds` | Omitted parameters then malformed, duplicate, signed, decimal, zero, negative, overflow, over-limit inputs. | Defaults 1/20; invalid input is 400 `invalid_argument`. |
| `TestPageResultMaxInt64TotalPages` | `Total: math.MaxInt64`, valid size. | Quotient plus one only for remainder; nonnegative/no overflow. |
| `TestPageResultInvalidFallback` | Page outside 1..10000, size outside 1..100, negative total. | Each produces one 500/internal with no data/details. |

## Middleware

| Exact test name | Input/setup | Required assertions |
| --- | --- | --- |
| `TestRequestIDReuseAndGeneration` | Rows valid `req_ABC-1`, missing, malformed `bad id!`. | Valid ID reused; other rows generate 1..128 `[A-Za-z0-9._-]`; header/body/log match. |
| `TestMiddlewareOrderAndFinalObservation` | Required stack order; handler panics. | Access observation records final recovered 500. |
| `TestRequestBodyLimit` | Future decoder gets `(1 << 20) + 1` bytes. | 413 `payload_too_large` Result, not library plain text. |
| `TestRecoveryUsesSafeEnvelope` | Handler panics `top-secret panic`. | 500/internal; panic/stack/secret absent; final status logged/observed. |
| `TestAccessObservationPostHandlerContext` | No RequestID middleware; handler responder inside AccessObservation. | Emergency context ID makes header/body/observer/log agree after `c.Next()`. |

## Server

| Exact test name | Input/setup | Required assertions |
| --- | --- | --- |
| `TestServerTimeouts` | Server from valid `ServerConfig`. | All four server timeouts equal config and are nonzero. |
| `TestHealthReadyEnvelope` | Observer ready; GET `/api/v1/health`. | 200 `ok`, `status: ready`, matching ID. |
| `TestHealthNotReadyEnvelope` | Observer not ready; GET health. | 503 `not_ready`, data omitted, matching ID. |
| `TestInfoEnvelope` | GET `/api/v1/info` with `X-Request-ID: req_info-1`. | 200 `ok`; only safe version/mode/ready; ID in header/body/access log. |
| `TestUnknownRouteEnvelope` | GET `/not-registered`. | 404 `not_found` Result, not Gin text/HTML. |
| `TestMethodNotAllowedEnvelope` | POST `/api/v1/health`. | 405 `method_not_allowed` Result. |
| `TestServerGracefulShutdown` | Test listener, start then close with deadline. | Admission stops, serve goroutine exits, ErrServerClosed normal/non-terminal. |
| `TestRouteInventory` | Inspect router routes. | Exactly GET health and GET info, no aliases/catch-all business route. |

## Forbidden endpoints

`TestForbiddenFoundationRoutesAbsent` table inputs are `/metrics`, `/api/v1/openapi.json`,
`/openapi.json`, `/swagger`, `/swagger/index.html`, `/api/v1/events`, `/api/v1/stream`,
plus a WebSocket-upgrade request to `/api/v1/ws`. Every result is a 404 `not_found` Result
envelope, not an upgrade. The test also inspects routes/imports for hidden aliases, Prometheus,
OpenAPI, Swagger, SSE, or WebSocket registration.

`TestFoundationRoutesAreOnlyHealthInfo` compares sorted inventory to exactly
`GET /api/v1/health` and `GET /api/v1/info`: the foundation surface is only **health/info**.

## Lifecycle

| Exact test name | Input/setup | Required assertions |
| --- | --- | --- |
| `TestApplicationNormalCancellation` | Fake A/B; cancel parent. | Starts A/B, closes B/A; close contexts deadline-bearing and non-cancelled. |
| `TestApplicationStartFailure` | B.Start sentinel `start B`; C after B. | C never starts; A closes once; `errors.Is(err, startB)`. |
| `TestApplicationServeFailure` | Terminal channel sentinel `serve failed`. | Reverse shutdown exactly once; `errors.Is(err, serveFailed)`. |
| `TestApplicationJoinsCloseErrors` | B/A close errors. | Both attempts; returned error matches both sentinels. |
| `TestApplicationShutdownDeadline` | Close waits context done. | Positive configured deadline; error includes DeadlineExceeded. |
| `TestApplicationOwnedGoroutineExits` | Worker observes start context. | Shutdown waits done signal; no owned worker remains. |
| `TestApplicationServerClosedIsNormal` | ErrServerClosed after cancellation. | Not returned as triggering failure. |
| `TestApplicationShutdownIsIdempotent` | Invoke shutdown seam twice. | Each component closes once. |

## CLI

`TestRootCommandHasExactlyTwelveLeaves` walks Cobra leaves and compares sorted use lines exactly:

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

It asserts exactly **12** leaves; `profile`, `provider`, `tool`, and `session` are groups.
`TestFoundationCLIBehavior` uses injected command dependencies: a temporary workspace, a
controlled parent context, a pre-bound loopback listener, and a readiness channel owned by the
application. It never sends an OS signal, sleeps, uses an ambient port, or waits forever.

The test table is exact:

| Command/input | Deterministic assertion |
| --- | --- |
| `init` in an empty temp directory | Exit zero; output reports each of the five directories and six initial files as `created`; exact workspace exists and no database/agents/tools runtime directory exists. A second `init` exits zero, reports the same eleven existing targets as `skipped`, and leaves a prewritten `USER.md` sentinel unchanged. |
| `status` against that initialized workspace | Exit zero; output is safe, truthful foundation status only: it identifies foundation mode and does not claim Agent, Profile, Provider, Tool, Scheduler, or session capability; it contains no config bytes, API key, URL, or secret. |
| `serve` with injected listener/context | Start command in a test goroutine; wait on the readiness channel (not a sleep), then GET the injected listener's `/api/v1/health` and `/api/v1/info`. Both requests have valid Result envelopes; router inventory remains exactly the two health/info routes. Cancel the injected parent context, then require command return and owned server goroutine exit before a test-owned bounded deadline; command returns nil, while internal `http.ErrServerClosed` is normal/non-terminal and never a block. |
| `chat`, `gateway`, `profile list`, `profile create demo`, `profile show demo`, `profile delete demo`, `provider list`, `tool list`, `session list` | Each returns stable `not implemented`, non-zero exit, and no fabricated Agent/Profile/Provider/Tool/Session data. |

## Init idempotency

`TestInitCreatesExactWorkspace` invokes init in an empty temporary directory and asserts exactly
five directories (`profiles`, `sessions`, `skills`, `logs`, `memory`) and exactly six files
(`AGENTS.md`, `SOUL.md`, `USER.md`, `profiles/default.yaml`, `memory/MEMORY.md`,
`mcp_servers.yaml`) under `.oryxos/`. It also asserts no `.oryxos/agents/`, no runtime
`tools/`, and no `sessions/oryxos.db` before persistence.

`TestInitIsIdempotentAndNonDestructive` writes sentinel content to `.oryxos/USER.md`, invokes
init twice, and asserts second output says `skipped`, all six paths remain, sentinel bytes do not
change, and existing profile/bootstrap/memory files are never overwritten.

## Architecture imports

`TestArchitectureForbiddenRuntimeImports` parses source imports. It asserts `internal/runtime`
imports none of Gin (`github.com/gin-gonic/gin`), Cobra (`github.com/spf13/cobra`), GORM
(`gorm.io/gorm`), or Eino-ext (`github.com/cloudwego/eino-ext`); `internal/web` imports neither
provider connector packages nor concrete `internal/store`; placeholders import nothing.

`TestArchitectureDependencyFamiliesAtEdges` parses every non-test Go import and applies these
dependency-family allowlists (a package prefix includes its subpackages):

| Dependency family | Only permitted edge |
| --- | --- |
| Gin, `github.com/gin-gonic/gin` | `internal/web` |
| Cobra, `github.com/spf13/cobra` | `cmd/oryxos` CLI edge |
| YAML, `gopkg.in/yaml.v3` | `internal/config` and the later `internal/profile` loader edge |
| cron, `github.com/robfig/cron/v3` | `internal/scheduler` |
| Eino-ext Provider connectors | `internal/provider` |
| official MCP SDK | `internal/tool/mcp` |
| GORM, `github.com/glebarez/sqlite`, and pure-Go `modernc.org/sqlite` | `internal/store` |

The test permits framework libraries at these documented edges and does not ban all concrete
dependencies outside provider/tool-mcp/store. It retains the earlier restrictions: runtime imports
none of Gin, Cobra, GORM, or Eino-ext; web imports no Provider connector or concrete store.
`TestArchitectureNoForbiddenRuntimeImports` rejects `github.com/mattn/go-sqlite3`, Prometheus
client, Eino ADK automatic-Agent imports, and package-global retry/circuit-breaker additions.

## Build and quality gates

`TestQualityToolPins` reads generated `go.mod`, `.golangci.yml`, and workflow. It asserts exact
requirements/tool directives `golang.org/x/tools v0.49.0`, `golang.org/x/vuln v1.7.0`,
`github.com/securego/gosec/v2 v2.28.0`; lint config v2; action
`golangci/golangci-lint-action@v9.3.0`; binary `v2.12.2`; no `@latest`, `@main`, `@master`,
or unpinned `uses:` reference.

The integration gate runs `go mod tidy -diff`, `go vet ./...`, `go test ./...`,
`go test -race ./...`, `golangci-lint run`, `go tool gosec ./cmd/... ./internal/...`,
`go tool govulncheck ./cmd/... ./internal/...`, and
`CGO_ENABLED=0 go build -o "$tmpdir/oryxos" ./cmd/oryxos`. It asserts a temporary binary exists
outside the repository and all commands exit zero; failure is actual failure, never skipped.

## Minimum execution matrix

| Layer | Required proof |
| --- | --- |
| Unit | All named contracts pass with `go test ./...`. |
| Concurrency | Observation/lifecycle/middleware ownership passes `go test -race ./...`. |
| Architecture | Import/route/command inventory proves boundaries. |
| Security | Pinned `go tool gosec ./cmd/... ./internal/...` and `go tool govulncheck ./cmd/... ./internal/...` pass. |
| Release | CGO-free temporary build succeeds. |
| Contract | Exactly 12 leaves, only health/info, empty success page `items: []`, forbidden runtime imports absent. |
