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
	chatModel, definitions, err := service.prepareCall(ctx, sessionID, selected, "provider chat")
	if err != nil {
		return llm.Response{}, err
	}

	startedAt := service.now()
	response, callErr := chatModel.Generate(ctx, llm.Request{Messages: messages, Tools: definitions})
	if outcomeErr := service.recordOutcome(ctx, callMetadataFrom(sessionID, selected), startedAt, &response, callErr); outcomeErr != nil {
		return llm.Response{}, outcomeErr
	}
	return response, nil
}

type callMetadata struct {
	sessionID string
	provider  string
	model     string
}

func callMetadataFrom(sessionID string, selected *profile.Profile) callMetadata {
	return callMetadata{sessionID: sessionID, provider: selected.Provider.Name, model: selected.Provider.Model}
}

func (service *Service) prepareCall(ctx context.Context, sessionID string, selected *profile.Profile, operation string) (llm.ChatModel, []llm.ToolDefinition, error) {
	if service == nil || service.registry == nil || service.recorder == nil {
		return nil, nil, fmt.Errorf("%s: service is not initialized", operation)
	}
	if ctx == nil {
		return nil, nil, fmt.Errorf("%s: context is nil", operation)
	}
	if strings.TrimSpace(sessionID) == "" {
		return nil, nil, fmt.Errorf("%s: session_id is required", operation)
	}
	if selected == nil || strings.TrimSpace(selected.Name) == "" {
		return nil, nil, fmt.Errorf("%s: profile is required", operation)
	}
	chatModel, ok := service.registry.Model(selected.Name)
	if !ok {
		return nil, nil, fmt.Errorf("%s: model binding for profile %q not found", operation, selected.Name)
	}
	var definitions []llm.ToolDefinition
	if len(selected.Tools) > 0 {
		if service.resolver == nil {
			return nil, nil, fmt.Errorf("%s: tool schema resolver is required", operation)
		}
		var err error
		definitions, err = service.resolver.Resolve(ctx, selected.Tools)
		if err != nil {
			return nil, nil, safeWrap(operation+": resolve tool schemas", err)
		}
	}
	return chatModel, definitions, nil
}

func (service *Service) recordOutcome(ctx context.Context, metadata callMetadata, startedAt time.Time, response *llm.Response, callErr error) error {
	finishedAt := service.now()
	duration := finishedAt.Sub(startedAt).Milliseconds()
	if duration < 0 {
		duration = 0
	}
	call := &store.LlmCall{
		SessionID:  metadata.sessionID,
		Provider:   metadata.provider,
		Model:      metadata.model,
		Success:    callErr == nil,
		DurationMS: duration,
		CreatedAt:  finishedAt.UTC(),
	}
	if callErr == nil && response != nil {
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
			return errors.Join(safePersistErr, safeCallErr)
		}
		return safePersistErr
	}
	return safeCallErr
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
