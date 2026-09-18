package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Karlsk/oryxos-go/internal/llm"
	"github.com/Karlsk/oryxos-go/internal/profile"
	"github.com/Karlsk/oryxos-go/internal/session"
)

// ErrMaxIterations reports that the model continued requesting Tools through
// every iteration allowed by the selected Profile.
var ErrMaxIterations = errors.New("react loop reached maximum iterations")

type chatService interface {
	Chat(ctx context.Context, sessionID string, selected *profile.Profile, messages []llm.Message) (llm.Response, error)
}

type toolExecutor interface {
	Execute(ctx context.Context, sessionID string, allowedTools []string, call llm.ToolCall) (string, error)
}

type promptBuilder interface {
	Build(ctx context.Context, current *session.Session, selected *profile.Profile, currentUserMessage string) ([]llm.Message, error)
}

type modelResponseAvailableError interface {
	ModelResponseAvailable() bool
}

// ReActLoop owns the reason-act-observe control flow.
type ReActLoop struct {
	chat     chatService
	executor toolExecutor
	prompts  promptBuilder
}

// NewReActLoop constructs a synchronous ReAct loop.
func NewReActLoop(chat chatService, executor toolExecutor, prompts promptBuilder) (*ReActLoop, error) {
	if chat == nil {
		return nil, fmt.Errorf("create react loop: chat service is nil")
	}
	if executor == nil {
		return nil, fmt.Errorf("create react loop: tool executor is nil")
	}
	if prompts == nil {
		return nil, fmt.Errorf("create react loop: prompt builder is nil")
	}
	return &ReActLoop{chat: chat, executor: executor, prompts: prompts}, nil
}

// Run appends one user message and executes at most the Profile iteration bound.
func (loop *ReActLoop) Run(ctx context.Context, current *session.Session, userMessage string, selected *profile.Profile) (string, error) {
	if loop == nil || loop.chat == nil || loop.executor == nil || loop.prompts == nil {
		return "", fmt.Errorf("run react loop: loop is not initialized")
	}
	if ctx == nil {
		return "", fmt.Errorf("run react loop: context is nil")
	}
	if current == nil || strings.TrimSpace(current.ID) == "" {
		return "", fmt.Errorf("run react loop: session is required")
	}
	if selected == nil || strings.TrimSpace(selected.Name) == "" {
		return "", fmt.Errorf("run react loop: profile is required")
	}
	if selected.Settings.MaxIterations <= 0 {
		return "", fmt.Errorf("run react loop: max_iterations must be positive")
	}
	current.Append(llm.Message{Role: llm.RoleUser, Content: userMessage})

	for iteration := 0; iteration < selected.Settings.MaxIterations; iteration++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		messages, err := loop.prompts.Build(ctx, current, selected, userMessage)
		if err != nil {
			return "", fmt.Errorf("build prompt: %w", err)
		}
		response, err := loop.chat.Chat(ctx, current.ID, selected, messages)
		if err != nil {
			if modelResponseAvailable(err) {
				current.Append(response.Message)
			}
			return "", err
		}
		current.Append(response.Message)
		if len(response.Message.ToolCalls) == 0 {
			return response.Message.Content, nil
		}
		for _, call := range response.Message.ToolCalls {
			result, executeErr := loop.executor.Execute(ctx, current.ID, selected.Tools, call)
			current.Append(llm.Message{
				Role:       llm.RoleTool,
				Content:    result,
				ToolCallID: call.ID,
				ToolName:   call.Function.Name,
			})
			if executeErr != nil {
				return "", executeErr
			}
		}
	}
	return "", ErrMaxIterations
}

func modelResponseAvailable(err error) bool {
	var marked modelResponseAvailableError
	return errors.As(err, &marked) && marked.ModelResponseAvailable()
}
