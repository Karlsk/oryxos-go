# OryxOS observability contract

## Contents

- [Purpose and boundary](#purpose-and-boundary)
- [Structured logging](#structured-logging)
- [Redacting handler](#redacting-handler)
- [Correlation context](#correlation-context)
- [Internal observations](#internal-observations)
- [HTTP observation boundary](#http-observation-boundary)
- [Sensitive-data rules](#sensitive-data-rules)
- [Required table tests](#required-table-tests)

## Purpose and boundary

`internal/observability` supplies correlated structured logging and in-process observations to
the foundation HTTP server and later runtime. It uses only the Go standard library `log/slog` for
logging. It exposes no HTTP route, exporter, registry, scrape format, or global singleton.

The foundation must not register Prometheus collectors, import a Prometheus client, or expose
`/metrics`. Prometheus integration is an extension-stage concern. The internal interface exists
so a later adapter can consume observations without changing handlers or middleware.

## Structured logging

Create the application logger by wrapping a JSON handler in the required redacting handler:

```go
func NewLogger(w io.Writer, level slog.Leveler) *slog.Logger {
	jsonHandler := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})
	return slog.New(NewRedactingHandler(jsonHandler))
}
```

Production wiring writes one JSON object per line to its configured sink. Tests inject a buffer.
Use `logger.LogAttrs` or `logger.With` with typed `slog` attributes; do not build JSON manually or
format machine-readable fields into a message string. A record has the normal `slog` fields plus
the applicable correlation fields and event-specific attributes.

Log names are lower-case, stable event names such as `http.request_complete`,
`app.start_failed`, and `app.shutdown_complete`. Errors are represented as a sanitized `error`
string attribute. Unexpected failures may include a safe error kind/code but never request bodies,
response bodies, headers, raw configuration, or arbitrary `fmt.Sprintf("%+v", request)` output.

## Redacting handler

`NewRedactingHandler(next slog.Handler) slog.Handler` is mandatory for every application logger;
`NewLogger` must install it exactly as shown above. It is a `slog.Handler` wrapper, not a
call-site convention. Its methods have these required behaviors:

```go
type RedactingHandler struct {
	next   slog.Handler
	groups []string
}

func (h *RedactingHandler) Enabled(ctx context.Context, level slog.Level) bool
func (h *RedactingHandler) Handle(ctx context.Context, record slog.Record) error
func (h *RedactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler
func (h *RedactingHandler) WithGroup(name string) slog.Handler
```

- `Enabled` delegates unchanged to `next.Enabled`.
- `Handle` creates a new `slog.Record` retaining the original time, level, message, and program
  counter, then recursively sanitizes every record attribute before adding it to that new record
  and delegating to `next.Handle`. It must never delegate the original record.
- `WithAttrs` sanitizes its complete attribute slice with the active group path before passing it
  to `next.WithAttrs`, then returns another `RedactingHandler` around that derived handler. This
  new wrapper retains a copy of the same `groups` path, preventing `logger.With(...)` and
  pre-attached attributes from bypassing sanitization.
- `WithGroup` copies and extends `groups`, then returns another `RedactingHandler` around
  `next.WithGroup(name)`. A grouped logger therefore still sanitizes later attributes in `Handle`
  and `WithAttrs`; nested `slog.Group` values are recursively sanitized as well.

Use the single shared redaction facility required by `configuration.md`. To avoid a package cycle,
place that facility in `internal/config`: `config` must not import `internal/observability`, while
`internal/observability` may import `internal/config` for sanitization. The facility exposes a
keyed-value sanitizer and an error-string sanitizer. It recognizes the sensitive keys defined in
the configuration contract (including `api_key`, `authorization`, `mcp_auth`, `mcp_token`,
`password`, `secret`, `token`, and `webhook_url`, case-insensitively) and replaces the complete
value with `[REDACTED]`. It recurses through `slog.KindGroup` attributes; a sensitive group key
redacts the entire group value. Sanitization receives the `groups` path retained by the handler,
so `logger.WithGroup("secret").With("value", "top-secret")` redacts the later value even though
the attribute key itself is not sensitive.

For an attribute named `error`, the handler must convert the error or error-like value through the
shared error-string sanitizer before delegation. The sanitizer removes values following sensitive
key/value patterns and credential-bearing URLs; if it cannot prove a string safe, it returns
`[REDACTED]`. This applies equally to `slog.Any("error", err)`, `slog.String("error", text)`,
and attributes attached with `With` or `WithGroup`. The handler preserves non-sensitive keyed
values and normal JSON behavior; it does not use reflection to serialize arbitrary request or
configuration objects.

## Correlation context

Use private typed context keys and a small immutable value:

```go
type Correlation struct {
	RequestID   string
	SessionID   string
	ProfileName string
	Channel     string
	ScheduleID  string
}

func WithCorrelation(ctx context.Context, c Correlation) context.Context
func Logger(ctx context.Context, base *slog.Logger) *slog.Logger
```

`Logger` adds a non-empty value under these exact names:

```text
request_id session_id profile_name channel schedule_id
```

HTTP middleware sets or reuses `request_id` before invoking a handler. Stateful session work
adds `session_id`, Profile invocation adds `profile_name` and `channel`, and scheduler-triggered
work adds `schedule_id`. Context values are immutable: a later boundary derives a child context
rather than modifying shared state. Values flow through CLI, HTTP, scheduler, runtime, tools, and
outbound calls as their caller context; contexts are never held on long-lived structs.

At any given event, omit a correlation key whose value is not known. Do not emit placeholder
values such as `unknown`, and do not use correlation identifiers as metric labels.

## Internal observations

Define a consumer-oriented in-process interface:

```go
type Observer interface {
	ObserveHTTP(ctx context.Context, method, route string, status int, duration time.Duration)
	SetReady(ready bool)
	Snapshot() Snapshot
}

type Snapshot struct {
	Ready        bool
	HTTPRequests []HTTPRequestSnapshot
}

type HTTPRequestSnapshot struct {
	Method          string
	Route           string
	Status          int
	Count           uint64
	TotalDuration   time.Duration
}
```

The foundation implementation is concurrency safe and in-process only. It maintains readiness
and aggregates HTTP request count plus total duration by `(method, matched-route-template,
status)`. It must use the matched route template (for example `/api/v1/sessions/:id`), not a raw
path, query string, request ID, session ID, profile name, channel, or schedule ID. That prevents
unbounded cardinality and accidental sensitive-data retention.

`SetReady(false)` is the initial state. The application sets readiness true only after successful
construction and component start, and false before reverse shutdown begins or after a terminal
failure. A snapshot is a copy safe for tests and internal callers; it is not an HTTP serialization
or a public metrics API.

## HTTP observation boundary

Middleware measures one request from immediately before `c.Next()` to immediately after it,
records the final status, derives the route template, calls `ObserveHTTP`, then emits the JSON
access log. It records observation and logs even after recovered handler panics, using the mapped
final status. The corresponding access event includes exactly these foundation HTTP attributes in
addition to correlation fields:

```text
method route status duration_ms
```

`route` excludes query strings. Do not log `RequestURI`, authorization headers, cookies, request
or response bodies, remote user identifiers, or arbitrary handler parameters. The later HTTP
server reference owns request-body limits and response mapping, but it must call this observer at
the final middleware boundary.

## Sensitive-data rules

The following values must never reach log attributes, log messages, observations, snapshots, or
metric labels: API keys, webhook URLs, MCP authentication, bearer tokens, passwords, secrets,
expanded sensitive configuration, and raw request/response bodies. The required redacting handler
uses the configuration redaction facility before delegation, so a sensitive keyed attribute is
represented only as `[REDACTED]`, and preferably is omitted entirely. It sanitizes `error`
strings before JSON output because an error string can contain a credential.

This applies to all levels, including debug logs and errors from rejected configuration. Logging
a URL with an embedded credential is forbidden even if it appears in a field not normally marked
sensitive. Observations receive only the fixed fields in `Observer.ObserveHTTP` and never raw
configuration or request content.

## Required table tests

Use table-driven tests with a JSON buffer logger and the in-memory observer. Parse each JSON log
line into `map[string]any`; do not make assertions by substring alone.

| Case | Setup | Required assertions |
| --- | --- | --- |
| `json_and_correlation` | Add all five correlation values, log one event | Valid JSON contains each exact field name/value and the expected event name. |
| `partial_correlation` | Add only `request_id` | Record contains `request_id`; it omits `session_id`, `profile_name`, `channel`, and `schedule_id`. |
| `http_duration_and_status` | Observe `GET`, `/api/v1/health`, `200`, known duration twice | Snapshot has one aggregate with count `2`, status `200`, and total duration equal to the two inputs. |
| `route_not_raw_path` | Middleware observes `/api/v1/sessions/secret-session?token=nope` matched as `/api/v1/sessions/:id` | Snapshot/log use the template and contain neither `secret-session` nor `token=nope`. |
| `readiness` | Observe initial, then set true, then false | Snapshots report false, true, false in that order. |
| `sanitized_error` | Log both `slog.Any("error", err)` and `slog.String("error", text)` containing `api_key=top-secret` and `https://hooks.example.invalid/a/very-secret-token` | Parsed JSON error values are `[REDACTED]` or safely redacted; every captured line contains neither secret string. |
| `direct_sensitive_attributes` | Call `NewLogger(...).LogAttrs` with `api_key`, `webhook_url`, `mcp_auth`, `authorization`, and a nested `slog.Group` containing `token` | Parsed JSON contains no supplied sensitive value; each sensitive attribute/group is omitted or `[REDACTED]`. |
| `pre_attached_sensitive_attributes` | Use `NewLogger(...).With(slog.String("api_key", "top-secret"), slog.Any("error", err))`, then log a record | The JSON line contains neither secret; this proves `RedactingHandler.WithAttrs` sanitizes before delegating. |
| `grouped_sensitive_attributes` | Use `NewLogger(...).WithGroup("provider").With("api_key", "top-secret")`, then log a record; repeat with nested `slog.Group` values and `WithGroup("secret").With("value", "top-secret")` | The JSON line contains neither secret; this proves `RedactingHandler.WithGroup` preserves the active group path and recursive group sanitization works. |
| `no_prometheus_surface` | Build router and inspect registered routes/imports | No `/metrics` route and no Prometheus registration/import is present. |

Also race-test concurrent `ObserveHTTP`, `SetReady`, and `Snapshot` calls. The test contract for
logs is content-based: a secret must be absent from every captured JSON line, not merely absent
from the field the test intended to protect.
