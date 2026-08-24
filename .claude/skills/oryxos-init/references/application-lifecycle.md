# OryxOS application lifecycle

## Contents

- [Purpose](#purpose)
- [Ownership and construction](#ownership-and-construction)
- [Run and signal contract](#run-and-signal-contract)
- [Bounded graceful reverse shutdown](#bounded-graceful-reverse-shutdown)
- [Failure behavior](#failure-behavior)
- [Required table and lifecycle tests](#required-table-and-lifecycle-tests)

## Purpose

`internal/app` owns dependency assembly, process lifecycle, and shutdown coordination. It is the
only package that wires concrete foundation dependencies together. Handlers and future runtime
components receive interfaces or already-constructed dependencies; they do not install signal
handlers, start their own process servers, or independently close shared resources.

Normal startup, serving, cancellation, and cleanup failures are returned as errors. `panic` is
not control flow for an expected configuration, listener, service, or shutdown error.

## Ownership and construction

Construct synchronously, in this exact dependency order:

1. Load and validate `config.ServerConfig`.
2. Build the mode-selected `slog` logger — production JSON or development colored console, chosen
   from `config.ServerConfig.LogFormat` — and in-process observations from `internal/observability`.
3. Build the shared API response and middleware dependencies using those observations.
4. Build the Gin router and the bounded `http.Server` from the validated server configuration.
5. Construct later Profile, Provider, Store, Tool, Scheduler, and Agent runtime dependencies only
   when their approved implementations exist; inject them into the router/scheduler through their
   documented interfaces.
6. Construct `Application` with the resulting components. It owns their start and close methods.

The foundation has no business runtime to start. Do not manufacture placeholder services merely to
fill step 5. The HTTP server is the last foundation component started, so it cannot accept traffic
until all constructed dependencies are ready.

Components that need lifecycle management use an internal, small contract such as:

```go
type Component interface {
	Start(context.Context) error
	Close(context.Context) error
}
```

`Application` registers managed components in their actual start order, which can differ from
their construction order: all applicable non-serving prerequisites start first, the scheduler
starts only after the runtime dependencies it needs, and the HTTP serving component starts last.
This keeps the process unready and unable to accept traffic until every applicable dependency is
ready. It must not store request contexts: `context.Context` is the first parameter of `Run`,
`Start`, `Close`, and all work methods. A component document must state whether `Start` blocks.
The HTTP serving component may start its one owned serve goroutine; it reports its terminal error
through an application-owned channel and `Close` waits for it to exit.

## Run and signal contract

The command entry point calls `Application.Run(parent context.Context) error`. `Run` creates one
root cancellation context with `signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)` and
defers its stop function. It starts components in registered start order, passes that root context to
each start, then waits for exactly one of:

- parent cancellation;
- `SIGINT` or `SIGTERM` cancelling the root context; or
- a component's terminal serve error.

`http.ErrServerClosed` after shutdown is normal and must not be returned as a serve failure. A
non-nil terminal error begins shutdown and remains part of the returned error. A second signal
must not start another shutdown sequence; the root context and a single `sync.Once`-guarded
shutdown path make shutdown idempotent.

Only the command/application boundary handles OS signals. `serve` and `gateway` use this same
`Application.Run` path. `chat`, `status`, and other one-shot commands receive their caller
context and do not install a process-wide signal handler.

## Bounded graceful reverse shutdown

On cancellation, terminal failure, or partial-start failure, `Application` must perform one
bounded graceful shutdown:

1. Stop admitting HTTP traffic with `http.Server.Shutdown` using a new
   `context.WithTimeout(context.Background(), cfg.ShutdownTimeout)` context. Do not derive this
   deadline from the already-cancelled root context.
2. Ask every successfully started non-HTTP component to stop and close in reverse start order.
   This includes later scheduler before runtime/provider/MCP/store dependencies it uses.
3. Wait for each owned goroutine to exit before its owning component reports `Close` complete.
4. Cancel the shutdown context and return all relevant errors with `errors.Join`.

The close order is therefore the reverse of actual successful start order, not an optimistic list
of desired dependencies. If construction fails before start, close only resources already acquired
by construction, also in reverse order. If a `Start` method fails, do not start later components;
close the successful prefix.

`ShutdownTimeout` is always positive because configuration validates it. Components must honor
the shutdown context; a timeout error is joined with other errors rather than being discarded.
`Close` remains safe to call after a failed start and after an already completed shutdown.

## Failure behavior

Wrap errors with operation and component names using `%w`; use `errors.Join` when more than one
failure must be returned. Never let a cleanup error erase the triggering serve/start error. Log
the sanitized error through the application logger, then return it so Cobra determines the command
exit status.

Every goroutine has all of the following documented at its creation site:

- an owning component or `Application`;
- the cancellation context it observes;
- a bounded/error-reporting path; and
- a wait/exit path used during close.

Do not create detached goroutines, background retry loops, global contexts, or signal-listening
goroutines outside this ownership model. Do not call `os.Exit` below `main`, because it bypasses
deferred cancellation and reverse cleanup.

## Required table and lifecycle tests

Use a fake component that records start/close events, supports an injected `Start` or `Close`
error, observes its supplied context, and optionally owns a goroutine that exits on cancellation.
Use a table-driven test with at least these rows:

| Case | Setup | Required assertions |
| --- | --- | --- |
| `normal_cancellation` | Two components start; cancel parent context | Starts occur `A`, `B`; closes occur `B`, `A`; each close receives a non-cancelled, deadline-bearing context. |
| `start_failure` | `B.Start` returns `start B` | `C` never starts; `A` closes once; returned error satisfies `errors.Is(err, startB)`. |
| `serve_failure` | HTTP/component terminal channel returns `serve failed` | Reverse shutdown runs once and returned error satisfies `errors.Is(err, serveFailed)`. |
| `multiple_close_errors` | `B.Close` and `A.Close` both fail | Both closes are attempted; returned error satisfies `errors.Is` for both failures. |
| `shutdown_deadline` | A close waits for `ctx.Done()` | Close receives the configured positive timeout; returned error includes `context.DeadlineExceeded` when it expires. |
| `owned_goroutine_exits` | Component starts a worker bound to its start context | Shutdown waits until the worker's done signal is observed; no goroutine remains after `Run` returns. |
| `server_closed_is_normal` | Serve reports `http.ErrServerClosed` only after cancellation | It is not the triggering/returned serve error. |

Use a controlled signal-context seam rather than sending real OS signals in unit tests. Include a
test that invokes shutdown twice and proves each component closes once, and a test that `Run`
returns errors rather than panicking for invalid configuration or a listener/start failure.
