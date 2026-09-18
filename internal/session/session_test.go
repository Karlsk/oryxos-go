package session

import (
	"fmt"
	"sync"
	"testing"

	"github.com/Karlsk/oryxos-go/internal/llm"
)

func TestSessionMessagesAreSerializedAndDefensivelyCopied(t *testing.T) {
	s := New("session-1")
	const writers = 64
	var group sync.WaitGroup
	group.Add(writers)
	for index := 0; index < writers; index++ {
		go func(index int) {
			defer group.Done()
			s.Append(llm.Message{Role: llm.RoleUser, Content: fmt.Sprintf("message-%d", index)})
		}(index)
	}
	group.Wait()

	messages := s.Messages()
	if len(messages) != writers {
		t.Fatalf("len(Messages()) = %d, want %d", len(messages), writers)
	}
	messages[0].Content = "mutated"
	messages[0].Extra = map[string]any{"nested": "mutated"}
	if s.Messages()[0].Content == "mutated" {
		t.Fatal("Messages() exposed the backing slice")
	}

	index := 0
	s.Append(llm.Message{
		Role:      llm.RoleAssistant,
		ToolCalls: []llm.ToolCall{{Index: &index, ID: "call-1", Extra: map[string]any{"key": "value"}}},
		Extra:     map[string]any{"nested": map[string]any{"key": "value"}},
	})
	copyOfMessages := s.Messages()
	copyOfMessages[len(copyOfMessages)-1].ToolCalls[0].ID = "changed"
	copyOfMessages[len(copyOfMessages)-1].ToolCalls[0].Extra["key"] = "changed"
	copyOfMessages[len(copyOfMessages)-1].Extra["nested"].(map[string]any)["key"] = "changed"
	got := s.Messages()[len(s.Messages())-1]
	if got.ToolCalls[0].ID != "call-1" || got.ToolCalls[0].Extra["key"] != "value" || got.Extra["nested"].(map[string]any)["key"] != "value" {
		t.Fatalf("nested message state was mutated: %#v", got)
	}
}

func TestSessionRecentTurnsKeepsCompleteToolGroups(t *testing.T) {
	s := New("session-1")
	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: "orphan prefix"},
		{Role: llm.RoleUser, Content: "old"},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "old-call", Function: llm.FunctionCall{Name: "lookup"}}}},
		{Role: llm.RoleTool, ToolCallID: "old-call", ToolName: "lookup", Content: "old-result"},
		{Role: llm.RoleAssistant, Content: "old-answer"},
		{Role: llm.RoleUser, Content: "new"},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "new-call", Function: llm.FunctionCall{Name: "lookup"}}}},
		{Role: llm.RoleTool, ToolCallID: "new-call", ToolName: "lookup", Content: "new-result"},
		{Role: llm.RoleAssistant, Content: "new-answer"},
	}
	for _, message := range messages {
		s.Append(message)
	}

	got := s.RecentTurns(1)
	if len(got) != 4 || got[0].Role != llm.RoleUser || got[0].Content != "new" || got[1].ToolCalls[0].ID != "new-call" || got[2].ToolCallID != "new-call" {
		t.Fatalf("RecentTurns(1) = %#v, want complete newest turn", got)
	}
	if got := s.RecentTurns(0); len(got) != 0 {
		t.Fatalf("RecentTurns(0) = %#v, want empty", got)
	}
}
