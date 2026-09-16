package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Karlsk/oryxos-go/internal/config"
	"github.com/Karlsk/oryxos-go/internal/profile"
	"github.com/Karlsk/oryxos-go/internal/store"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// LlmCallRecorder persists one record for each model call attempt.
type LlmCallRecorder interface {
	Create(ctx context.Context, call *store.LlmCall) error
}

// Service performs one synchronous model call for a Profile binding.
type Service struct {
	registry *Registry
	adapter  ToolSchemaAdapter
	recorder LlmCallRecorder
	now      func() time.Time
}

// ProviderService is the explicit architecture name for the Provider call boundary.
type ProviderService = Service

// NewService constructs the Provider service. Adapter may be nil only for Profiles with no Tools.
func NewService(registry *Registry, adapter ToolSchemaAdapter, recorder LlmCallRecorder) (*Service, error) {
	if registry == nil {
		return nil, fmt.Errorf("create provider service: registry is nil")
	}
	if recorder == nil {
		return nil, fmt.Errorf("create provider service: recorder is nil")
	}
	return &Service{registry: registry, adapter: adapter, recorder: recorder, now: time.Now}, nil
}

// NewProviderService constructs the documented ProviderService API.
func NewProviderService(registry *ProviderRegistry, adapter ToolSchemaAdapter, recorder LlmCallRecorder) (*ProviderService, error) {
	return NewService(registry, adapter, recorder)
}

// Chat binds metadata-only Tool schemas, calls the Profile model once, audits the outcome,
// and returns the connector response unchanged.
func (service *Service) Chat(ctx context.Context, sessionID string, selected *profile.Profile, messages []*schema.Message) (*schema.Message, error) {
	if service == nil || service.registry == nil || service.recorder == nil {
		return nil, fmt.Errorf("provider chat: service is not initialized")
	}
	if ctx == nil {
		return nil, fmt.Errorf("provider chat: context is nil")
	}
	if strings.TrimSpace(sessionID) == "" {
		return nil, fmt.Errorf("provider chat: session_id is required")
	}
	if selected == nil || strings.TrimSpace(selected.Name) == "" {
		return nil, fmt.Errorf("provider chat: profile is required")
	}
	chatModel, ok := service.registry.Model(selected.Name)
	if !ok {
		return nil, fmt.Errorf("provider chat: model binding for profile %q not found", selected.Name)
	}
	if len(selected.Tools) > 0 {
		if service.adapter == nil {
			return nil, fmt.Errorf("provider chat: tool schema adapter is required")
		}
		toolInfos, err := service.adapter.ToEinoToolInfos(ctx, selected.Tools)
		if err != nil {
			return nil, safeWrap("provider chat: resolve tool schemas", err)
		}
		chatModel, err = chatModel.WithTools(toolInfos)
		if err != nil {
			return nil, safeWrap("provider chat: bind tool schemas", err)
		}
	}

	startedAt := service.now()
	response, callErr := chatModel.Generate(
		ctx,
		messages,
		model.WithTemperature(selected.Provider.Temperature),
		model.WithModel(selected.Provider.Model),
	)
	finishedAt := service.now()
	if callErr == nil && response == nil {
		callErr = fmt.Errorf("provider returned nil response")
	}
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
	if callErr == nil && response.ResponseMeta != nil && response.ResponseMeta.Usage != nil {
		call.PromptTokens = response.ResponseMeta.Usage.PromptTokens
		call.CompletionTokens = response.ResponseMeta.Usage.CompletionTokens
		call.TotalTokens = response.ResponseMeta.Usage.TotalTokens
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
			return nil, errors.Join(safePersistErr, safeCallErr)
		}
		return nil, safePersistErr
	}
	if safeCallErr != nil {
		return nil, safeCallErr
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
