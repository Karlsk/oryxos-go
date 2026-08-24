# OryxOS Go quality contract

## Contents

- [Purpose and scope](#purpose-and-scope)
- [Required generated artifacts](#required-generated-artifacts)
- [Authoritative Go guidance](#authoritative-go-guidance)
- [Code construction rules](#code-construction-rules)
- [Documentation and test rules](#documentation-and-test-rules)
- [Pinned toolchain](#pinned-toolchain)
- [Go tool directives](#go-tool-directives)
- [golangci-lint policy](#golangci-lint-policy)
- [Local workflow](#local-workflow)
- [Review checklist](#review-checklist)
- [Prohibited shortcuts](#prohibited-shortcuts)

## Purpose and scope

This reference defines the quality bar for generated OryxOS Go foundation work. It is a contract
for a later scaffold/implementation task, not authorization to create the listed project files
during documentation-only work. It complements `project-layout.md`,
`application-lifecycle.md`, and `api-contract.md`.

The release remains Go 1.26+, a single CGO-free binary, and does not gain an external runtime.
Each Go implementation delivery records real quality-gate outcomes. Documentation alone never
claims a build or test has passed.

## Required generated artifacts

A later implementation must generate and maintain all of these repository artifacts:

| Artifact | Required responsibility |
| --- | --- |
| `docs/DevelopmentGuide.md` | Local setup, supported Go version, `go tool` bootstrap, Make targets, test/race/security workflow, and links to the three external style sources below. Never teach global tool installation. |
| `docs/ApiConvention.md` | `Result[T]`, `PageResult[T]`, snake_case, real HTTP status, stable codes, `X-Request-ID`, UTC RFC 3339 timestamps, safe details, and `items: []`. |
| `.editorconfig` | UTF-8, LF, final newline, trim trailing whitespace, Go tabs, and two-space Markdown/YAML indentation. It never overrides `gofmt`. |
| `.github/pull_request_template.md` | Scope/deferral, test evidence, vet/race/security/CGO-build checkboxes, API/architecture impact, and secret/generated-noise confirmation. |
| `Makefile` | Exact `fmt`, `lint`, `test`, `test-race`, `security`, `build`, and `verify` targets from `ci-verification.md`. |
| `.golangci.yml` | golangci-lint v2 configuration tested by the pinned v2.12.2 binary. |
| `.pre-commit-config.yaml` | Fast local checks only, as defined in `ci-verification.md`. |
| `.github/workflows/verify.yml` | Full pull-request/default-branch verification using pinned actions and tools. |

No generated file contains a secret, fake business behavior, or unrelated functionality. Existing
user work is classified `absent -> create`, `identical -> skip`, `compatible -> precise patch`,
or `conflict -> stop for approval`.

## Authoritative Go guidance

The generated development guide links to and applies:

- [Effective Go](https://go.dev/doc/effective_go).
- [Google Go Style](https://google.github.io/styleguide/go/).
- [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments).

The more restrictive OryxOS rule wins: public JSON is snake_case, all failures use the responder
contract, sensitive values are redacted, and the foundation HTTP surface is only health/info.

## Code construction rules

- Run `gofmt` and pinned `goimports` on every changed Go file.
- Define small, consumer-owned interfaces beside the consumer that needs them. Inject concrete
  adapters at the application edge; do not invent broad Repository/Controller/manager layers.
- Put `context.Context` first in every context-aware work signature. Propagate it to HTTP,
  components, tools, and runtime calls; never store it in a struct or replace it with
  `context.Background` except the explicit lifecycle shutdown deadline.
- Wrap operation-boundary errors with `%w`; use `errors.Is`/`errors.As` and `errors.Join` for
  multiple shutdown failures. Return normal errors rather than panic and never put credentials
  into errors or logs.
- Every goroutine documents its owner, cancellation source, bounded/error-reporting path, and
  wait/exit path. No detached goroutines, global loops, or `os.Exit` below `main`.
- Bound concurrency: request/body size, timeout, queue, worker count, connection pool, retry,
  and shutdown wait must be finite and documented. Do not add concurrency merely for throughput.
- Use typed `slog` attributes. Only `internal/web/api` may emit JSON responses.
- Every exported package has a package comment; supported exported identifiers have concise Go
  doc comments that begin with the identifier. Compile-safe placeholders remain truthful.

## Documentation and test rules

Use table-driven tests for input matrices and exact named subtests. Assert observable contracts:
decode JSON envelopes, inspect route/import inventories, count attempts, and use `errors.Is` for
wrapped failures. Prefer `httptest`, temporary directories, injected lookup/listener/clock, and
fakes over real networks, OS signals, or sleeps.

Every behavior bug receives a regression test unless it is documentation-only. Tests never carry
real credentials. Run the race detector for concurrency-owning packages and the whole repository
before merge. The exact matrix is in `test-contracts.md`.

## Pinned toolchain

These are officially verified pins as of 2026-08-16. Record their official URLs in
`DevelopmentGuide.md`, use the exact version, and prohibit `@latest`, floating tags, and
unversioned downloads.

| Tool | Exact pin | Official source |
| --- | --- | --- |
| goimports | `golang.org/x/tools v0.49.0` | [Go package version](https://pkg.go.dev/golang.org/x/tools@v0.49.0/cmd/goimports) |
| govulncheck | `golang.org/x/vuln v1.7.0` | [Go package version](https://pkg.go.dev/golang.org/x/vuln@v1.7.0/cmd/govulncheck) |
| gosec | `github.com/securego/gosec/v2 v2.28.0` | [official release](https://github.com/securego/gosec/releases/tag/v2.28.0) |
| golangci-lint binary | `v2.12.2` | [official release](https://github.com/golangci/golangci-lint/releases/tag/v2.12.2) |
| golangci-lint Action | `golangci/golangci-lint-action v9.3.0` | [official release](https://github.com/golangci/golangci-lint-action/releases/tag/v9.3.0) |

`golangci-lint` stays an official pinned binary/action, not an unpinned Go tool dependency. The
workflow uses exactly `golangci/golangci-lint-action@v9.3.0` and binary `v2.12.2`, never `@v9`,
`@main`, `@latest`, or a marketplace shorthand.

## Go tool directives

The generated Go 1.26 `go.mod` uses exact requirements and tool directives for Go-distributed
tools, which are invoked with `go tool`:

```go
require (
	github.com/securego/gosec/v2 v2.28.0
	golang.org/x/tools v0.49.0
	golang.org/x/vuln v1.7.0
)

tool (
	github.com/securego/gosec/v2/cmd/gosec
	golang.org/x/tools/cmd/goimports
	golang.org/x/vuln/cmd/govulncheck
)
```

Use `go mod tidy -diff` to prove the declarations/sums are current. Do not use a `tools.go`
blank-import workaround, `go install ...@latest`, global scanners, or a tool directive without
its exact required module version.

## golangci-lint policy

`.golangci.yml` declares `version: "2"` and is validated with `golangci-lint v2.12.2`. Enable
at least `errcheck`, `govet`, `staticcheck`, `unused`, `ineffassign`, `unconvert`, `gocritic`,
`gocyclo`, `revive`, `errorlint`, `noctx`, and `bodyclose`. Do not suppress generated or
hand-written code merely to silence a finding. Each narrow exclusion has a reason, scope, and
review condition.

Lint is complementary to `go vet`, tests, race detection, source security, vulnerability checks,
and review. Local and CI execution use the same pinned lint release.

## Local workflow

Run `make fmt`, inspect the resulting diff, then execute applicable tests. Before handoff run
`make verify` and report each command's exact outcome. If a Go module does not exist, report the
unavailable gate rather than simulating success. Pre-commit is quick; CI/`make verify` is full.

## Review checklist

- Is scope within the documented dependency direction and foundation boundary?
- Are interfaces consumer-owned and context signatures context-first?
- Are errors `%w`-wrapped/joined and redacted?
- Does each goroutine have an owner, cancellation, bound, and join?
- Are exported APIs documented and public behavior table-tested?
- Are formatting, lint, test, race, security, and CGO-build outcomes recorded?
- Does the change contain no secret, generated binary, floating version, deferred endpoint, or
  fabricated behavior?

## Prohibited shortcuts

Do not globally suppress linters, downgrade a failed test to TODO, skip `-race` for goroutines,
replace evidence with a manual claim, add `@latest`, install tools globally, or vendor scanners.
Do not make the foundation appear to implement the later API routes or Agent/Provider/Tool work.
