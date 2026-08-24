# OryxOS outbound HTTP client boundary

## Contents

- [Purpose and scope](#purpose-and-scope)
- [Permitted foundation capability](#permitted-foundation-capability)
- [Context, cancellation, and timeout rules](#context-cancellation-and-timeout-rules)
- [Connection pooling and transport ownership](#connection-pooling-and-transport-ownership)
- [Explicit retry boundary](#explicit-retry-boundary)
- [Later ToolExecutor retry contract](#later-toolexecutor-retry-contract)
- [Provider and Tool limits](#provider-and-tool-limits)
- [Required tests](#required-tests)
- [Extension-stage exclusions](#extension-stage-exclusions)

## Purpose and scope

This reference defines the foundation boundary for outbound HTTP mechanics. It permits a small,
shared `net/http` construction seam for pooled connections, positive timeouts, and context-aware
requests. It does not implement an LLM Provider client, a Tool client, a webhook client, a
business SDK, or a general retry framework. The foundation has no outbound business call to make.

The construction seam is intentionally consumer-neutral. `internal/app` may construct one shared
`*http.Client` and inject it into a later approved consumer. It must not add a Provider-specific,
Tool-specific, Agent-specific, or generated API client in this foundation phase. A later feature
may select the narrow package location that preserves the documented dependency direction; this
reference does not require a speculative new package or source file today.

## Permitted foundation capability

The only allowed capability is constructing a reusable standard-library client from explicit,
validated transport and timeout settings. A suitable future interface is:

```go
type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

func NewOutboundClient(cfg OutboundHTTPConfig) (*http.Client, error)
```

`OutboundHTTPConfig` must express positive overall request and transport timeouts. Its validation
rejects zero and negative values rather than silently relying on an unbounded default. It is
process-level mechanical configuration only; it must not become a second source for Profile
Provider credentials, model configuration, URL allowlists, Tool authorization, or notification
channels.

The shared client can be injected as `HTTPClient` so future callers can test cancellation and
response mapping without a network. It exposes only `Do`; it must not add methods that encode a
provider protocol, a Tool operation, JSON business DTO, retry policy, or a fake successful result.

## Context, cancellation, and timeout rules

Every outbound call begins with a caller-owned context:

```go
req, err := http.NewRequestWithContext(ctx, method, target, body)
resp, err := client.Do(req)
```

`ctx` is supplied by the caller and carries correlation from CLI, HTTP, Scheduler, or runtime.
The client never replaces it with `context.Background`, stores it on a struct, or derives an
unbounded background context. A caller cancellation or deadline must interrupt the request and be
observable to the caller. Callers close a non-nil `resp.Body` on every path.

The client's configured request timeout and the caller's context deadline are complementary: the
earlier deadline wins. A consumer that requires a distinct timeout derives a bounded child from
its caller context and documents why; it does not mutate the shared client or use a global timeout.
Never set a timeout to zero to accommodate a long-running request. A later Provider connector
must keep its own approved time budget while still accepting and propagating its caller context.

## Connection pooling and transport ownership

One application-owned `http.Transport` may be shared by the injected client(s). It may enable
keep-alive and define bounded idle connection pools plus positive dial, TLS-handshake,
response-header, and idle-connection timeouts. This is connection reuse, not a retry mechanism.
The transport is immutable after construction; per-request mutation races are forbidden.

The owner closes idle connections during normal application shutdown after consumers stop making
new calls. Consumers never close a shared transport. Tests may supply an isolated transport. The
foundation must not create a new `http.Client` or transport for every request, because doing so
defeats pooling and makes timeout behavior inconsistent.

Redirect behavior is an explicit construction decision. If redirects are allowed for a later
HTTP Tool, its Sandbox validator must run on the initial URL and every redirect destination as
required by `AGENTS.md`; a generic client must not silently grant a redirect bypass. The
foundation does not implement URL allowlisting or a redirect policy because those are later Tool
and Sandbox responsibilities.

## Explicit retry boundary

The outbound client performs one `Do` call for one caller attempt. It must not implement
transparent retries, retry middleware, retryable-status loops, backoff, automatic failover, or a
retrying RoundTripper. It must not hide a failed attempt from its caller. The caller receives the
response or error from that attempt and decides under its own approved policy.

In particular, do not add a retry library, an exponential-backoff helper to the shared client, a
wrapper that repeats `Do`, or a provider fallback chain. A timeout, TCP error, DNS error, 5xx
response, or redirect-policy rejection is returned to the calling boundary. The shared transport's
normal connection pooling is permitted; no application-level transparent retry policy is.

This boundary avoids duplicated side effects and keeps failure accounting clear. It does not
contradict the later Tool retry policy: that policy belongs to `ToolExecutor`, where it can inspect
Tool metadata, idempotency, error classification, correlation, logging, and final persistence.

## Later ToolExecutor retry contract

When the later `internal/tool` implementation exists, `ToolExecutor` is the sole owner of
automatic Tool retries. It may retry only when both conditions hold:

1. The actual error is explicitly classified retryable.
2. The Tool is idempotent, or this request carries a reliable idempotency key.

Eligible retries use exponential backoff that is bounded by the caller context. There are at most three retries
after the initial attempt (therefore at most four attempts total). On cancellation
or deadline expiry, stop immediately; do not schedule a background retry. Each actual attempt is
logged with its safe retry classification and duration. The final outcome writes one
`tool_invocations` record; the core schema gains neither a retry table nor retry-count field.

`write_file`, `shell`, `http_post`, `notify`, and `save_memory` are non-idempotent by default and
therefore are not automatically retried without a reliable idempotency key and an explicitly
retryable error. A Tool returning a non-retryable error is attempted once even if it is idempotent.
The executor runs multi-tool calls serially in model-return order; retrying one Tool does not make
separate Tool calls parallel.

## Provider and Tool limits

The foundation does not build a DeepSeek, MiniMax, OpenAI-compatible, MCP, webhook, or any other
provider/tool client. Provider connector construction remains later `internal/provider` work and
must expose Eino core interfaces to the runtime. HTTP Tools remain later `internal/tool/builtin`
work and must run URL validation, redirects, input/output limits, timeouts, and invocation
recording through the ToolExecutor/Sandbox boundary.

No future consumer may interpret the generic client's possession of a `*http.Client` as
authorization to call arbitrary URLs. `http_get`, `http_post`, and `notify` must validate their
targets through the unified Sandbox before every initial request and redirect. Provider endpoints
come from validated Profile configuration and must keep secrets out of logs and error values.

## Required tests

When the construction seam and later executor are implemented, use focused tests at their owning
boundaries:

| Case | Setup | Required assertion |
| --- | --- | --- |
| `caller_context_cancels_request` | Blocking test server; cancel caller context | `Do` returns a context cancellation/deadline error; no background request continues. |
| `positive_timeout_required` | Zero/negative transport or client timeout config | Construction fails with a stable field-path error. |
| `shared_transport_reused` | Multiple requests through injected shared client | One configured transport is reused; consumers do not construct per-request clients. |
| `no_transparent_retry` | RoundTripper counts calls and first call fails | One caller attempt makes one `Do` call; no hidden second attempt occurs. |
| `retryable_idempotent_tool` | Later ToolExecutor gets retryable error from idempotent Tool | It retries with bounded exponential backoff and stops after at most three retries. |
| `non_idempotent_tool_not_retried` | `http_post` or `notify` fails retryably without key | Exactly one attempt occurs. |
| `idempotency_key_allows_retry` | Non-idempotent Tool has reliable key and retryable error | Executor, not client, performs bounded retry. |
| `cancellation_stops_backoff` | Context expires while executor waits | No further Tool attempt starts and final error preserves cancellation. |

Tests must prove the boundary by call count rather than merely asserting that a helper name lacks
the word retry. They must separately prove that the generic client never owns the ToolExecutor
policy.

## Extension-stage exclusions

A circuit breaker remains extension-stage only. Do not introduce breaker state, open/half-open
states, health probes, shared failure counters, provider routing, hedge racing, automatic
provider fallback, bulkheads, or adaptive retry budgets in the foundation. Those behaviors change
availability semantics and require a later approved specification.
