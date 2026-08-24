# OryxOS CI and verification contract

## Contents

- [Purpose and scope](#purpose-and-scope)
- [Generated automation artifacts](#generated-automation-artifacts)
- [Required Make targets](#required-make-targets)
- [Makefile command contract](#makefile-command-contract)
- [Pre-commit: quick and local](#pre-commit-quick-and-local)
- [Continuous integration: full and reproducible](#continuous-integration-full-and-reproducible)
- [GitHub Actions pinning](#github-actions-pinning)
- [Failure and reporting](#failure-and-reporting)
- [Read-only skill validation](#read-only-skill-validation)
- [Explicit exclusions](#explicit-exclusions)

## Purpose and scope

This reference turns the Go quality policy into reproducible repository automation. It only
describes files a later implementation generates; Task 5 does not create the Makefile, workflow,
pre-commit configuration, lint configuration, or PR template.

These checks add to AGENTS.md's `go test ./...`, `go vet ./...`, and
`CGO_ENABLED=0 go build ./cmd/oryxos` requirements. A gate is evidence only when the command
succeeds against current source. A missing Go/module prerequisite is reported as unavailable,
not passed.

## Generated automation artifacts

| File | Contract |
| --- | --- |
| `Makefile` | Exposes exactly the required public targets below and no target that suppresses failure. Private helpers such as `fmt-check` are permitted. |
| `.golangci.yml` | Uses `version: "2"`, compatible with `golangci-lint v2.12.2`. |
| `.pre-commit-config.yaml` | Fast, local, deterministic pre-commit checks only; no downloads/installations. |
| `.github/workflows/verify.yml` | Full CI for pull requests and default-branch pushes using the exact pinned tools in `go-quality.md`. |
| `.github/pull_request_template.md` | Requires scope/deferral, quality evidence, security/secret review, and API/architecture impact. |

## Required Make targets

The later Makefile must expose these seven targets exactly:

| Target | Required result |
| --- | --- |
| `fmt` | Formats all project Go files with `gofmt` and pinned `goimports`; it may modify files. |
| `lint` | Runs pinned `golangci-lint run` against `.golangci.yml`. |
| `test` | Runs `go test ./...`. |
| `test-race` | Runs `go test -race ./...`. |
| `security` | Runs pinned `gosec` and `govulncheck` through `go tool`. |
| `build` | Builds a temporary CGO-free `oryxos` binary from `./cmd/oryxos`. |
| `verify` | Deterministically runs tidiness first, then non-mutating format check, `go vet`, lint, ordinary/race tests, security, and the CGO-free build. |

`verify` fails at the first failed command; it cannot turn a suppressed error into apparent
success. Its sequence is fixed: `go mod tidy -diff`, format check, `go vet ./...`, lint, ordinary
tests, race tests, security, then build. A target must not silently omit vet, a scanner, or race.

## Makefile command contract

The later-generated Makefile uses a Go-file list excluding `vendor`, declares these targets
phony, and implements the following commands verbatim (minor Make/POSIX quoting is allowed):

```make
.PHONY: fmt lint test test-race security build verify fmt-check

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './vendor/*')
	go tool goimports -w $$(find . -name '*.go' -not -path './vendor/*')

fmt-check:
	set -eu; tmpdir="$$(mktemp -d)"; trap 'rm -rf "$$tmpdir"' EXIT; gofmt -d $$(find . -name '*.go' -not -path './vendor/*') >"$$tmpdir/gofmt.diff"; test ! -s "$$tmpdir/gofmt.diff"; go tool goimports -d $$(find . -name '*.go' -not -path './vendor/*') >"$$tmpdir/goimports.diff"; test ! -s "$$tmpdir/goimports.diff"

lint:
	golangci-lint run

test:
	go test ./...

test-race:
	go test -race ./...

security:
	go tool gosec ./...
	go tool govulncheck ./...

build:
	tmpdir="$$(mktemp -d)"; trap 'rm -rf "$$tmpdir"' EXIT; CGO_ENABLED=0 go build -o "$$tmpdir/oryxos" ./cmd/oryxos

verify:
	go mod tidy -diff
	$(MAKE) fmt-check
	go vet ./...
	$(MAKE) lint
	$(MAKE) test
	$(MAKE) test-race
	$(MAKE) security
	$(MAKE) build
```

`verify` has no prerequisites: its explicit GNU Make `$(MAKE)` recipe calls preserve the stated
order and are expanded by Make, not passed to the shell as command substitutions. `set -e` and direct
formatter invocations preserve nonzero formatter status, while each `test ! -s` rejects a
nonempty diff. `fmt-check` creates one unique temporary directory, removes only that directory
through its shell trap, and never writes Go source. It must still run every exact command when
earlier checks pass. `fmt` repairs files; `fmt-check` is non-mutating for verification. The build
destination is likewise a unique temporary directory, never a repository artifact.

## Pre-commit: quick and local

Pre-commit is intentionally limited to fast checks:

1. reject merge-conflict markers, trailing whitespace, and files missing a final newline;
2. run non-mutating `gofmt`/`goimports` checks for changed Go files (or direct the author to
   `make fmt`), without staging modifications automatically;
3. run `go test ./...` when Go/module prerequisites exist; and
4. run a focused source check that rejects `@latest` and unpinned `uses:` values in generated
   workflow/tool configuration.

Pre-commit does **not** run `go mod tidy`, `go test -race`, `gosec`, `govulncheck`, a full build,
a network download, or `golangci-lint` through an unknown system binary. Contributors may run
`make verify` at any time; CI enforces the complete gate.

## Continuous integration: full and reproducible

CI runs for every pull request and default-branch push on a clean Go 1.26 environment. It first
performs non-mutating `gofmt -d` and `go tool goimports -d` checks, then runs this full gate:

```bash
go mod tidy -diff
go vet ./...
go test ./...
go test -race ./...
golangci-lint run
go tool gosec ./...
go tool govulncheck ./...
CGO_ENABLED=0 go build -o "$tmpdir/oryxos" ./cmd/oryxos
```

CI creates `tmpdir` with `mktemp -d` and removes only that directory after the build. It does not
commit formatting, run automatic fixes, rely on a system SQLite installation, or ignore exit
codes. `go vet ./...` is mandatory even if lint runs the corresponding analyzer.

## GitHub Actions pinning

All marketplace actions use explicit documented version refs. In particular the workflow contains:

```yaml
- uses: golangci/golangci-lint-action@v9.3.0
  with:
    version: v2.12.2
```

The pins are official releases listed in `go-quality.md`. Checkout/Go setup actions receive
similarly exact trusted refs. `@main`, `@master`, `@latest`, a major-only tag, an unqualified
`uses:` value, or a floating lint binary is forbidden. The text `pull_request_template` denotes
the required template artifact only; it cannot replace enforced CI.

## Failure and reporting

Every failed command blocks delivery until corrected or an approved narrow exception exists.
Report the exact command, exit outcome, and safe diagnostic, never secrets, expanded config,
request bodies, or credentials. Fix or narrowly triage lint/security findings with owner,
rationale, bounded scope, and review date; do not add a broad exclusion.

PR authors record executed commands/outcomes. Reviewers verify no generated binary/tool cache,
secret, floating version, or foundation-scope expansion entered the diff. A CGO build failure is
a release blocker.

## Read-only skill validation

After documentation changes, run these text checks:

```bash
rg -n 'Effective Go|Google Go Style|Code Review Comments|golangci-lint|gosec|govulncheck' \
  .agents/skills/oryxos-init/references/go-quality.md
rg -n 'go test -race|CGO_ENABLED=0|pull_request_template' \
  .agents/skills/oryxos-init/references/ci-verification.md
rg -n 'Config|Observability|PageResult|Middleware|12|health/info|Architecture' \
  .agents/skills/oryxos-init/references/test-contracts.md
```

They confirm required wording only. They never substitute for `make verify`, CI, the race
detector, security scanning, or a build when sources exist.

## Explicit exclusions

Automation does not add global installation, dependency upgrades, Git commits, release/publish
flows, container builds, coverage quotas, external SaaS, Prometheus, OpenAPI, or a runtime
endpoint. The skill itself runs no Git command.
