package provider

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Karlsk/oryxos-go/internal/config"
	"github.com/Karlsk/oryxos-go/internal/llm"
	"github.com/Karlsk/oryxos-go/internal/profile"
	deepseekmodel "github.com/cloudwego/eino-ext/components/model/deepseek"
	openaimodel "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

func TestDefaultFactoriesUseNativeDeepSeekAndFixedMiniMaxEndpoint(t *testing.T) {
	deepseekFake, _ := newFakeModel(&schema.Message{}, nil)
	minimaxFake, _ := newFakeModel(&schema.Message{}, nil)
	var deepseekConfig *deepseekmodel.ChatModelConfig
	var minimaxConfig *openaimodel.ChatModelConfig
	factories := defaultModelFactories(connectorConstructors{
		deepseek: func(_ context.Context, cfg *deepseekmodel.ChatModelConfig) (model.ToolCallingChatModel, error) {
			copy := *cfg
			deepseekConfig = &copy
			return deepseekFake, nil
		},
		openai: func(_ context.Context, cfg *openaimodel.ChatModelConfig) (model.ToolCallingChatModel, error) {
			copy := *cfg
			minimaxConfig = &copy
			return minimaxFake, nil
		},
	})
	deepseekTemperature := float32(0.2)
	minimaxTemperature := float32(0.8)
	if _, err := factories[DeepSeek](context.Background(), ProviderConfig{Name: DeepSeek, APIKey: "ds", Model: "deepseek-chat", Temperature: deepseekTemperature}); err != nil {
		t.Fatalf("deepseek factory error = %v", err)
	}
	if _, err := factories[MiniMax](context.Background(), ProviderConfig{Name: MiniMax, APIKey: "mm", Model: "MiniMax-M3", Temperature: minimaxTemperature}); err != nil {
		t.Fatalf("minimax factory error = %v", err)
	}
	if deepseekConfig == nil || deepseekConfig.BaseURL != "" || deepseekConfig.APIKey != "ds" || deepseekConfig.Model != "deepseek-chat" {
		t.Fatalf("DeepSeek config = %#v, want native default endpoint and supplied settings", deepseekConfig)
	}
	if minimaxConfig == nil || minimaxConfig.BaseURL != "https://api.minimax.cn/v1" || minimaxConfig.APIKey != "mm" || minimaxConfig.Model != "MiniMax-M3" || minimaxConfig.Temperature == nil || *minimaxConfig.Temperature != minimaxTemperature {
		t.Fatalf("MiniMax config = %#v, want fixed endpoint and supplied settings", minimaxConfig)
	}
	if reflect.TypeOf(ProviderConfig{}).NumField() != 4 {
		t.Fatalf("ProviderConfig fields = %d, want endpoint-free four fields", reflect.TypeOf(ProviderConfig{}).NumField())
	}
}

func TestRegistryRoutesAndIsolatesProfiles(t *testing.T) {
	registry := NewRegistry()
	var captured []ProviderConfig
	for _, providerName := range []string{DeepSeek, MiniMax} {
		name := providerName
		if err := registry.RegisterFactory(name, func(_ context.Context, cfg ProviderConfig) (llm.ChatModel, error) {
			captured = append(captured, cfg)
			fake, _ := newFakeOryxModel(llm.Response{Message: llm.Message{Role: llm.RoleAssistant, Content: cfg.Model}}, nil)
			return fake, nil
		}); err != nil {
			t.Fatalf("RegisterFactory(%s) error = %v", name, err)
		}
	}
	profiles := []*profile.Profile{
		{Name: "ops", Provider: profile.ProviderConfig{Name: DeepSeek, Model: "deepseek-chat", Temperature: 0.2}},
		{Name: "reasoning", Provider: profile.ProviderConfig{Name: DeepSeek, Model: "deepseek-reasoner", Temperature: 0.8}},
		{Name: "digest", Provider: profile.ProviderConfig{Name: MiniMax, Model: "MiniMax-M3", Temperature: 0.4}},
	}
	definitions := map[string]config.ProviderDefinition{
		DeepSeek: {Name: DeepSeek, APIKey: "deepseek-key"},
		MiniMax:  {Name: MiniMax, APIKey: "minimax-key"},
	}
	for _, selected := range profiles {
		if err := registry.BindProfile(context.Background(), selected, definitions[selected.Provider.Name]); err != nil {
			t.Fatalf("BindProfile(%s) error = %v", selected.Name, err)
		}
	}
	if len(captured) != 3 || captured[0].Model == captured[1].Model || captured[0].Temperature == captured[1].Temperature || captured[2].Name != MiniMax {
		t.Fatalf("captured configs = %#v, want isolated per-Profile settings", captured)
	}
	for _, name := range []string{"ops", "reasoning", "digest"} {
		if _, ok := registry.Model(name); !ok {
			t.Errorf("Model(%q) = false", name)
		}
	}
	if err := registry.BindProfile(context.Background(), profiles[0], definitions[DeepSeek]); err == nil {
		t.Fatal("duplicate BindProfile() error = nil")
	}
	if err := registry.RegisterFactory(DeepSeek, nil); err == nil {
		t.Fatal("duplicate RegisterFactory() error = nil")
	}
}

func TestRegistryBindProfilesAtomicallyRebuildsCurrentSnapshot(t *testing.T) {
	registry := NewRegistry()
	for _, providerName := range []string{DeepSeek, MiniMax} {
		name := providerName
		if err := registry.RegisterFactory(name, func(_ context.Context, cfg ProviderConfig) (llm.ChatModel, error) {
			fake, _ := newFakeOryxModel(llm.Response{Message: llm.Message{Role: llm.RoleAssistant, Content: cfg.Name + ":" + cfg.Model}}, nil)
			return fake, nil
		}); err != nil {
			t.Fatalf("RegisterFactory(%s) error = %v", name, err)
		}
	}
	directory := t.TempDir()
	profilePath := filepath.Join(directory, "agent.yaml")
	load := func(contents string) *profile.Registry {
		t.Helper()
		if err := os.WriteFile(profilePath, []byte(contents), 0o600); err != nil {
			t.Fatalf("write profile: %v", err)
		}
		loaded, diagnostics, err := (profile.Loader{}).LoadDirectory(directory, map[string]struct{}{DeepSeek: {}, MiniMax: {}})
		if err != nil || len(diagnostics) != 0 {
			t.Fatalf("LoadDirectory() = %v, %v", diagnostics, err)
		}
		return loaded
	}
	definitions := []config.ProviderDefinition{{Name: DeepSeek, APIKey: "ds"}, {Name: MiniMax, APIKey: "mm"}}
	first := load("name: agent\nprovider:\n  name: deepseek\n  model: deepseek-chat\n")
	if err := registry.BindProfiles(context.Background(), first, definitions); err != nil {
		t.Fatalf("first BindProfiles() error = %v", err)
	}
	second := load("name: agent\nprovider:\n  name: minimax\n  model: MiniMax-M3\n")
	if err := registry.BindProfiles(context.Background(), second, definitions); err != nil {
		t.Fatalf("second BindProfiles() error = %v", err)
	}
	bound, ok := registry.Model("agent")
	if !ok {
		t.Fatal("Model(agent) = false")
	}
	response, err := bound.Generate(context.Background(), llm.Request{})
	if err != nil || response.Message.Content != "minimax:MiniMax-M3" {
		t.Fatalf("rebuilt model response = %#v, %v", response, err)
	}
}

func TestProviderServicePreservesToolCallAndNeverExecutesTool(t *testing.T) {
	response := llm.Response{Message: llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "call-1", Type: "function", Function: llm.FunctionCall{Name: "read_file", Arguments: `{"path":"README.md"}`}}}}}
	fake, state := newFakeOryxModel(response, nil)
	registry := NewRegistry()
	_ = registry.RegisterFactory(DeepSeek, func(context.Context, ProviderConfig) (llm.ChatModel, error) { return fake, nil })
	selected := &profile.Profile{Name: "ops", Provider: profile.ProviderConfig{Name: DeepSeek, Model: "deepseek-chat", Temperature: 0.3}, Tools: []string{"read_file"}}
	if err := registry.BindProfile(context.Background(), selected, config.ProviderDefinition{Name: DeepSeek, APIKey: "key"}); err != nil {
		t.Fatalf("BindProfile() error = %v", err)
	}
	source := &fakeToolDefinitionSource{definitions: map[string]llm.ToolDefinition{"read_file": {Name: "read_file", Description: "read"}}}
	resolver, _ := NewToolSchemaResolver(source)
	recorder := &fakeRecorder{}
	service, err := NewService(registry, resolver, recorder)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	ctx := context.WithValue(context.Background(), testContextKey{}, "sentinel")
	got, err := service.Chat(ctx, "session-1", selected, []llm.Message{{Role: llm.RoleUser, Content: "read"}})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if len(got.Message.ToolCalls) != 1 || got.Message.ToolCalls[0].ID != "call-1" {
		t.Fatalf("Chat() response = %#v, want unchanged tool call", got)
	}
	if state.generateCalls != 1 || len(state.requests) != 1 || len(state.requests[0].Tools) != 1 || state.contextObserved != ctx {
		t.Fatalf("model state = %#v, want one bound Generate with original context", state)
	}
	if state.requests[0].Tools[0].Name != "read_file" || len(state.requests[0].Messages) != 1 {
		t.Fatalf("model request = %#v, want OryxOS messages and Tool metadata", state.requests[0])
	}
}

func TestProviderServiceAuditsSuccessFailureAndPersistenceErrors(t *testing.T) {
	t.Run("success_with_usage", func(t *testing.T) {
		response := llm.Response{Message: llm.Message{Role: llm.RoleAssistant, Content: "ok"}, Usage: llm.Usage{PromptTokens: 2, CompletionTokens: 3, TotalTokens: 5}}
		fake, _ := newFakeOryxModel(response, nil)
		service, recorder, selected := serviceForTest(t, fake)
		base := time.Date(2026, 9, 15, 1, 2, 3, 0, time.UTC)
		service.now = sequenceClock(base, base.Add(7*time.Millisecond))
		got, err := service.Chat(context.Background(), "session-success", selected, nil)
		if err != nil || got.Message.Content != response.Message.Content {
			t.Fatalf("Chat() = %#v, %v", got, err)
		}
		if len(recorder.calls) != 1 {
			t.Fatalf("audit calls = %d, want 1", len(recorder.calls))
		}
		call := recorder.calls[0]
		if !call.Success || call.PromptTokens != 2 || call.CompletionTokens != 3 || call.TotalTokens != 5 || call.DurationMS != 7 || !call.CreatedAt.Equal(base.Add(7*time.Millisecond)) {
			t.Fatalf("audit = %#v, want successful usage record", call)
		}
	})

	t.Run("failure_is_sanitized_and_audited", func(t *testing.T) {
		const secret = "sk-provider-secret"
		providerErr := errors.New("upstream rejected api key " + secret)
		fake, _ := newFakeOryxModel(llm.Response{}, providerErr)
		service, recorder, selected := serviceForTest(t, fake)
		_, err := service.Chat(context.Background(), "session-failure", selected, nil)
		if err == nil || strings.Contains(err.Error(), secret) {
			t.Fatalf("Chat() error = %v, want sanitized failure", err)
		}
		if len(recorder.calls) != 1 || recorder.calls[0].Success || recorder.calls[0].ErrorMessage == nil || strings.Contains(*recorder.calls[0].ErrorMessage, secret) {
			t.Fatalf("audit calls = %#v, want sanitized failure", recorder.calls)
		}
		for current := err; current != nil; current = errors.Unwrap(current) {
			if strings.Contains(current.Error(), secret) {
				t.Fatalf("returned error chain leaked secret: %v", current)
			}
		}
	})

	t.Run("missing_usage_records_zero", func(t *testing.T) {
		fake, _ := newFakeOryxModel(llm.Response{Message: llm.Message{Role: llm.RoleAssistant, Content: "ok"}}, nil)
		service, recorder, selected := serviceForTest(t, fake)
		_, err := service.Chat(context.Background(), "session-zero", selected, nil)
		if err != nil {
			t.Fatalf("Chat() error = %v", err)
		}
		call := recorder.calls[0]
		if call.PromptTokens != 0 || call.CompletionTokens != 0 || call.TotalTokens != 0 {
			t.Fatalf("usage = %d/%d/%d, want zeros", call.PromptTokens, call.CompletionTokens, call.TotalTokens)
		}
	})

	t.Run("persistence_error_wins", func(t *testing.T) {
		persistErr := errors.New("insert failed")
		fake, _ := newFakeOryxModel(llm.Response{}, errors.New("provider failed"))
		service, recorder, selected := serviceForTest(t, fake)
		recorder.err = persistErr
		_, err := service.Chat(context.Background(), "session-persist", selected, nil)
		if !errors.Is(err, persistErr) || !strings.Contains(err.Error(), "provider failed") {
			t.Fatalf("Chat() error = %v, want persistence error with provider context", err)
		}
		if len(recorder.calls) != 1 {
			t.Fatalf("audit attempts = %d, want exactly one", len(recorder.calls))
		}
	})

	t.Run("canceled_call_uses_non_canceled_audit_context", func(t *testing.T) {
		fake, _ := newFakeOryxModel(llm.Response{}, context.Canceled)
		service, recorder, selected := serviceForTest(t, fake)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := service.Chat(ctx, "session-canceled", selected, nil)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Chat() error = %v, want context cancellation", err)
		}
		if len(recorder.calls) != 1 || recorder.contextObserved == nil || recorder.contextObserved.Err() != nil {
			t.Fatalf("audit context = %v, calls = %d; want one non-canceled audit attempt", recorder.contextObserved, len(recorder.calls))
		}
	})
}

func TestProviderServiceRejectsInvalidInputsBeforeModelCall(t *testing.T) {
	fake, state := newFakeOryxModel(llm.Response{}, nil)
	service, _, selected := serviceForTest(t, fake)
	cases := []struct {
		name      string
		sessionID string
		profile   *profile.Profile
	}{
		{"empty_session", "", selected},
		{"nil_profile", "session", nil},
		{"unknown_profile", "session", &profile.Profile{Name: "missing", Provider: selected.Provider}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := service.Chat(context.Background(), tc.sessionID, tc.profile, nil); err == nil {
				t.Fatal("Chat() error = nil")
			}
		})
	}
	if state.generateCalls != 0 {
		t.Fatalf("Generate calls = %d, want zero", state.generateCalls)
	}
}

func serviceForTest(t *testing.T, fake llm.ChatModel) (*Service, *fakeRecorder, *profile.Profile) {
	t.Helper()
	registry := NewRegistry()
	if err := registry.RegisterFactory(DeepSeek, func(context.Context, ProviderConfig) (llm.ChatModel, error) { return fake, nil }); err != nil {
		t.Fatalf("RegisterFactory() error = %v", err)
	}
	selected := &profile.Profile{Name: "agent", Provider: profile.ProviderConfig{Name: DeepSeek, Model: "deepseek-chat", Temperature: 0.7}}
	if err := registry.BindProfile(context.Background(), selected, config.ProviderDefinition{Name: DeepSeek, APIKey: "key"}); err != nil {
		t.Fatalf("BindProfile() error = %v", err)
	}
	recorder := &fakeRecorder{}
	service, err := NewService(registry, nil, recorder)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service, recorder, selected
}

func sequenceClock(times ...time.Time) func() time.Time {
	index := 0
	return func() time.Time {
		value := times[index]
		if index < len(times)-1 {
			index++
		}
		return value
	}
}

type testContextKey struct{}
