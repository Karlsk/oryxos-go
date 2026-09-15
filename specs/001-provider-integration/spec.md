# Feature Specification: LLM Provider Integration

**Feature Branch**: `[001-provider-integration]`

**Created**: 2026-09-14

**Status**: Draft

**Input**: User description: "第 16 节：建立统一 Provider 入口，让多个 Profile 可选择 DeepSeek 或 MiniMax，并对每次模型调用留痕。"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Multiple Profiles Use the Intended Provider (Priority: P1)

As an OryxOS operator, I can declare DeepSeek and MiniMax as available Providers and let each Profile select its intended Provider and model, so multiple Agents can coexist without routing or configuration leaking between them.

**Why this priority**: Correct and isolated routing is the minimum viable capability; without it, an Agent may call the wrong model or use another Profile's settings.

**Independent Test**: Load Profiles selecting DeepSeek and MiniMax, invoke each independently, and verify every request reaches only the selected Provider. Repeat with two Profiles selecting the same Provider but different model settings and verify their settings remain isolated.

**Acceptance Scenarios**:

1. **Given** DeepSeek and MiniMax are both declared and two valid Profiles select different Providers, **When** each Profile makes a model request, **Then** each request reaches only its selected Provider.
2. **Given** two valid Profiles select the same Provider with different model settings, **When** each Profile makes a request, **Then** each request uses only that Profile's settings.
3. **Given** a Profile references an undeclared Provider, **When** Profiles are loaded, **Then** that Profile is rejected with a clear error and no substitute Provider is selected.

---

### User Story 2 - Configuration Changes Do Not Require Code Changes (Priority: P2)

As an OryxOS administrator, I can manage Provider connections separately from Agent choices and switch a Profile between already declared Providers or models by changing configuration only.

**Why this priority**: Separating connection ownership from Agent choices avoids duplicated credentials and keeps model changes operational rather than developmental.

**Independent Test**: Change a valid Profile's selected Provider or model without changing the application, reload the startup configuration, and verify subsequent calls use the new selection.

**Acceptance Scenarios**:

1. **Given** both core Providers are declared, **When** an administrator changes a Profile's Provider or model and restarts OryxOS, **Then** that Profile uses the new selection without a code change.
2. **Given** multiple valid Profiles and one malformed Profile, **When** startup configuration is loaded, **Then** the malformed Profile is reported and skipped while all valid Profiles remain available.
3. **Given** the instance-level Provider declaration is invalid or its required credential is unavailable, **When** OryxOS starts, **Then** startup fails with a clear, redacted configuration error.

---

### User Story 3 - Every Model Call Is Auditable (Priority: P3)

As an auditor, I can determine which Provider and model a Session called, how many tokens were reported, how long it took, whether it succeeded, and why a failed call failed.

**Why this priority**: Enterprise operators need a durable record for both normal usage and incidents; recording only successful calls leaves the most important failures invisible.

**Independent Test**: Execute one successful and one failing model call and verify both produce Session-linked records with the required outcome data before control returns to the caller.

**Acceptance Scenarios**:

1. **Given** a successful model response, **When** the call completes, **Then** one Session-linked record contains Provider, model, available token counts, duration, and a successful outcome.
2. **Given** a timeout, rate limit, authentication error, or upstream failure, **When** the call returns an error, **Then** one Session-linked record already contains a failed outcome and a redacted reason.
3. **Given** a request includes descriptions of available Tools, **When** the model returns an intention to call a Tool, **Then** the intention is returned unchanged to the caller and no Tool is executed by the Provider feature.

### Edge Cases

- Two Profile files declare the same Profile name; the duplicate must not silently overwrite the first valid registration.
- A Profile omits its Provider name or model; only that Profile is rejected.
- The instance-level Provider list contains duplicate names, an unsupported core Provider, or a missing credential; startup fails before any Profile is registered.
- A failed call provides no token usage; the audit record uses zero token counts while preserving the failed outcome.
- A failure message contains credential-like or URL-secret material; persisted and returned messages are redacted.
- A request has no Tool descriptions; the selected model is still called once without enabling automatic Tool execution.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: OryxOS MUST support exactly the two core Providers DeepSeek and MiniMax for this feature; MiniMax MUST be exercised through its OpenAI-compatible behavior.
- **FR-002**: Administrators MUST be able to declare available Provider names and environment-backed API keys once at instance startup, separately from individual Profile choices; the startup declaration MUST NOT accept a Provider endpoint.
- **FR-003**: Each Profile MUST declare its selected Provider, model, and optional temperature without containing Provider credentials.
- **FR-004**: OryxOS MUST combine a valid Profile choice with its matching instance-level Provider declaration and keep the resulting model configuration isolated by Profile.
- **FR-005**: OryxOS MUST reject an undeclared or unsupported Provider reference explicitly and MUST NOT silently choose a fallback.
- **FR-006**: A malformed Profile MUST be reported and skipped without preventing other valid Profiles from loading; an invalid instance-level Provider declaration MUST prevent startup.
- **FR-007**: For one model call, OryxOS MUST accept a Session identifier, Profile, conversation messages, and optional Tool descriptions, then return the model response unchanged to its caller.
- **FR-008**: The Provider feature MUST translate Tool descriptions and return model Tool-call intentions but MUST NOT execute any Tool or delegate execution to an automatic Agent loop.
- **FR-009**: Every successful or failed model call attempt MUST create exactly one durable, Session-linked record containing Provider, model, prompt tokens, completion tokens, total tokens, duration, success status, and an optional redacted error reason.
- **FR-010**: If accurate token usage is unavailable, OryxOS MUST record zero for the unavailable counts rather than omit the call record.
- **FR-011**: Provider credentials MUST come from environment-backed startup configuration and MUST NOT appear in Profile files, source code, logs, persisted error reasons, or returned errors. Connector selection, protocol adaptation, and endpoint policy MUST be encapsulated by the explicit DeepSeek and MiniMax factories; native connector defaults MAY be used when they already represent the official endpoint.
- **FR-012**: Provider failures MUST be returned to the caller without fallback, hedge racing, circuit breaking, or automatic retry by this feature.
- **FR-013**: Streaming responses, model-cost dashboards, ReAct iteration, and actual Tool execution MUST remain outside this feature.

### Key Entities

- **Provider Declaration**: An instance-level statement that a core Provider is available, containing only its explicit Provider name and environment-backed credential source.
- **Profile Provider Choice**: A per-Agent selection of Provider, model, and temperature; it contains no credential.
- **Profile Model Binding**: The isolated association between one Profile and the model configuration produced from the two configuration layers.
- **Model Call Record**: A Session-linked audit entry describing Provider, model, token usage, duration, success status, and an optional redacted failure reason.
- **Tool Description**: Metadata supplied to a model to describe an available capability; it is not executable behavior in this feature.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A routing matrix covering DeepSeek, MiniMax, and two Profiles sharing one Provider produces 100% correct, non-crossed selections.
- **SC-002**: In automated success and failure scenarios, 100% of attempted model calls produce exactly one Session-linked audit record before the caller receives the result.
- **SC-003**: With at least two valid Profiles and one malformed Profile, all valid Profiles remain available and only the malformed Profile is rejected with a clear error.
- **SC-004**: In all Tool-description scenarios, the Provider feature performs zero Tool executions while preserving every returned Tool-call intention.
- **SC-005**: Static and runtime checks find zero plaintext Provider credentials in Profiles, source files, logs, persisted error reasons, or returned errors.
- **SC-006**: An operator can switch a Profile between the two declared Providers or models using configuration and restart alone, with no application code change.

## Assumptions

- The repository's existing application, configuration, CLI, logging, and package foundations from the preceding foundation milestone are available.
- Instance-level Provider declarations are process startup inputs rather than additional workspace initialization artifacts; the workspace artifact count remains unchanged.
- Model-call records use the existing core persistence boundary; this feature introduces no additional business-data category.
- Availability and exact call shapes of the two external Provider integrations will be verified from the locally resolved module versions during planning.
- Real Provider credentials and network access are reserved for explicitly invoked integration smoke tests; normal automated tests use controlled substitutes.
