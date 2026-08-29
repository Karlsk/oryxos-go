# OryxOS Development Guide

## Supported toolchain

OryxOS requires Go 1.26 or newer and produces one CGO-free binary. Use the Go-distributed tools declared in `go.mod` with `go tool`; do not install tools globally, use `@latest`, or use floating version tags.

The locked runtime dependencies are:

| Dependency | Version |
| --- | --- |
| Gin | `github.com/gin-gonic/gin v1.12.0` |
| Cobra | `github.com/spf13/cobra v1.10.2` |
| YAML | `gopkg.in/yaml.v3 v3.0.1` |

The locked development tools are:

| Tool | Version | Official source |
| --- | --- | --- |
| goimports | `golang.org/x/tools v0.49.0` | [Go package](https://pkg.go.dev/golang.org/x/tools@v0.49.0/cmd/goimports) |
| govulncheck | `golang.org/x/vuln v1.7.0` | [Go package](https://pkg.go.dev/golang.org/x/vuln@v1.7.0/cmd/govulncheck) |
| gosec | `github.com/securego/gosec/v2 v2.28.0` | [Official release](https://github.com/securego/gosec/releases/tag/v2.28.0) |
| golangci-lint binary | `v2.12.2` | [Official release](https://github.com/golangci/golangci-lint/releases/tag/v2.12.2) |
| golangci-lint Action | `v9.3.0` | [Official release](https://github.com/golangci/golangci-lint-action/releases/tag/v9.3.0) |

`golangci-lint` must be an already available exact `v2.12.2` binary or an official `v2.12.2` binary unpacked into a temporary directory. It is not installed globally and is not added as a Go module dependency.

Go 1.26 canonically records the three tool-only module requirements as `// indirect` after `go mod tidy`; their exact versions and the matching `tool` directives remain mandatory.

## Local workflow

Format first, inspect the diff, then run the relevant checks:

```bash
make fmt
make lint
make test
make test-race
make security
make build
make verify
```

The seven public Make targets are `fmt`, `lint`, `test`, `test-race`, `security`, `build`, and `verify`. `verify` runs the complete non-mutating gate in this order: module tidiness, format check, vet, lint, ordinary tests, race tests, security scans, and a temporary CGO-free build.

The pre-commit configuration intentionally runs only deterministic local checks: whitespace/conflict detection, non-mutating Go formatting checks, ordinary tests, and pinned-reference checks. It does not download tools, stage changes, run race/security/build gates, or modify source files.

## Engineering standards

Follow [Effective Go](https://go.dev/doc/effective_go), [Google Go Style](https://google.github.io/styleguide/go/), and [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments). OryxOS requirements take precedence: public JSON uses `snake_case`, all API responses use the shared responder envelope, sensitive values are redacted, and the foundation HTTP surface remains only health/info.

Use `gofmt` and the pinned `goimports` for changed Go files. Keep interfaces small and owned by consumers, pass `context.Context` first, wrap boundary errors with `%w`, and avoid credentials in errors or logs. Production code under `internal/**` emits diagnostics through `slog`; only user-facing Cobra output in `cmd/` may write directly to command output.

## Required verification

Before handoff, run and record the exact outcomes of:

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

Confirm the temporary binary exists, then remove only that temporary directory. Never leave a generated `oryxos` binary in the repository.
