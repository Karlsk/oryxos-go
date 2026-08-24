# OryxOS HTTP API contract

## Contents

- [Purpose and ownership](#purpose-and-ownership)
- [Canonical envelope types](#canonical-envelope-types)
- [Responder API](#responder-api)
- [HTTP status and stable codes](#http-status-and-stable-codes)
- [Request ID correlation](#request-id-correlation)
- [Pagination](#pagination)
- [JSON and time conventions](#json-and-time-conventions)
- [Error details and response safety](#error-details-and-response-safety)
- [Foundation endpoint examples](#foundation-endpoint-examples)
- [Required tests](#required-tests)
- [Prohibited response paths](#prohibited-response-paths)

## Purpose and ownership

`internal/web/api` owns one uniform public response contract for the Gin foundation and later
core REST handlers. It is the only package allowed to translate an application outcome into an
HTTP status plus JSON response. Handlers validate and call services; they call a responder rather
than choosing their own JSON shape. This makes success, pagination, health, info, and all known
errors observable through the same outer envelope.

This document defines HTTP response mechanics, not business DTOs, Profile configuration,
Provider behavior, Store queries, or Tool execution. Business handlers may define their data
types in their own layer, but their completed response is always `Result[T]` or
`Result[PageResult[T]]`.

## Canonical envelope types

The following generic Go structures are exact public contracts and must be preserved verbatim:

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

`Result` is always the outer object. `Code`, `Message`, and `RequestID` are always present.
Success responses set `Data` to a non-nil pointer. Error responses omit `data`; they may include
safe `details`. A `Result` is never replaced with a bare object, array, string, Gin error,
framework HTML, or plain-text error body.

`PageResult.Items` is never nil. `api.Page` must normalize a nil input slice to `[]T{}` before it
constructs the envelope, so JSON always contains `"items": []`, including a page with no results.
`Total` and `TotalPages` are nonnegative.

## Responder API

The package provides these only public response writers:

```go
func Success[T any](c *gin.Context, status int, code, message string, data T)
func Page[T any](c *gin.Context, status int, code, message string, page PageResult[T])
func Error(c *gin.Context, status int, code, message string, details map[string]any)
```

`Success` writes `Result[T]` with `Data: &data`. `Page` writes `Result[PageResult[T]]` with a
non-nil `Data` pointer after normalizing `Items`. `Error` writes a `Result[struct{}]` with no data
pointer. The package's one unexported write function sets `X-Request-ID`, takes the ID from the
request correlation context, and invokes Gin JSON serialization. Thus the responder is the only
place that directly calls `gin.Context.JSON`/`c.JSON`.

The public responder inputs are validated deterministically before any response write:

- `Success` accepts only the exact descriptors `200/ok/ok` and `201/created/created`.
- `Page` accepts only the exact descriptor `200/ok/ok` and a valid page as defined below.
- `Error` accepts only an exact error descriptor from the status/code/message table below.
- A `PageResult` is invalid when `Page` is outside `1..10000`, `PageSize` is outside `1..100`, or
  `Total` is negative. The responder normalizes nil `Items` and ignores the caller-supplied
  `TotalPages`, replacing it with the canonical overflow-safe value.

Any invalid descriptor, invalid page, or invalid detail representation writes exactly one fixed
fallback: HTTP `500`, code `internal`, message `internal server error`, no data, and no details.
The fallback is constructed directly by the unexported write function; it does not call `Success`,
`Page`, or `Error`, so validation cannot recurse. Before any write, that function returns without
writing when `c.Writer.Written()` is true. Therefore an erroneous second responder call cannot
write a second body or header.

The normal request-ID middleware must set a valid correlation ID. If a responder is invoked on a
miswired route without one, `ensureRequestID` generates one cryptographically random emergency
opaque ID, stores it in `c.Request` through `observability.WithCorrelation`, and sets the header.
It caches that ID for the request, so the fallback body/header and later access-observation
middleware all read the same value. Access observation must obtain `c.Request.Context()` only
after `c.Next()` returns, so it sees this handler-installed context rather than a stale pre-handler
copy. This narrowly scoped emergency rule repairs correlation only;
it does not make missing RequestID middleware a supported server configuration.

## HTTP status and stable codes

HTTP status conveys transport semantics; `Code` conveys a stable machine-readable application
classification. A code is a lower-case ASCII string with underscores and must not be derived from
an error message. Clients should branch on `Code` and, where needed, HTTP status—not on
`Message`. Messages are stable, user-safe summaries and do not contain credentials or raw input.

The foundation and later core use this initial mapping:

| HTTP status | Code | Stable message | Use |
| --- | --- | --- | --- |
| `200` | `ok` | `ok` | Successful read, health, info, or invocation result. |
| `201` | `created` | `created` | A resource was created. |
| `400` | `invalid_request` | `invalid request` | Malformed JSON or request shape. |
| `400` | `invalid_argument` | `invalid argument` | A parsed field violates a documented constraint. |
| `404` | `not_found` | `not found` | No registered route or requested resource. |
| `405` | `method_not_allowed` | `method not allowed` | Path exists but method is unsupported. |
| `409` | `conflict` | `conflict` | State conflict. |
| `413` | `payload_too_large` | `payload too large` | Request exceeds body limit. |
| `429` | `rate_limited` | `rate limited` | Reserved for later rate-limit feature; do not implement it now. |
| `500` | `internal` | `internal server error` | Unhandled/unknown server failure. |
| `501` | `not_implemented` | `not implemented` | Honest deferred behavior when endpoint is later introduced. |
| `503` | `not_ready` | `service not ready` | Health/readiness is false. |

Do not return `200` for an error, encode HTTP status into `Code`, create ad hoc codes from
Go error strings, or leak backend-specific provider/MCP/SQLite errors. A later feature that adds
a new code documents its status mapping and compatibility reason before shipping it.

Every API response, including a future successful archive/delete operation, uses a JSON `Result`
envelope with its real HTTP status. The foundation health/info routes and all current error paths
therefore have a body with the same outer contract.

## Request ID correlation

`X-Request-ID` is the exact public header. HTTP request middleware reuses a valid incoming value
or creates one, places it in the request context as `observability.Correlation.RequestID`, and sets
the header before handler execution. `api.Success`, `api.Page`, and `api.Error` obtain that one
context value and set the same header before writing `Result.RequestID`.

The access logger obtains `request_id` from the same context through `observability.Logger`.
Therefore body `request_id`, `X-Request-ID` response header, and JSON log field
`request_id` must match byte-for-byte. This applies to health, info, unknown route, method
error, validation error, body-size error, recovered panic, and future business responses.

## Pagination

Pagination is query-driven and one-based. The shared parser accepts `page` and `page_size` with
these exact bounds:

| Parameter | Omitted default | Accepted range | Rejection |
| --- | --- | --- | --- |
| `page` | `1` | integer `1` through `10000` | `400 invalid_argument` with safe field detail. |
| `page_size` | `20` | integer `1` through `100` | `400 invalid_argument` with safe field detail. |

Reject duplicate, non-integer, signed, decimal, zero, negative, overflow, or out-of-range values;
do not silently coerce them. An empty result page is successful and has `items: []`. A requested
page beyond the last page is successful with `items: []` unless a later endpoint contract
explicitly adopts a different behavior.

For a total `n` and page size `s`, calculate `total_pages` without adding to `n`:

```go
totalPages := n / int64(s)
if n%int64(s) != 0 {
	totalPages++
}
```

This is overflow-safe for every nonnegative `int64` total, including `math.MaxInt64`; zero total
has `total_pages: 0`. `api.Page` verifies that total is nonnegative and supplied page values are
bounded, normalizes `Items` when nil, and replaces caller-supplied `TotalPages` with this
canonical result.

## JSON and time conventions

All public JSON keys use `snake_case`; Go fields without a required tag must not escape through
the responder. Omit optional `data` and `details` only according to exact tags on `Result`.
Do not use `omitempty` for `PageResult.Items`, because an empty page must encode as `items: []`.

Public timestamps are RFC 3339 UTC strings. Serialize with:

```go
value.UTC().Format(time.RFC3339)
```

Parse accepted timestamp input with `time.RFC3339`, reject invalid values with `invalid_argument`,
and normalize stored/returned values to UTC. Do not emit local-time offsets, Unix epoch numbers,
or unconstrained formatter output in the public API.

## Error details and response safety

`Details` is optional but never accepts arbitrary diagnostic data. A nil `details` argument is
valid and is omitted. A non-nil `map[string]any` argument is untrusted and is included only when
it has exactly these two keys:

```text
field: a string matching [a-z][a-z0-9_]{0,63}
rule:  one of required, invalid_format, out_of_range, duplicate, too_large
```

Both values must be strings; no other key, missing key, extra key, array, map, pointer, struct,
number, boolean, nested value, or invalid string is permitted. A valid detail is copied into a
new two-key map. An invalid detail representation does not partially sanitize or echo input: it
uses the fixed `500/internal` fallback described above and drops every supplied detail. This exact
allow-list makes a nested or secret-bearing map unable to reach a response.

Details must not include raw request bodies, headers, cookies, query values that may be secret,
stack traces, file paths outside the approved surface, provider errors, database errors, API keys,
webhook URLs, MCP authentication, tokens, passwords, or expanded configuration.

The error message remains the stable table message unless a specific endpoint contract documents
another safe stable value. Logging can retain a sanitized internal error with its correlation; the
responder does not expose it. A recovered panic always uses `500/internal` and has no panic detail.

## Foundation endpoint examples

Ready health response:

```json
{
  "code": "ok",
  "message": "ok",
  "data": {"status": "ready"},
  "request_id": "req_01H..."
}
```

Not-ready health response:

```json
{
  "code": "not_ready",
  "message": "service not ready",
  "request_id": "req_01H..."
}
```

An empty page must serialize `items` as an array, not `null`:

```json
{
  "code": "ok",
  "message": "ok",
  "data": {"items": [], "page": 1, "page_size": 20, "total": 0, "total_pages": 0},
  "request_id": "req_01H..."
}
```

## Required tests

Use `httptest` and decode responses into `map[string]any` or the exact generic shape. At minimum
implement these cases:

| Case | Setup | Required assertions |
| --- | --- | --- |
| `success_envelope` | Call `api.Success` from test handler | Real success status, required outer fields, non-nil data, matching header/body request ID. |
| `error_envelope` | Call `api.Error` with `invalid_argument` | Real `400`, no data key, safe details, matching request ID. |
| `page_empty_items` | Call `api.Page` with nil items | JSON has `"items": []`, never `null`; totals/page fields present. |
| `paging_defaults_and_bounds` | Omit then vary page/page_size | Defaults 1/20; malformed/out-of-limit values map to `400 invalid_argument`. |
| `max_int64_total_pages` | Call `api.Page` with `Total: math.MaxInt64` and valid page size | Returned `total_pages` equals `math.MaxInt64/int64(page_size)`, plus one only when the remainder is nonzero; it is nonnegative and no overflow occurs. |
| `invalid_responder_descriptor` | Call each responder with an unknown status/code/message triple | Exactly one response is `500/internal`, has no data/details, and no recursive or second write occurs. |
| `invalid_page_fallback` | Call `api.Page` with page bounds outside `1..10000`, page size outside `1..100`, and negative totals | Each case is exactly one `500/internal` envelope with no data/details. |
| `invalid_nested_details_fallback` | Call `api.Error` with a nested map, extra key, missing key, invalid field, or invalid rule | Each case is exactly one `500/internal` envelope; neither the nested value nor another supplied value is serialized. |
| `missing_request_id_emergency` | Invoke responder with no correlation request ID while access-observation middleware surrounds the handler | After `c.Next()`, observer/logger read the emergency-ID context installed by the responder; `X-Request-ID`, body `request_id`, and access-log `request_id` match, and a repeated responder call does not write again. |
| `time_utc` | Serialize non-UTC `time.Time` | Output is RFC 3339 and ends in `Z`. |
| `health_and_info_use_responder` | Request both foundation routes | Both bodies decode as Result envelopes with request ID and real statuses. |
| `not_found_and_recovery_use_responder` | Unknown route and panic handler | `404`/`500` envelopes; no framework text/HTML; ID matches header/body/log. |
| `no_direct_json_outside_responder` | Static/package inspection | No direct `gin.Context.JSON`, `c.JSON`, or `AbortWithStatusJSON` call outside `internal/web/api`. |
| `details_redaction` | Attempt unsafe secret/raw-body detail | Response omits supplied sensitive value. |

## Prohibited response paths

No handler, middleware, service, runtime component, CLI command, or future Tool may directly call
`gin.Context.JSON` or `c.JSON` outside the responder. Do not use Gin's default recovery, plain
`http.Error`, `AbortWithStatusJSON`, a bare `map[string]any`, or a standalone JSON encoder for
an API response. Do not expose SSE, WebSocket frames, OpenAPI documents, Swagger UI, or Prometheus
text in the foundation; each would be a different response protocol and is extension-stage work.
