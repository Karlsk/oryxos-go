package runtime

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Karlsk/oryxos-go/internal/bootstrap"
	"github.com/Karlsk/oryxos-go/internal/llm"
	"github.com/Karlsk/oryxos-go/internal/profile"
	"github.com/Karlsk/oryxos-go/internal/session"
	"github.com/Karlsk/oryxos-go/internal/skill"
)

func TestPromptBuilderUsesExactPrecedenceMemoryCapAndClock(t *testing.T) {
	boot := bootstrap.NewSnapshot([]bootstrap.Section{
		{Reference: "AGENTS.md", Content: "project rules"},
		{Reference: "SOUL.md", Content: "persona"},
		{Reference: "USER.md", Content: "preferences"},
	})
	skills := skill.NewSnapshot([]skill.Section{{Reference: "weather/SKILL.md", Content: "weather skill"}})
	clock := func() time.Time { return time.Date(2026, 9, 18, 9, 30, 0, 0, time.FixedZone("CST", 8*60*60)) }
	builder, err := NewPromptBuilder(
		"runtime rules",
		map[string]bootstrap.Snapshot{"agent": boot},
		map[string]skill.Snapshot{"agent": skills},
		staticMemory(strings.Repeat("记", 4100)),
		map[string]int{"agent": 10_000},
		clock,
	)
	if err != nil {
		t.Fatalf("NewPromptBuilder() error = %v", err)
	}
	selected := &profile.Profile{Name: "agent", Identity: profile.IdentityConfig{AgentName: "Oryx", Prompt: "identity"}, Settings: profile.SettingsConfig{MaxHistoryTurns: 20}}
	s := session.New("s")
	s.Append(llm.Message{Role: llm.RoleUser, Content: "current"})
	messages, err := builder.Build(context.Background(), s, selected, "current")
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	joined := renderMessages(messages)
	labels := []string{"[RUNTIME_RULES]", "[PROJECT_RULES]", "[AGENT_IDENTITY]", "[SKILLS]", "[USER_PREFERENCES]", "[LONG_TERM_MEMORY]", "[HISTORY]", "[USER_MESSAGE]", "[CURRENT_DATETIME]"}
	last := -1
	for _, label := range labels {
		index := strings.Index(joined, label)
		if index <= last {
			t.Fatalf("label %s at %d after %d in %q", label, index, last, joined)
		}
		last = index
	}
	for _, content := range []string{"runtime rules", "project rules", "identity", "persona", "weather skill", "preferences", "2026-09-18T09:30:00+08:00"} {
		if !strings.Contains(joined, content) {
			t.Errorf("prompt does not contain %q", content)
		}
	}
	memorySection := between(joined, "[LONG_TERM_MEMORY]\n", "\n[HISTORY]")
	if len([]rune(memorySection)) != 4000 {
		t.Fatalf("memory length = %d, want 4000 characters", len([]rune(memorySection)))
	}
}

func TestPromptBuilderKeepsNewestCompleteTurnsAndCurrentRequest(t *testing.T) {
	builder, err := NewPromptBuilder("rules", map[string]bootstrap.Snapshot{"agent": {}}, map[string]skill.Snapshot{"agent": {}}, staticMemory(""), map[string]int{"agent": 10_000}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	s := session.New("s")
	for _, message := range []llm.Message{
		{Role: llm.RoleUser, Content: "old-user"},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "old-call", Function: llm.FunctionCall{Name: "tool"}}}},
		{Role: llm.RoleTool, ToolCallID: "old-call", ToolName: "tool", Content: "old-tool"},
		{Role: llm.RoleAssistant, Content: "old-answer"},
		{Role: llm.RoleUser, Content: "new-user"},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "new-call", Function: llm.FunctionCall{Name: "tool"}}}},
		{Role: llm.RoleTool, ToolCallID: "new-call", ToolName: "tool", Content: "new-tool"},
		{Role: llm.RoleAssistant, Content: "new-answer"},
		{Role: llm.RoleUser, Content: "current"},
	} {
		s.Append(message)
	}
	selected := &profile.Profile{Name: "agent", Settings: profile.SettingsConfig{MaxHistoryTurns: 1}, Tools: []string{"allowed"}}
	messages, err := builder.Build(context.Background(), s, selected, "current")
	if err != nil {
		t.Fatal(err)
	}
	joined := renderMessages(messages)
	if strings.Contains(joined, "old-user") || !strings.Contains(joined, "new-user") || !strings.Contains(joined, "new-call") || !strings.Contains(joined, "new-tool") {
		t.Fatalf("history was not truncated by complete turn: %q", joined)
	}
	currentCount := 0
	for _, message := range messages {
		if message.Role == llm.RoleUser && message.Content == "current" {
			currentCount++
		}
	}
	if currentCount != 1 {
		t.Fatalf("current user message count = %d, want 1", currentCount)
	}
	if selected.Tools[0] != "allowed" {
		t.Fatal("Build() mutated Profile Tool selection")
	}
}

func TestPromptBuilderKeepsCurrentToolTurnOnceAndDropsOnlyOldCompleteTurnsForBudget(t *testing.T) {
	builder, err := NewPromptBuilder(
		"rules",
		map[string]bootstrap.Snapshot{"agent": {}},
		map[string]skill.Snapshot{"agent": {}},
		staticMemory(""),
		map[string]int{"agent": 360},
		func() time.Time { return time.Date(2026, 9, 18, 9, 30, 0, 0, time.UTC) },
	)
	if err != nil {
		t.Fatal(err)
	}
	s := session.New("s")
	for _, message := range []llm.Message{
		{Role: llm.RoleUser, Content: "old-user-" + strings.Repeat("x", 300)},
		{Role: llm.RoleAssistant, Content: "old-answer-" + strings.Repeat("y", 300)},
		{Role: llm.RoleUser, Content: "keep-user"},
		{Role: llm.RoleAssistant, Content: "keep-answer"},
		{Role: llm.RoleUser, Content: "current"},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "current-call", Function: llm.FunctionCall{Name: "lookup", Arguments: `{}`}}}},
		{Role: llm.RoleTool, ToolCallID: "current-call", ToolName: "lookup", Content: "current-result"},
	} {
		s.Append(message)
	}
	selected := &profile.Profile{Name: "agent", Settings: profile.SettingsConfig{MaxHistoryTurns: 20}}

	messages, err := builder.Build(context.Background(), s, selected, "current")
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	joined := renderMessages(messages)
	if strings.Contains(joined, "old-user-") || !strings.Contains(joined, "keep-user") || !strings.Contains(joined, "keep-answer") {
		t.Fatalf("budget did not remove only the oldest complete turn: %q", joined)
	}
	if !strings.Contains(joined, "current-call") || !strings.Contains(joined, "current-result") {
		t.Fatalf("current Tool turn was split: %q", joined)
	}
	currentCount := 0
	for _, message := range messages {
		if message.Role == llm.RoleUser && message.Content == "current" {
			currentCount++
		}
	}
	if currentCount != 1 {
		t.Fatalf("current user message count = %d, want 1", currentCount)
	}
	if got := messagesRuneCount(messages); got > 360 {
		t.Fatalf("prompt rune count = %d, want <= 360", got)
	}
}

func TestPromptBuilderFailsWhenProtectedContextExceedsBudget(t *testing.T) {
	builder, err := NewPromptBuilder(
		"rules",
		map[string]bootstrap.Snapshot{"agent": {}},
		map[string]skill.Snapshot{"agent": {}},
		staticMemory(""),
		map[string]int{"agent": 1},
		time.Now,
	)
	if err != nil {
		t.Fatal(err)
	}
	s := session.New("s")
	s.Append(llm.Message{Role: llm.RoleUser, Content: "current"})
	_, err = builder.Build(context.Background(), s, &profile.Profile{Name: "agent"}, "current")
	if err == nil || !strings.Contains(err.Error(), "protected context") {
		t.Fatalf("Build() error = %v, want protected-context budget failure", err)
	}
}

type staticMemory string

func (memory staticMemory) Read(context.Context, *profile.Profile) (string, error) {
	return string(memory), nil
}

func renderMessages(messages []llm.Message) string {
	var builder strings.Builder
	for _, message := range messages {
		builder.WriteString(message.Content)
		for _, call := range message.ToolCalls {
			builder.WriteString(call.ID)
		}
		builder.WriteString(message.ToolCallID)
		builder.WriteByte('\n')
	}
	return builder.String()
}

func between(value, start, end string) string {
	startIndex := strings.Index(value, start)
	if startIndex < 0 {
		return ""
	}
	startIndex += len(start)
	endIndex := strings.Index(value[startIndex:], end)
	if endIndex < 0 {
		return ""
	}
	return value[startIndex : startIndex+endIndex]
}
