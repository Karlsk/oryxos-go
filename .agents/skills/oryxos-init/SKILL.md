---
name: oryxos-init
description: >-
  Use when initializing or incrementally completing the OryxOS Go repository, especially
  for requests involving a Go project scaffold, foundation modules, structured logging,
  Gin HTTP/API conventions, CLI skeleton, CI, linting, or security checks.
---

# OryxOS Init

## Overview

Create a truthful, bounded Go 1.26+ foundation. Complete its confirmed skeleton without making
unimplemented business behavior appear real.

## Scope and Non-Goals

Build the foundation: module/executable wiring, workspace initialization, the complete
`project-layout.md` package tree, structured logging, and only `/api/v1/health` and
`/api/v1/info`. Require `CGO_ENABLED=0` for release.

Defer business Agents, demos, the remaining eight business REST endpoints, Prometheus, OpenAPI,
SSE, WebSockets, auth, RBAC, multi-tenancy, Provider fallback, transparent retries, and circuit
breakers. Never fake a capability, install tools globally, or perform Git operations.

## Sources of Truth

Read, in order: `docs/DemandAnalysis.md` (what and scope), `docs/TechnicalSolution.md`
(how), then `AGENTS.md` (engineering constraints). Existing files show state, not authority.
If higher-priority sources conflict or scope expands, stop for approval.

## Preflight Parameters

Before edits, infer and record module path, target, service name, and port. Ask for any value
that cannot be reliably inferred. Identify candidate files, overlap, and runnable checks; time
pressure or overwrite never waives preflight.

## Non-Destructive Change Policy

Classify each candidate exactly:

```text
absent -> create
identical -> skip
compatible -> precise patch
conflict -> stop for approval
```

Preserve user work, public contracts, module paths, and CI. “Overwrite” never approves discarding
a conflict.

## Execution Workflow

1. Read governing sources; complete preflight, classification, and conflict approval.
2. State the foundation outcome and business/extension deferrals.
3. In this execution, create or incrementally complete the entire `project-layout.md` skeleton.
   Implement and verify real `config`, `observability`, API responder, middleware, HTTP,
   lifecycle, Cobra, and quality/CI packages. Every unimplemented business package contains only
   its exact documented `doc.go` placeholder.
4. Register all exact 12 Cobra leaves now: `init`, `status`, and `serve` are functional; the
   other nine return stable nonzero `not implemented`. Implement the exact generic `Result[T]`
   and `Result[PageResult[T]]` contracts now. Only the eight business REST endpoints and business
   capabilities remain later work.
5. Run required checks, record exact outcomes, reinspect scope, then report.

## Reference Routing

Read the mandatory [foundation router](references/foundation-modules.md) in this execution; follow
each linked subsystem contract before its phase. Route acceptance/counts to DemandAnalysis,
architecture to TechnicalSolution, and build/safety rules to AGENTS.
Read exact callers for conflicts. Use IndustryResearch only as context, never to expand scope.

## Phase Gates

Proceed only with all four parameters, approved conflicts, documented scope, and a CGO-free
verification plan. Stop on missing evidence, a failed check, an API/contract change, or a
request for deferred work.

## Quick Reference

- Empty repository: build the complete approved foundation skeleton.
- Half-initialized repository: inspect, classify, then precisely patch or stop.
- Missing prerequisite: report the command, exact failure, and risk; do not claim success.

## Common Mistakes

“Make it complete” does not authorize fake responses or extension endpoints. Prometheus,
OpenAPI, retries, circuit breakers, and auto-commit remain deferred. Broad overwrite does not
resolve a conflict.

## Final Report

Report: status; scope and deferrals; preflight values; files created/patched/skipped;
conflicts; `command -> exact outcome`; self-review of sources, scope, safety, and boundaries;
and concerns/blockers. State completion only when the recorded evidence proves it. Suggest,
but do not run, `feat(init): establish OryxOS Go foundation`.
