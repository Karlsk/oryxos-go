package session

import (
	"sync"

	"github.com/Karlsk/oryxos-go/internal/llm"
)

// Session is the minimal in-memory conversation aggregate required by the
// runtime. Lesson 18 owns lookup, persistence, archive state, and ID creation.
type Session struct {
	ID string

	mu       sync.RWMutex
	messages []llm.Message
}

// New constructs a Session with an identity supplied by the Session service.
func New(id string) *Session {
	return &Session{ID: id}
}

// Append adds one complete message to the ordered conversation.
func (session *Session) Append(message llm.Message) {
	if session == nil {
		return
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	session.messages = append(session.messages, cloneMessage(message))
}

// Messages returns a deep defensive copy of the complete conversation.
func (session *Session) Messages() []llm.Message {
	if session == nil {
		return nil
	}
	session.mu.RLock()
	defer session.mu.RUnlock()
	return cloneMessages(session.messages)
}

// RecentTurns returns the newest complete user-rooted turns. Messages before
// the first user message are never returned as conversation history.
func (session *Session) RecentTurns(limit int) []llm.Message {
	if session == nil || limit <= 0 {
		return nil
	}
	session.mu.RLock()
	defer session.mu.RUnlock()

	starts := make([]int, 0)
	for index, message := range session.messages {
		if message.Role == llm.RoleUser {
			starts = append(starts, index)
		}
	}
	if len(starts) == 0 {
		return nil
	}
	startIndex := 0
	if len(starts) > limit {
		startIndex = len(starts) - limit
	}
	return cloneMessages(session.messages[starts[startIndex]:])
}

func cloneMessages(messages []llm.Message) []llm.Message {
	cloned := make([]llm.Message, len(messages))
	for index, message := range messages {
		cloned[index] = cloneMessage(message)
	}
	return cloned
}

func cloneMessage(message llm.Message) llm.Message {
	cloned := message
	cloned.Extra = cloneMap(message.Extra)
	cloned.ToolCalls = make([]llm.ToolCall, len(message.ToolCalls))
	for index, call := range message.ToolCalls {
		cloned.ToolCalls[index] = call
		cloned.ToolCalls[index].Extra = cloneMap(call.Extra)
		if call.Index != nil {
			value := *call.Index
			cloned.ToolCalls[index].Index = &value
		}
	}
	return cloned
}

func cloneMap(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	cloned := make(map[string]any, len(values))
	for key, value := range values {
		cloned[key] = cloneValue(value)
	}
	return cloned
}

func cloneValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneMap(typed)
	case []any:
		cloned := make([]any, len(typed))
		for index, nested := range typed {
			cloned[index] = cloneValue(nested)
		}
		return cloned
	case []string:
		return append([]string(nil), typed...)
	default:
		return value
	}
}
