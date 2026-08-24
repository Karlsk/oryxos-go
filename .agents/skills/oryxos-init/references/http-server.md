# OryxOS foundation HTTP server

## Contents

- [Purpose and scope](#purpose-and-scope)
- [Construction contract](#construction-contract)
- [Router and route boundary](#router-and-route-boundary)
- [Response and readiness behavior](#response-and-readiness-behavior)
- [Middleware order and behavior](#middleware-order-and-behavior)
- [Request IDs and correlation](#request-ids-and-correlation)
- [Recovery, body limits, and errors](#recovery-body-limits-and-errors)
- [Serving and graceful shutdown](#serving-and-graceful-shutdown)
- [Required tests](#required-tests)
- [Explicit exclusions](#explicit-exclusions)

## Purpose and scope

`internal/web` owns the bounded Gin HTTP foundation. It translates HTTP requests into the shared
`internal/web/api` responder contract, installs request-boundary middleware, and exposes only two
foundation endpoints. It is not a Channel, Provider, Tool, Store, or Agent-runtime implementation.
Future business handlers receive services through narrow interfaces; they do not directly access a
Provider connector or Store implementation.

The foundation route set is exactly:

```text
GET /api/v1/health
GET /api/v1/info
```

The later approved core implementation adds the remaining eight documented REST endpoints. This
foundation must not pre-register them as placeholders: an unimplemented route has the normal
`not_found` API error envelope. The router has no global mutable state and is constructed once by
`internal/app` after configuration, logging, observations, and responder dependencies exist.

## Construction contract

Construct Gin with `gin.New()`. Do not use `gin.Default()`: its default logger and recovery
middleware bypass the required JSON access-log, correlation, recovery, and response-envelope
contracts. The constructor receives a validated `config.ServerConfig`, an `observability.Observer`,
and a base `*slog.Logger` created through `observability.NewLogger`.

The server constructor must build this exact bounded `http.Server` shape:

```go
server := &http.Server{
	Addr:              cfg.ListenAddress,
	Handler:           router,
	ReadHeaderTimeout: cfg.ReadHeaderTimeout,
	ReadTimeout:       cfg.ReadTimeout,
	WriteTimeout:      cfg.WriteTimeout,
	IdleTimeout:       cfg.IdleTimeout,
}
```

All four `http.Server` timeouts are mandatory and nonzero. `config.LoadServerYAML` guarantees
that precondition and supplies the exact default values `5s`, `30s`, `5m`, and `60s` respectively.
Do not substitute a zero timeout, a package-level default, or a shorter write timeout. The server
does not own `ShutdownTimeout`; `internal/app` supplies that bounded deadline to `Shutdown` as
documented in `application-lifecycle.md`.

`NewServer` returns the router and its server component without listening. Binding the listener
happens during component `Start`, after all prerequisite components have started. A bind failure
is returned synchronously and prevents the process from becoming ready.

## Router and route boundary

Register routes beneath one `/api/v1` group and register only these handlers:

| Method | Path | Handler responsibility |
| --- | --- | --- |
| `GET` | `/api/v1/health` | Return the current in-process readiness state. |
| `GET` | `/api/v1/info` | Return safe build/version and foundation-mode information. |

Set `router.HandleMethodNotAllowed = true`. Register `NoRoute` and `NoMethod` handlers that call
`api.Error`; therefore unknown paths and unsupported methods return the same outer `Result`
envelope as success responses. The framework's default text or HTML 404/405 response is forbidden.

The route inventory is a hard foundation boundary. Tests must inspect `router.Routes()` and prove
that it has exactly two entries with the stated method/path pairs. In particular, do not register
aliases, a catch-all success handler, a route-discovery endpoint, or the later Session, Agent,
Profile, Memory, Tool, or Scheduler routes.

## Response and readiness behavior

Every application response, including health, info, a recovered panic, a body-limit failure, 404,
and 405, is emitted through `api.Success`, `api.Page`, or `api.Error`. Each has the same outer
`Result` envelope and includes the request ID supplied by middleware. Handlers and middleware
must not write a status or JSON body directly after delegating to a responder.

`GET /api/v1/health` has these semantics:

| Condition | HTTP status | Code | Data |
| --- | --- | --- | --- |
| `observer.Snapshot().Ready` is true | `200` | `ok` | `{ "status": "ready" }` |
| readiness is false | `503` | `not_ready` | omitted |

`GET /api/v1/info` always returns `200` and code `ok` when the server can handle the request.
Its data contains only `version`, `mode`, and `ready`; `mode` is the honest foundation mode, not
an assertion that unfinished Agent, Tool, Provider, or Scheduler capabilities are available.
Values such as API keys, URLs, Profile content, configuration bytes, build environment, and
arbitrary request values are never included.

Health and info data use `snake_case` JSON fields. If either ever contains a timestamp, serialize
it using `time.RFC3339` after `.UTC()`, so the value ends in `Z`.

## Middleware order and behavior

Install middleware in this registration order:

```text
1. RequestID
2. RequestBodyLimit
3. AccessObservation
4. Recovery
5. route handler / NoRoute / NoMethod
```

Gin executes the list in that order on entry and in reverse as each `Next` returns. Consequently,
`AccessObservation` surrounds `Recovery` and observes/logs the final mapped status even when a
handler panics. `RequestID` runs before every later component, so every response and access event
has one correlation value. `RequestBodyLimit` runs before a future JSON decoder can consume an
unbounded body.

`RequestBodyLimit` applies one shared maximum of `1 << 20` bytes (one MiB) using
`http.MaxBytesReader`. Future request-decoding helpers must map `*http.MaxBytesError` to
`api.Error` with status `413` and code `payload_too_large`; they must not return the standard
library's plain-text body. The two GET foundation routes do not read a request body, but the limit
is installed now so later routes cannot accidentally bypass it.

`AccessObservation` records its start time immediately before `c.Next()`. Immediately after
`c.Next()` returns, it obtains the current `ctx := c.Request.Context()`, final status, and Gin
matched route template. It must not cache a request context before handler execution. It then uses
that same post-handler context for both observation and correlated logging:

```go
started := time.Now()
c.Next()
ctx := c.Request.Context()
duration := time.Since(started)
observer.ObserveHTTP(ctx, method, route, status, duration)
logger := observability.Logger(ctx, baseLogger)
logger.LogAttrs(ctx, slog.LevelInfo, "http.request_complete",
	slog.String("method", method),
	slog.String("route", route),
	slog.Int("status", status),
	slog.Int64("duration_ms", duration.Milliseconds()))
```

Use the matched template (for example `/api/v1/health`) rather than a raw path or query string.
The log adds only correlation attributes supplied by `observability.Logger` plus `method`,
`route`, `status`, and `duration_ms`. It never logs request/response bodies, `RequestURI`, query
strings, headers, cookies, remote addresses, or arbitrary handler parameters.

## Request IDs and correlation

The public header is exactly `X-Request-ID`. `RequestID` accepts an incoming value only when it is
1 through 128 ASCII characters from `[A-Za-z0-9._-]`; it otherwise generates a cryptographically
random opaque identifier. It does not reject a request merely because the supplied header is
invalid. The generated/reused value is set on `X-Request-ID` before `c.Next()` and remains stable
for the whole request.

The middleware derives a request context with
`observability.WithCorrelation(ctx, observability.Correlation{RequestID: id})`, replaces
`c.Request` with `c.Request.WithContext(derived)`, and stores no context on a long-lived struct.
The responder obtains the ID from that same request context and places the identical value in
`Result.RequestID`. After handler execution, access logging obtains its logger and observer
context from the then-current `c.Request.Context()`, rather than a pre-handler copy. Thus exactly
one request ID appears in the response header, response body, and JSON access record, including
the responder's narrowly scoped emergency-ID path.

No handler may accept a request ID as a JSON field or generate a second one. Future session,
profile, channel, and schedule correlation derives child contexts from this request context as
specified by `observability.md`.

## Recovery, body limits, and errors

`Recovery` is custom middleware, not Gin's default recovery. It uses a `defer` around `c.Next()`.
On panic it logs a sanitized error through the correlated logger, calls `c.Abort()`, and calls
`api.Error(c, http.StatusInternalServerError, "internal", "internal server error", nil)`. It
must not expose panic text, stack traces, request data, credentials, or the Go runtime's default
HTML/plain-text error body. If a response was already committed, it cannot safely replace it; the
middleware records the failure and does not attempt a second response write.

All known HTTP failures map through stable API codes. At foundation scope these include
`not_found` (404), `method_not_allowed` (405), `payload_too_large` (413), `not_ready` (503), and
`internal` (500). Validation details may contain safe field names and constraint messages only;
they never contain a raw body, header, secret, credential-bearing URL, or configuration value.

No code outside `internal/web/api`'s responder may call `gin.Context.JSON`, `c.JSON`,
`c.AbortWithStatusJSON`, or write a JSON response directly. Handlers call only `api.Success`,
`api.Page`, and `api.Error`; this keeps health, info, and every error on the same envelope.

## Serving and graceful shutdown

The HTTP component's `Start(context.Context)` binds its listener synchronously, then starts one
owned goroutine to call `server.Serve(listener)`. Start does not block on serving. The component
reports a non-`http.ErrServerClosed` terminal error to the application-owned terminal-error
channel and records a wait path for the owned goroutine. It never starts a detached goroutine.

The component's `Close(ctx)` calls `server.Shutdown(ctx)` exactly through the application's
single shutdown path, closes idle connections if necessary after a shutdown deadline, and waits
for the serve goroutine to exit. `internal/app` creates `ctx` with
`context.WithTimeout(context.Background(), cfg.ShutdownTimeout)`, rather than using an already
cancelled request or root context. `http.ErrServerClosed` caused by this shutdown is normal.

Before reverse shutdown begins, the application calls `observer.SetReady(false)`. During a normal
successful start it sets readiness true only after all components, including the serving
component, have started. The HTTP component does not install signal handlers; `Application.Run`
owns signals and invokes close exactly once.

## Required tests

Use `httptest` with an in-memory observer and JSON-buffer logger. At minimum implement these
focused tests:

| Case | Setup | Required assertions |
| --- | --- | --- |
| `server_timeouts` | Build from valid `ServerConfig` | Four timeout fields exactly match config and are nonzero. |
| `health_ready_envelope` | Set observer ready true; GET health | `200`; header equals body `request_id`; code is `ok`; data status is `ready`. |
| `health_not_ready_envelope` | Leave observer false; GET health | `503`; body is Result envelope with code `not_ready` and matching request ID. |
| `info_envelope` | GET info with valid request ID | `200`; incoming ID is in header, body, and parsed `http.request_complete` log. |
| `unknown_route_envelope` | GET unregistered path | `404`, code `not_found`, and valid Result rather than framework text/HTML. |
| `method_not_allowed_envelope` | POST health | `405`, code `method_not_allowed`, and valid Result. |
| `panic_recovery` | Temporary test handler panics | `500`, `internal`; final `500` is observed/logged; response omits panic/stack text. |
| `request_id_generation` | No or malformed request-ID header | Generated ID is identical in header, body, and parsed access log. |
| `emergency_request_id_post_handler_context` | Omit RequestID middleware in a test router; handler invokes responder; access middleware surrounds it | Responder's emergency ID replaces `c.Request.Context()` during handler execution; after `c.Next()`, observer and `observability.Logger` use that current context, so header, body, and access-log IDs match exactly. |
| `body_limit_contract` | Future decoder reads over one MiB | `413`/`payload_too_large` through responder, not standard-library body. |
| `graceful_shutdown` | Start on test listener then close with deadline | Shutdown stops admission, serve goroutine exits, `ErrServerClosed` is not terminal failure. |
| `route_inventory` | Inspect router routes | Exactly `GET /api/v1/health` and `GET /api/v1/info`. |
| `forbidden_routes_absent` | Inspect routes and issue representative requests | Metrics, OpenAPI, Swagger, SSE, and WebSocket routes are unregistered and return `404 not_found` Result envelopes. |

The forbidden-route test must cover `/metrics`, `/api/v1/openapi.json`, `/openapi.json`,
`/swagger`, `/swagger/index.html`, `/api/v1/events`, `/api/v1/stream`, and a WebSocket-upgrade
request to `/api/v1/ws`. It must also inspect imports/route registration so a hidden alias cannot
pass solely because a representative path differs.

## Explicit exclusions

The foundation does not add authentication, authorization, rate limiting, CORS policy, SSE,
WebSockets, OpenAPI, Swagger UI, Prometheus, `/metrics`, Webhook triggers, a public observations
endpoint, or any business endpoint. It does not expose runtime/provider status beyond safe,
truthful foundation info. Those items require a later approved specification.
