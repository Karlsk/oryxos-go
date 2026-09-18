# Runtime Contract

## Shared entry point

```go
type AgentRequest struct {
	ProfileName string
	Channel     string
	UserID      string
	SessionID   string
	Message     string
	Stateless   bool
}

type AgentResponse struct {
	SessionID string
	Content   string
}

type AgentService interface {
	Invoke(ctx context.Context, req AgentRequest) (AgentResponse, error)
}
```

`AgentService.Invoke` performs these steps synchronously:

1. Validate the request and resolve the Profile by `ProfileName`.
2. Ask the Session collaboration seam to resolve or create the Session. It never constructs a
   Session ID itself.
3. Pass the explicit Profile and Session to `ReActLoop`.
4. Save the accumulated Session with a cancellation-detached context on both success and failure.
5. If both the loop and save fail, return an error preserving both causes without leaking secrets.
6. On success, return the resolved Session ID and final content.

CLI, Web, and Scheduler callers use this same interface. This lesson tests their request shapes but
does not add entry-point commands, handlers, or scheduler jobs.

## ReAct loop

```go
Run(ctx context.Context, session *session.Session, userMessage string, selected *profile.Profile) (string, error)
```

Behavior:

- Append one user message before iteration.
- Use the Profile maximum iteration count, defaulted and validated by Profile loading.
- Build bounded context and call the existing audited Provider behavior once per iteration.
- Append the complete assistant `llm.Message` before inspecting `ToolCalls`.
- Return assistant content immediately when no Tool calls exist.
- Execute Tool calls in model order and append one matching Tool-role message after each result.
- Return cancellation, model, prompt, Tool, and recording failures without replacing them with a
  successful response.
- If the model succeeds but `llm_calls` persistence fails, retain the complete assistant message,
  return the marked persistence error, and execute none of that message's Tool calls. The runtime
  recognizes this through a narrow response-availability error marker and does not import the
  concrete Provider package.
- Return the stable `ErrMaxIterations` sentinel after exactly the configured call count.

The loop has no Stream method and creates no goroutines.

## Prompt contract

Prompt sections use these labels and precedence:

```text
[RUNTIME_RULES]
[PROJECT_RULES]
[AGENT_IDENTITY]
[SKILLS]
[USER_PREFERENCES]
[LONG_TERM_MEMORY]
[HISTORY]
[USER_MESSAGE]
[CURRENT_DATETIME]
```

Profile Tool definitions accompany the request as OryxOS metadata and are not embedded as Eino
values. Memory is capped at 4,000 characters. History keeps the newest complete
`max_history_turns` user-rooted turns, default 20. The current user turn and Tool-call correlation
groups remain intact.

## Loader contract

- Empty Bootstrap configuration means `AGENTS.md`, `SOUL.md`, `USER.md` in that order.
- Explicit Bootstrap configuration replaces the default list and preserves configured order.
- Missing default templates produce empty labelled sections.
- Missing explicitly referenced Bootstrap or Skill content is an error.
- Each Skill reference is a relative path beneath `.oryxos/skills/`, must resolve to a file named
  `SKILL.md`, and must not duplicate another reference; no directory auto-discovery occurs.
- Only Profile-referenced Skills are loaded; Skills never enter Tool registration.
- Normalized paths must remain within the configured workspace root.
- Returned content is copied into an immutable startup snapshot; no watcher or hot reload exists.

## Session concurrency contract

- Every message read or append on one Session is serialized by that Session.
- Read methods return defensive copies, including nested Tool-call slices and mutable maps.
- Concurrent appends must retain every complete message and pass `go test -race`.
