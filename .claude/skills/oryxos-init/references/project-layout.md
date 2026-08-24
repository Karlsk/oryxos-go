# OryxOS project layout

## Contents

- [Purpose and scope](#purpose-and-scope)
- [Complete target skeleton](#complete-target-skeleton)
- [Package boundaries and dependency direction](#package-boundaries-and-dependency-direction)
- [Compile-safe placeholder rules](#compile-safe-placeholder-rules)
- [CLI command tree and foundation behavior](#cli-command-tree-and-foundation-behavior)
- [Implementation order and checks](#implementation-order-and-checks)

## Purpose and scope

This reference turns Technical Solution section 11 and the confirmed init-skill design into
the target Go layout. It is a routing and scaffolding contract, not an authorization to
implement Agent behavior. Preserve the repository module path and existing compatible files.

The foundation phase creates the complete directory skeleton. It provides real, tested
foundation behavior only for configuration, observability, application lifecycle, HTTP
foundation, API contract, middleware, and the three functional CLI commands specified below.
All other packages remain documented compile-safe placeholders until a later approved feature
specification implements them.

## Complete target skeleton

```text
oryxos-go/
├── cmd/
│   └── oryxos/
│       └── main.go
├── internal/
│   ├── app/                 # Application wiring and lifecycle
│   ├── config/              # ConfigLoader, types, and strict validation
│   ├── profile/             # ProfileLoader and ProfileRegistry
│   ├── skill/               # SkillLoader
│   ├── bootstrap/           # BootstrapLoader and prompt segments
│   ├── provider/            # provider factories and DeepSeek/MiniMax adapters
│   ├── runtime/             # AgentService, ReActLoop, and PromptBuilder
│   ├── memory/              # MarkdownMemoryStore
│   ├── session/             # SessionService and SessionStore
│   ├── tool/
│   │   ├── builtin/         # nine built-in tools
│   │   ├── mcp/             # official MCP Go SDK client adapter
│   │   ├── registry.go
│   │   └── executor.go
│   ├── sandbox/             # file, command, and URL allowlists
│   ├── scheduler/           # Profile schedules to AgentService
│   ├── channel/
│   │   └── cli/             # the sole core-phase Channel
│   ├── web/                 # Gin router, handlers, and DTOs
│   │   ├── api/             # shared Result and PageResult contract
│   │   └── middleware/      # request boundary middleware
│   ├── store/               # GORM, migrations, and three-table store
│   └── observability/       # slog and request/session correlation
├── docs/
├── go.mod
└── go.sum
```

`internal/web/api` and `internal/web/middleware` are foundation subpackages required by the
confirmed design. `internal/channel` is a directory boundary; `internal/channel/cli` is the
package. `internal/tool` is the package containing the later `registry.go` and `executor.go`;
its `builtin` and `mcp` directories are separate packages.

Foundation package locations are `internal/app`, `internal/config`,
`internal/observability`, `internal/web`, `internal/web/api`, and
`internal/web/middleware`. The remaining package locations are `internal/profile`,
`internal/skill`, `internal/bootstrap`, `internal/provider`, `internal/runtime`,
`internal/memory`, `internal/session`, `internal/tool`, `internal/tool/builtin`,
`internal/tool/mcp`, `internal/sandbox`, `internal/scheduler`, `internal/channel/cli`, and
`internal/store`.

Do not create `.oryxos/agents/`, a runtime `tools/` configuration directory, or Java-style
Controller, Repository, or Lifecycle layers.

## Package boundaries and dependency direction

Use this exact dependency direction:

```text
cmd -> app -> handler/channel/scheduler -> service/runtime
runtime -> Eino core interfaces + internal domain ports
provider/tool-mcp/store -> concrete external libraries
```

Consequences that must hold as the skeleton becomes real code:

- `runtime` must not import Gin, Cobra, GORM, or Eino-ext.
- `web` must not directly access a provider connector or a Store implementation.
- CLI, Web, and Scheduler must ultimately share `AgentService.Invoke`; do not build a second
  business execution path for an access channel.
- Provider factories may use Eino-ext adapters, but their consumers use Eino core interfaces.
- All executable tools eventually go through the ToolRegistry and ToolExecutor. Skill,
  Bootstrap, and Memory are prompt-context sources, not tools.
- The core release requires Go 1.26+ and remains one CGO-free binary: it cannot depend on JVM,
  Python, a system SQLite installation, or another external runtime.

## Compile-safe placeholder rules

Create each unimplemented business package with its own `doc.go`. A placeholder contains only
the shown package documentation and package declaration: no imports, interfaces, exported
types, constructors, TODO behavior, or return values. In particular, never invent an interface
or return a fake success merely to make a command appear complete.

When a later feature genuinely implements a package, keep or refine its Go package comment and
replace only the placeholder status with verified behavior. Foundation packages (`app`,
`config`, `observability`, `web`, `web/api`, and `web/middleware`) are implemented through their
dedicated references, not represented as fake business placeholders.

### `internal/profile/doc.go`

```go
// Package profile contains Profile loading and runtime registration.
//
// The oryxos-init Skill creates this package as a compile-safe placeholder.
// Profile behavior is implemented by a later feature specification.
package profile
```

### `internal/skill/doc.go`

```go
// Package skill contains loading of Profile-referenced SKILL.md content.
//
// The oryxos-init Skill creates this package as a compile-safe placeholder.
// Skill behavior is implemented by a later feature specification.
package skill
```

### `internal/bootstrap/doc.go`

```go
// Package bootstrap contains loading of Bootstrap prompt-context files.
//
// The oryxos-init Skill creates this package as a compile-safe placeholder.
// Bootstrap behavior is implemented by a later feature specification.
package bootstrap
```

### `internal/provider/doc.go`

```go
// Package provider contains model-provider factories and adapters.
//
// The oryxos-init Skill creates this package as a compile-safe placeholder.
// Provider behavior is implemented by a later feature specification.
package provider
```

### `internal/runtime/doc.go`

```go
// Package runtime contains the OryxOS Agent runtime and ReAct loop.
//
// The oryxos-init Skill creates this package as a compile-safe placeholder.
// Runtime behavior is implemented by a later feature specification.
package runtime
```

### `internal/memory/doc.go`

```go
// Package memory contains the Markdown-backed long-term memory store.
//
// The oryxos-init Skill creates this package as a compile-safe placeholder.
// Memory behavior is implemented by a later feature specification.
package memory
```

### `internal/session/doc.go`

```go
// Package session contains session resolution and persistence coordination.
//
// The oryxos-init Skill creates this package as a compile-safe placeholder.
// Session behavior is implemented by a later feature specification.
package session
```

### `internal/tool/doc.go`

```go
// Package tool contains the OryxOS tool registry and execution boundary.
//
// The oryxos-init Skill creates this package as a compile-safe placeholder.
// Tool behavior is implemented by a later feature specification.
package tool
```

### `internal/tool/builtin/doc.go`

```go
// Package builtin contains OryxOS built-in tool implementations.
//
// The oryxos-init Skill creates this package as a compile-safe placeholder.
// Built-in tool behavior is implemented by a later feature specification.
package builtin
```

### `internal/tool/mcp/doc.go`

```go
// Package mcp contains the official MCP Go SDK client adapter.
//
// The oryxos-init Skill creates this package as a compile-safe placeholder.
// MCP tool behavior is implemented by a later feature specification.
package mcp
```

### `internal/sandbox/doc.go`

```go
// Package sandbox contains application-level file, command, and URL validation.
//
// The oryxos-init Skill creates this package as a compile-safe placeholder.
// Sandbox behavior is implemented by a later feature specification.
package sandbox
```

### `internal/scheduler/doc.go`

```go
// Package scheduler contains Profile schedule triggering for AgentService.
//
// The oryxos-init Skill creates this package as a compile-safe placeholder.
// Scheduler behavior is implemented by a later feature specification.
package scheduler
```

### `internal/channel/cli/doc.go`

```go
// Package cli contains the core-phase command-line message Channel.
//
// The oryxos-init Skill creates this package as a compile-safe placeholder.
// CLI Channel behavior is implemented by a later feature specification.
package cli
```

### `internal/store/doc.go`

```go
// Package store contains GORM persistence, migrations, and core call-record stores.
//
// The oryxos-init Skill creates this package as a compile-safe placeholder.
// Store behavior is implemented by a later feature specification.
package store
```

These fourteen distinct placeholder rules cover every unimplemented business package in the
complete skeleton. They are intentionally not replacements for the later core implementation:
the core still requires two Providers, nine built-in tools, three SQLite business tables, and
the documented runtime behavior when those features are approved.

## CLI command tree and foundation behavior

Register exactly these 12 Cobra leaf commands:

```text
oryxos init
oryxos status
oryxos chat [--profile <name>]
oryxos serve
oryxos gateway
oryxos profile list
oryxos profile create <name>
oryxos profile show <name>
oryxos profile delete <name>
oryxos provider list
oryxos tool list
oryxos session list
```

Foundation behavior is deliberately narrow:

| Command | Required foundation behavior |
| --- | --- |
| `oryxos init` | Functional, idempotent, and non-destructive: create missing workspace content and report existing targets as `skipped`. |
| `oryxos status` | Functional: report verified foundation status without claiming unfinished Agent capabilities. |
| `oryxos serve` | Functional: run the bounded HTTP foundation with only `GET /api/v1/health` and `GET /api/v1/info`. |
| The other nine commands | Return a stable `not implemented` error and exit non-zero. Do not emit fabricated business data. |

Do not add a thirteenth leaf command. `profile`, `provider`, `tool`, and `session` are Cobra
grouping commands, not leaf commands.

## Implementation order and checks

Follow the phase order from `foundation-modules.md`: configuration; observability; API contract;
middleware; HTTP server; lifecycle; Cobra commands; quality gates. Then implement later business
features only under their own approved specifications.

For any scaffold run, classify every candidate before editing: `absent -> create`, `identical ->
skip`, `compatible -> precise patch`, and `conflict -> stop for approval`. Do not overwrite user
work or perform Git operations.

At the relevant implementation phase, verify the layout and command contract with at least:

```bash
rg -n 'internal/(app|config|profile|provider|runtime|memory|session|sandbox|scheduler|store)' \
  .agents/skills/oryxos-init/references/project-layout.md
rg -n 'oryxos (init|status|chat|serve|gateway)' \
  .agents/skills/oryxos-init/references/project-layout.md
rg -n 'oryxos (profile list|profile create|profile show|profile delete|provider list|tool list|session list)' \
  .agents/skills/oryxos-init/references/project-layout.md
rg -n '^oryxos chat \[--profile <name>\]$|^oryxos profile (create|show|delete) <name>$' \
  .agents/skills/oryxos-init/references/project-layout.md
```

When Go source exists, apply the project gates proportionately: `gofmt` on modified Go files,
`go test ./...`, `go vet ./...`, and `CGO_ENABLED=0 go build ./cmd/oryxos`. If prerequisites do
not exist, report the exact failed or inapplicable command instead of claiming a build passed.
