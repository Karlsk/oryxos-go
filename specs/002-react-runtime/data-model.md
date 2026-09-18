# Data Model: ReAct Runtime Orchestration

## AgentRequest

Represents one request entering the shared Agent runtime.

| Field | Meaning | Rules |
|---|---|---|
| `ProfileName` | Unique Profile runtime selection | Required and must resolve in the immutable registry |
| `Channel` | Caller category or channel identity | Passed unchanged to the Session collaboration seam |
| `UserID` | Caller identity within the channel | Passed unchanged; never combined into an ID by AgentService |
| `SessionID` | Optional explicit Session identity | Resolution belongs to the Session service |
| `Message` | Current user message | Required and appended exactly once |
| `Stateless` | Requests isolated invocation behavior | Interpretation belongs to the Session service |

## AgentResponse

| Field | Meaning | Rules |
|---|---|---|
| `SessionID` | Session used for the invocation | Returned only from the resolved Session |
| `Content` | Final assistant text | Present only after a no-Tool final response |

## Session Aggregate

The lesson 17 Session aggregate is the ordered in-memory conversation used by the runtime. Lesson
18 adds identity generation, persistence, lookup, archive state, and concurrency control.

| Field | Meaning | Rules |
|---|---|---|
| ID | Stable identity supplied by the Session service | Required before model or Tool auditing |
| Messages | Ordered OryxOS messages | User, complete assistant, and correlated Tool messages only |

### Message invariants

- The current user message is appended once before the first model call.
- A complete assistant message is appended before any Tool it requests executes.
- Every Tool result carries the matching assistant Tool-call ID and Tool name.
- A history turn starts with a user message and includes all following assistant/Tool messages until
  the next user message.
- Truncation removes whole oldest turns and never leaves an orphan assistant Tool call or Tool result.
- A per-Session lock serializes message reads and appends; callers receive defensive copies rather
  than the mutable backing slice.

## Prompt Source Snapshot

Immutable startup material keyed by Profile name.

| Component | Source | Rule |
|---|---|---|
| Runtime rules | OryxOS constant | Highest authority and always present |
| Project rules | Bootstrap `AGENTS.md` or explicit replacement | Clearly labelled |
| Agent identity | Profile identity plus `SOUL.md` when present | Profile/Skill authority precedes persona |
| Skills | Only Profile-referenced `SKILL.md` files | Missing explicit reference is invalid |
| User preferences | `USER.md` when present | Cannot override higher-priority instructions |
| Long-term Memory | Memory reader at request time | Maximum 4,000 characters; lowest authority |
| History | Session aggregate | Newest complete configured turns |
| Current request | Current user message | Never removed by truncation |
| Tool metadata | Selected Profile Tool names | Only the permitted subset |
| Current time | Injected clock | Final prompt section |

Bootstrap and Skill content is copied into the startup snapshot. Memory and Session are mutable
state and are read through their own service seams.

## OryxTool

Runtime wrapper around one executable OryxOS Tool.

| Field | Meaning | Rule |
|---|---|---|
| Tool | Executable `InvokableTool` | Required; provides metadata and JSON-argument execution |
| Retryable | Whether explicitly retryable failures are eligible | Does not permit retry alone |
| Idempotent | Whether repeated execution is safe by default | Side-effecting Tools default false |
| Timeout | Per-attempt bound | Must be positive |

## ToolInvocation

Maps exactly to the approved `tool_invocations` table.

| Field | Type / constraint |
|---|---|
| `id` | INTEGER PRIMARY KEY AUTOINCREMENT |
| `session_id` | TEXT NOT NULL, indexed |
| `tool_name` | TEXT NOT NULL |
| `input_json` | TEXT NOT NULL |
| `result_json` | TEXT NOT NULL |
| `success` | BOOLEAN NOT NULL |
| `error_message` | TEXT NULL; redacted |
| `duration_ms` | INTEGER NOT NULL; non-negative |
| `created_at` | DATETIME NOT NULL |

### Validation invariants

- Session ID and Tool name are non-empty.
- Input and result are always stored as non-null text, including failure outcomes.
- Successful rows have no error message.
- Failed rows carry a non-empty sanitized error message.
- Duration is never negative and creation time is non-zero.
- Exactly one row represents the final outcome of one logical Tool call, regardless of physical
  retry count.

### State transitions

```text
requested
  -> rejected_before_invoke -> failed row
  -> executing
       -> retry_eligible -> executing (bounded)
       -> succeeded -> successful row
       -> failed -> failed row
```

No retry-attempt table or column is introduced. Physical attempts are structured-log events.
