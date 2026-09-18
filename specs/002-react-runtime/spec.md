# Feature Specification: ReAct Runtime Orchestration

**Feature Branch**: `main`

**Created**: 2026-09-17

**Status**: Draft

**Input**: User description: "第17节：实现由 OryxOS 自主管理的 ReAct 循环、上下文组装、工具执行、统一调用入口与工具调用审计。"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Complete a Tool-Assisted Request (Priority: P1)

An Agent user submits a request that cannot be answered from model knowledge alone. The Agent
reasons about the next action, invokes the permitted Tool or Tools, observes their results, and
continues until it can return a final answer.

**Why this priority**: This is the minimum useful Agent behavior and the core value of the
feature. Without the controlled reason-act-observe cycle, OryxOS remains a chatbot rather than an
Agent runtime.

**Independent Test**: Use a deterministic model double that requests one or more Tools before
returning a final answer. The request is complete when each Tool ran once in model order, every
assistant and Tool message is preserved, and the final answer is returned.

**Acceptance Scenarios**:

1. **Given** a model response with no Tool request, **When** the Agent processes a user message,
   **Then** it returns that response after one model call.
2. **Given** a model response requesting one Tool followed by a final response, **When** the Agent
   processes the request, **Then** it executes the Tool, feeds the matching result back to the
   model, and returns the final response.
3. **Given** one model response containing multiple Tool requests, **When** the Agent executes the
   response, **Then** it invokes every Tool serially and in the exact order supplied by the model.
4. **Given** a model that continues requesting Tools, **When** the configured iteration limit is
   reached, **Then** processing stops with an explicit iteration-limit failure and retains the
   accumulated conversation state.

---

### User Story 2 - Build Trustworthy Agent Context (Priority: P2)

An Agent receives the correct runtime rules, project guidance, identity, business Skills, user
preferences, long-term Memory, recent conversation, current request, and permitted Tool metadata
without one source silently overriding a higher-priority source.

**Why this priority**: Tool orchestration is only reliable when the model sees complete,
well-bounded, correctly ordered context. This also prevents Memory or user-maintained content from
overriding runtime safety rules.

**Independent Test**: Assemble context from controlled Profile, Bootstrap, Skill, Memory, Session,
and Tool inputs and verify source boundaries, precedence, truncation, Tool visibility, and current
date/time placement without calling a real model.

**Acceptance Scenarios**:

1. **Given** all context sources, **When** context is assembled, **Then** the sources appear in the
   required priority order with clear provenance boundaries and current date/time at the end.
2. **Given** more conversation history than the Profile permits, **When** context is assembled,
   **Then** the earliest excess turns are omitted while the recent turns and current request remain.
3. **Given** long-term Memory longer than 4,000 characters, **When** context is assembled, **Then**
   no more than 4,000 Memory characters are included.
4. **Given** a Profile that permits only a subset of available Tools, **When** context is assembled,
   **Then** only that subset is exposed to the model.

---

### User Story 3 - Invoke One Shared Runtime Safely (Priority: P3)

CLI, Web, and scheduled callers submit the same logical Agent request and receive a response tied
to the selected Profile and Session. Failures do not discard work already accumulated during the
attempt.

**Why this priority**: A single orchestration path prevents entry points from developing different
Agent behavior and establishes the shared runtime contract required by later lessons.

**Independent Test**: Submit representative requests for all three caller categories through the
shared invocation boundary and verify Profile isolation, Session saving on success and failure,
and cancellation propagation.

**Acceptance Scenarios**:

1. **Given** valid requests from CLI, Web, and scheduler callers, **When** each request is invoked,
   **Then** all three use the same orchestration behavior and return their Session identity with the
   final content.
2. **Given** two requests selecting different Profiles, **When** they execute in sequence or
   concurrently, **Then** neither request observes the other Profile's identity, Skills, Tools, or
   settings.
3. **Given** cancellation, model failure, Tool failure, or iteration exhaustion, **When** invocation
   ends, **Then** the error is returned and the conversation accumulated before failure is still
   saved.

### Edge Cases

- The selected Profile does not exist.
- An explicitly referenced Bootstrap or Skill file is missing.
- A Tool name is unknown, disallowed by the selected Profile, or receives invalid arguments.
- A Tool succeeds but recording its outcome fails, or the Tool itself fails before producing a
  result.
- Cancellation happens before context loading, during a model call, or during Tool execution.
- A Tool response has empty content but still requires a matching Tool-call identifier.
- The configured iteration or history limit is absent and therefore uses its documented default.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST expose one shared Agent invocation path for CLI, Web, and scheduled
  callers, with Profile selection and Session resolution performed before the reasoning cycle.
- **FR-002**: The system MUST append the current user message exactly once before the first model
  call and MUST preserve the full conversation accumulated during processing.
- **FR-003**: The system MUST repeat model reasoning and Tool observation until a model response
  contains no Tool request or the selected Profile's iteration limit is reached; the default limit
  MUST be 10.
- **FR-004**: A model response without Tool requests MUST terminate the cycle and become the final
  Agent response.
- **FR-005**: Multiple Tool requests from one model response MUST execute serially in their
  original order.
- **FR-006**: The system MUST preserve every complete assistant response, including Tool-call
  identifiers and arguments, and MUST append one matching Tool-result message for every executed
  Tool request.
- **FR-007**: Before Tool execution, the system MUST validate the requested Tool, its arguments, and
  the selected Profile's allow-list through the single controlled execution path. Concrete Tools
  MUST retain the operation-specific Sandbox boundary that lesson 24 will connect before external
  filesystem, command, or HTTP effects.
- **FR-008**: Every logical Tool invocation MUST produce exactly one queryable outcome record that
  includes the Session, Tool name, input, result, success status, redacted failure reason, duration,
  and creation time.
- **FR-009**: Cancellation, model failures, Tool failures, context-loading failures, persistence
  failures, and iteration exhaustion MUST remain observable to the caller and MUST NOT be silently
  converted into successful responses.
- **FR-010**: The shared invocation path MUST save accumulated Session state after both successful
  and unsuccessful reasoning cycles.
- **FR-011**: Context MUST preserve this precedence: runtime safety rules, project rules, Profile
  identity and Skills, default persona, user preferences, and long-term Memory.
- **FR-012**: Context MUST keep Bootstrap, identity, Skill, user preference, Memory, recent Session
  history, current user message, and Tool metadata distinguishable by provenance.
- **FR-013**: Long-term Memory included in context MUST be capped at 4,000 characters. Session
  history MUST default to the most recent 20 turns and omit older turns when the Profile limit or
  model context budget requires truncation.
- **FR-014**: Context MUST include current date and time after the other prompt sections and MUST
  expose Tool metadata only for Tools enabled by the selected Profile.
- **FR-015**: Profiles without an explicit Bootstrap list MUST use the default project rules,
  persona, and user-preference files in that order. Explicitly referenced missing Bootstrap or Skill
  content MUST fail loading rather than being silently skipped.
- **FR-016**: Loaded Profile, Bootstrap, and Skill context MUST remain an immutable startup snapshot;
  configuration changes take effect only after restart.
- **FR-017**: Automatic Tool retry MUST occur only for explicitly retryable failures when the Tool
  is idempotent or carries a reliable idempotency key, with no more than three retries. File writes,
  shell commands, HTTP posts, notifications, and Memory writes MUST NOT retry by default.
- **FR-018**: The reasoning cycle MUST retain Tool execution control inside OryxOS and MUST NOT use
  an automatic Agent or automatic Tool-execution framework.
- **FR-019**: Message reads and appends for the same Session MUST be serialized so concurrent callers
  cannot race, lose messages, or observe a partially updated conversation.
- **FR-020**: When a model call succeeds but its `llm_calls` record cannot be persisted, the system
  MUST retain the complete returned assistant message in the Session, MUST return the persistence
  failure to the caller, and MUST NOT execute any Tool request contained in that message.

### Key Entities

- **Agent Request**: A caller's message plus Profile, channel, user, Session, and statelessness
  selections required by the shared invocation path.
- **Agent Response**: The final content and Session identity returned after a successful reasoning
  cycle.
- **Session**: The ordered conversation state containing user, assistant, and Tool-result messages.
- **Profile Runtime Snapshot**: The immutable combination of Profile selection, loaded context,
  model binding, permitted Tools, and bounded settings used for one Agent identity.
- **Tool Invocation Outcome**: The queryable final record for one logical Tool request, whether it
  succeeds or fails.

## Scope Boundaries

This feature does not deliver parallel Tool execution, Agent-to-Agent delegation, Agent-level
streaming, CLI streaming, SSE, WebSocket, model-generated context summaries, hot reload, Provider
fallback, additional database tables, new REST endpoints, or new CLI commands. It does not deliver
the concrete built-in and MCP Tool catalog or complete Sandbox implementation assigned to later
lessons.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: All deterministic acceptance scenarios for zero, one, and multiple Tool requests
  complete with the expected final response and 100% message-to-Tool-call identifier matching.
- **SC-002**: A non-converging request stops after exactly the configured number of model calls in
  100% of iteration-limit tests.
- **SC-003**: Multiple Tool requests execute in model order with zero reordered or duplicate
  invocations across the acceptance suite.
- **SC-004**: Every successful or failed logical Tool request produces exactly one queryable outcome
  record, with no missing or duplicate records in the acceptance suite.
- **SC-005**: Context assembly tests show 100% compliance with source order, 4,000-character Memory
  limit, configured history limit, Profile Tool filtering, and current date/time placement.
- **SC-006**: Requests from all three caller categories exhibit identical reasoning behavior, and
  every tested failure preserves accumulated Session state while returning the original failure.
- **SC-007**: Concurrent Session message tests complete with no race report, lost message, or exposed
  mutable message slice.
- **SC-008**: Audit-persistence failure tests retain the successful assistant response in 100% of
  cases and execute zero Tool calls from the unaudited response.

## Assumptions

- Lesson 16 already provides validated Profile selection, Profile-isolated model bindings, model
  calls, and model-call auditing.
- Session persistence and caller-specific Session identity rules are completed in lesson 18; this
  feature defines and tests the shared orchestration boundary against a Session service contract
  without duplicating Session identity logic.
- Full built-in Tool, MCP, and Sandbox implementations arrive in later lessons; this feature uses
  their documented controlled-execution boundaries and deterministic test doubles.
- Each `Profile.skills` entry is a workspace-relative path beneath `.oryxos/skills/` that resolves
  to a `SKILL.md` file. References do not trigger directory discovery and duplicates are invalid.
- Automated tests use local doubles and temporary directories. Real Provider and end-to-end weather
  checks remain explicit manual validation.
