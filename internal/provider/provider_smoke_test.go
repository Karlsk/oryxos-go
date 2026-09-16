//go:build integration

package provider

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Karlsk/oryxos-go/internal/config"
	"github.com/Karlsk/oryxos-go/internal/llm"
	"github.com/Karlsk/oryxos-go/internal/profile"
	"github.com/Karlsk/oryxos-go/internal/store"
)

func TestProviderSmoke(t *testing.T) {
	cases := []struct {
		name     string
		env      string
		model    string
		provider string
	}{
		{name: "deepseek", env: "DEEPSEEK_API_KEY", model: "deepseek-flash", provider: DeepSeek},
		{name: "minimax", env: "MINIMAX_API_KEY", model: "MiniMax-M3", provider: MiniMax},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			apiKey := os.Getenv(tc.env)
			if apiKey == "" {
				t.Skipf("%s is not set", tc.env)
			}
			registry := NewDefaultRegistry()
			selected := &profile.Profile{
				Name:     "smoke-" + tc.name,
				Provider: profile.ProviderConfig{Name: tc.provider, Model: tc.model, Temperature: 0},
				Tools:    []string{"echo"},
			}
			if err := registry.BindProfile(context.Background(), selected, config.ProviderDefinition{Name: tc.provider, APIKey: apiKey}); err != nil {
				t.Fatalf("BindProfile() error = %v", err)
			}
			source := &fakeToolDefinitionSource{definitions: map[string]llm.ToolDefinition{
				"echo": {
					Name:        "echo",
					Description: "Echo the supplied text. Call this tool for the smoke test.",
					InputSchema: []byte(`{"type":"object","properties":{"text":{"type":"string","description":"text to echo"}},"required":["text"]}`),
				},
			}}
			resolver, _ := NewToolSchemaResolver(source)
			database, err := store.OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "smoke.db"))
			if err != nil {
				t.Fatalf("OpenSQLite() error = %v", err)
			}
			sqlDatabase, err := database.DB()
			if err != nil {
				t.Fatalf("DB() error = %v", err)
			}
			t.Cleanup(func() { _ = sqlDatabase.Close() })
			recorder, err := store.NewLlmCallRepository(database)
			if err != nil {
				t.Fatalf("NewLlmCallRepository() error = %v", err)
			}
			service, err := NewService(registry, resolver, recorder)
			if err != nil {
				t.Fatalf("NewService() error = %v", err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			response, err := service.Chat(ctx, "smoke-session-"+tc.name, selected, []llm.Message{{
				Role:    llm.RoleUser,
				Content: `Call the echo tool exactly once with text "oryxos-smoke". Do not answer directly.`,
			}})
			var calls []store.LlmCall
			if queryErr := database.Order("id").Find(&calls).Error; queryErr != nil {
				t.Fatalf("query llm_calls: %v", queryErr)
			}
			if err != nil {
				if len(calls) != 1 || calls[0].Success {
					t.Fatalf("Chat() error = %v; failure audit = %#v", err, calls)
				}
				t.Fatalf("Chat() error = %v", err)
			}
			if len(response.Message.ToolCalls) == 0 || response.Message.ToolCalls[0].ID == "" {
				t.Fatalf("response = %#v, want preserved Tool call with ID", response)
			}
			if len(calls) != 1 || !calls[0].Success || calls[0].SessionID != "smoke-session-"+tc.name {
				t.Fatalf("success audit = %#v, want one successful durable row", calls)
			}
		})
	}
}
