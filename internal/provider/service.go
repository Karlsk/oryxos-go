package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Karlsk/oryxos-go/internal/config"
	"github.com/Karlsk/oryxos-go/internal/llm"
	"github.com/Karlsk/oryxos-go/internal/profile"
	"github.com/Karlsk/oryxos-go/internal/store"
)

// LlmCallRecorder persists one record for each model call attempt.
type LlmCallRecorder interface {
	Create(ctx context.Context, call *store.LlmCall) error
}

// Service performs one synchronous model call for a Profile binding.
type Service struct {
	registry *Registry
	resolver ToolSchemaResolver
	recorder LlmCallRecorder
	now      func() time.Time
}

// ProviderService is the explicit architecture name for the Provider call boundary.
type ProviderService = Service

// NewService constructs the Provider service. Resolver may be nil only for Profiles with no Tools.
func NewService(registry *Registry, resolver ToolSchemaResolver, recorder LlmCallRecorder) (*Service, error) {
	if registry == nil {
		return nil, fmt.Errorf("create provider service: registry is nil")
	}
	if recorder == nil {
		return nil, fmt.Errorf("create provider service: recorder is nil")
	}
	return &Service{registry: registry, resolver: resolver, recorder: recorder, now: time.Now}, nil
}

// NewProviderService constructs the documented ProviderService API.
func NewProviderService(registry *ProviderRegistry, resolver ToolSchemaResolver, recorder LlmCallRecorder) (*ProviderService, error) {
	return NewService(registry, resolver, recorder)
}

// Chat resolves metadata-only Tool definitions, calls the Profile model once,
// audits the outcome, and returns the OryxOS response.
func (service *Service) Chat(ctx context.Context, sessionID string, selected *profile.Profile, messages []llm.Message) (llm.Response, error) {
	if service == nil || service.registry == nil || service.recorder == nil {
		return llm.Response{}, fmt.Errorf("provider chat: service is not initialized")
	}
	if ctx == nil {
		return llm.Response{}, fmt.Errorf("provider chat: context is nil")
	}
	if strings.TrimSpace(sessionID) == "" {
		return llm.Response{}, fmt.Errorf("provider chat: session_id is required")
	}
	if selected == nil || strings.TrimSpace(selected.Name) == "" {
		return llm.Response{}, fmt.Errorf("provider chat: profile is required")
	}
	chatModel, ok := service.registry.Model(selected.Name)
	if !ok {
		return llm.Response{}, fmt.Errorf("provider chat: model binding for profile %q not found", selected.Name)
	}
	var definitions []llm.ToolDefinition
	if len(selected.Tools) > 0 {
		if service.resolver == nil {
			return llm.Response{}, fmt.Errorf("provider chat: tool schema resolver is required")
		}
		var err error
		definitions, err = service.resolver.Resolve(ctx, selected.Tools)
		if err != nil {
			return llm.Response{}, safeWrap("provider chat: resolve tool schemas", err)
		}
	}

	startedAt := service.now()
	response, callErr := chatModel.Generate(ctx, llm.Request{Messages: messages, Tools: definitions})
	finishedAt := service.now()
	duration := finishedAt.Sub(startedAt).Milliseconds()
	if duration < 0 {
		duration = 0
	}

	call := &store.LlmCall{
		SessionID:  sessionID,
		Provider:   selected.Provider.Name,
		Model:      selected.Provider.Model,
		Success:    callErr == nil,
		DurationMS: duration,
		CreatedAt:  finishedAt.UTC(),
	}
	if callErr == nil {
		call.PromptTokens = response.Usage.PromptTokens
		call.CompletionTokens = response.Usage.CompletionTokens
		call.TotalTokens = response.Usage.TotalTokens
	}
	var safeCallErr error
	if callErr != nil {
		safeCallErr = safeWrap("provider call failed", callErr)
		message := safeCallErr.Error()
		call.ErrorMessage = &message
	}

	if persistErr := service.recorder.Create(context.WithoutCancel(ctx), call); persistErr != nil {
		safePersistErr := safeWrap("persist llm call", persistErr)
		if safeCallErr != nil {
			return llm.Response{}, errors.Join(safePersistErr, safeCallErr)
		}
		return llm.Response{}, safePersistErr
	}
	if safeCallErr != nil {
		return llm.Response{}, safeCallErr
	}
	return response, nil
}

type sanitizedWrappedError struct {
	prefix  string
	message string
	cause   error
}

func (wrapped *sanitizedWrappedError) Error() string {
	return wrapped.prefix + ": " + wrapped.message
}

func (wrapped *sanitizedWrappedError) Unwrap() error { return wrapped.cause }

func safeWrap(prefix string, cause error) error {
	if cause == nil {
		return nil
	}
	message := config.SanitizeErrorString(cause.Error())
	wrappedCause := cause
	if message != cause.Error() {
		wrappedCause = nil
	}
	return &sanitizedWrappedError{
		prefix:  prefix,
		message: message,
		cause:   wrappedCause,
	}
}
