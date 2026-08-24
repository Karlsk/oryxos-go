# OryxOS foundation module routing

## Contents

- [Purpose](#purpose)
- [Required phase order](#required-phase-order)
- [Reference index](#reference-index)
- [Cross-cutting execution rules](#cross-cutting-execution-rules)
- [Foundation completion boundary](#foundation-completion-boundary)

## Purpose

This file is an index. It routes each foundation subsystem to exactly one detailed reference;
it does not duplicate implementation contracts or authorize behavior outside the documented
foundation. Read the selected reference together with `docs/DemandAnalysis.md`,
`docs/TechnicalSolution.md`, and `AGENTS.md`; the higher-priority requirement documents win on
any conflict.

Every linked reference below is a mandatory same-execution contract. Read it before implementing
its phase; a link is not deferral permission or authorization to invent an omitted contract.

## Required phase order

```text
config -> observability -> API contract -> middleware -> HTTP server
       -> lifecycle -> Cobra commands -> quality gates
```

Complete each phase’s contract and verification before advancing. This order ensures that the
server starts with validated configuration, correlated structured logging, a single response
format, and request boundaries already in place. It does not authorize parallel business Agent
implementation.

## Reference index

| Subsystem | Detailed reference | Routing boundary |
| --- | --- | --- |
| Target tree, compile-safe business placeholders, dependency direction, workspace initialization, and Cobra leaf-command inventory | [project-layout.md](project-layout.md) | Use for the exact skeleton, placeholder-only rules, all 12 commands, and the functional/not-implemented split. |
| Configuration | [configuration.md](configuration.md) | Service listen settings, log output format, HTTP timeouts, environment parsing, defaults, strict validation, and secret redaction; not Profile business configuration. |
| Structured logging and internal HTTP observations | [observability.md](observability.md) | `slog` dual-mode logging (development colored console, production JSON), correlation fields, internal metrics interface, and sensitive-data exclusion; no Prometheus endpoint. |
| Shared HTTP responses and pagination | [api-contract.md](api-contract.md) | The exact generic `Result[T]` and `Result[PageResult[T]]` contract, error mapping, request IDs, and JSON conventions. |
| Request boundary middleware | [http-server.md](http-server.md) | Request-ID reuse/generation, recovery, JSON access log, timing/status observation, and body-size boundaries. |
| Bounded Gin server | [http-server.md](http-server.md) | Router, non-zero server timeouts, graceful shutdown, and only the foundation health/info endpoints. |
| Outbound HTTP policy boundary | [http-client-boundary.md](http-client-boundary.md) | Client construction and cancellation only; no transparent retry or circuit breaker. Tool-level retry remains later ToolExecutor behavior. |
| Application assembly and lifecycle | [application-lifecycle.md](application-lifecycle.md) | Dependency assembly, context propagation, signal handling, reverse shutdown, deadlines, and aggregated close errors. |
| Cobra root, command grouping, and exit semantics | [project-layout.md](project-layout.md) | Exactly 12 leaf commands, including `chat [--profile <name>]` and required Profile-operation names; only init, status, and serve are functional in this phase. |
| Contract and architecture tests | [test-contracts.md](test-contracts.md) | Required test matrix for foundation behavior, 12 commands, endpoint boundary, init idempotence, and dependency constraints. |
| Go style and local quality workflow | [go-quality.md](go-quality.md) | Development guide, formatting, linting, testing, race testing, source security, and dependency-vulnerability checks. |
| CI, pre-commit, and read-only verification | [ci-verification.md](ci-verification.md) | Pinned CI configuration, fast local pre-commit checks, and the non-mutating skill verification script. |

Each row names one authoritative detailed reference for that subsystem. `project-layout.md` is
the detailed reference for the CLI because the confirmed design does not define a separate CLI
reference. Every linked file must exist and be read before its implementation phase; a missing
link is a blocking contract defect, not permission to invent behavior.

## Cross-cutting execution rules

- Preserve the documented core boundary: no Prometheus endpoint, OpenAPI, Swagger, SSE,
  WebSockets, authentication, RBAC, multi-tenancy, Provider fallback, transparent HTTP retries,
  or circuit breakers in the foundation.
- Use Go 1.26+ and build a single CGO-free release binary. Do not introduce a JVM, Python,
  system SQLite, or another external runtime dependency.
- Keep `context.Context` flowing through CLI, HTTP, lifecycle, and later runtime boundaries;
  normal failures return errors rather than panics.
- Do not log API keys, webhook URLs, MCP authentication, or expanded sensitive configuration.
- Preserve dependency direction: runtime depends on Eino core interfaces and internal ports;
  only provider, MCP-tool, and store adapters use their concrete external libraries.
- Treat existing files as user work. Use `absent -> create`, `identical -> skip`, `compatible ->
  precise patch`, and `conflict -> stop for approval`; never use a broad overwrite as conflict
  resolution.
- Run no Git operations. A future delivery may suggest the checkpoint
  `docs(skills): define oryxos Go project skeleton` without creating it.

## Foundation completion boundary

The foundation is complete only when the routed modules are real and verified, the command tree
has exactly twelve leaves, and unfinished business packages remain honest compile-safe
placeholders. The only foundation HTTP routes are `GET /api/v1/health` and
`GET /api/v1/info`; the full ten-endpoint core API belongs to its later approved implementation.

For any Go implementation delivery, report exact outcomes for `go test ./...`, `go vet ./...`,
and `CGO_ENABLED=0 go build ./cmd/oryxos`, plus any unavailable prerequisites. Do not report
those commands as passing merely because this index or its routing references were written.
