package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoaderParsesFullProfileAndDefaultsTemperature(t *testing.T) {
	directory := t.TempDir()
	writeProfile(t, directory, "ops.yaml", `name: ops
description: Operations assistant
identity:
  agent_name: Oryx Ops
  prompt: Be careful.
provider:
  name: minimax
  model: MiniMax-M2.7
tools: [read_file]
skills: [ops]
mcp_servers: [news]
notify_channels:
  - name: alerts
    type: webhook
    url: ${ALERT_URL}
schedules:
  - id: daily
    cron: "0 8 * * *"
    timezone: Asia/Shanghai
    message: run
    enabled: true
channels:
  - name: cli
    config:
      prompt: "> "
bootstrap: [AGENTS.md, SOUL.md, USER.md]
settings:
  max_iterations: 10
  max_history_turns: 20
`)

	registry, diagnostics, err := (Loader{LookupEnv: func(name string) (string, bool) {
		return "https://hooks.example.invalid/notify", name == "ALERT_URL"
	}}).LoadDirectory(directory, map[string]struct{}{"minimax": {}})
	if err != nil {
		t.Fatalf("LoadDirectory() error = %v", err)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %v, want none", diagnostics)
	}
	got, ok := registry.Get("ops")
	if !ok {
		t.Fatal("registry.Get(ops) = false")
	}
	if got.Provider.Temperature != 0.7 || got.Identity.AgentName != "Oryx Ops" || len(got.Tools) != 1 || len(got.NotifyChannels) != 1 {
		t.Fatalf("profile = %#v, want full shape and default temperature", got)
	}
	if got.NotifyChannels[0].URL != "https://hooks.example.invalid/notify" {
		t.Fatalf("notify URL = %q, want expanded value", got.NotifyChannels[0].URL)
	}
}

func TestLoaderDefaultsRuntimeSettingsWhenOmitted(t *testing.T) {
	directory := t.TempDir()
	writeProfile(t, directory, "default.yaml", minimalProfile("default", "deepseek", "deepseek-chat", "0.3"))

	registry, diagnostics, err := (Loader{}).LoadDirectory(directory, map[string]struct{}{"deepseek": {}})
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("LoadDirectory() = %v, %v", diagnostics, err)
	}
	got, ok := registry.Get("default")
	if !ok {
		t.Fatal("registry.Get(default) = false")
	}
	if got.Settings.MaxIterations != 10 || got.Settings.MaxHistoryTurns != 20 {
		t.Fatalf("settings = %#v, want defaults 10/20", got.Settings)
	}
}

func TestLoaderIsolatesNonPositiveRuntimeSettings(t *testing.T) {
	cases := []struct {
		name     string
		settings string
		want     string
	}{
		{name: "zero_iterations", settings: "  max_iterations: 0\n", want: "max_iterations"},
		{name: "negative_iterations", settings: "  max_iterations: -1\n", want: "max_iterations"},
		{name: "zero_history", settings: "  max_history_turns: 0\n", want: "max_history_turns"},
		{name: "negative_history", settings: "  max_history_turns: -1\n", want: "max_history_turns"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			directory := t.TempDir()
			content := minimalProfile("bad", "deepseek", "deepseek-chat", "0.3") + "settings:\n" + tc.settings
			writeProfile(t, directory, "bad.yaml", content)

			registry, diagnostics, err := (Loader{}).LoadDirectory(directory, map[string]struct{}{"deepseek": {}})
			if err != nil {
				t.Fatalf("LoadDirectory() fatal error = %v", err)
			}
			if registry.Len() != 0 || len(diagnostics) != 1 {
				t.Fatalf("registry=%d diagnostics=%v, want one isolated diagnostic", registry.Len(), diagnostics)
			}
			if !strings.Contains(diagnostics[0].Error(), tc.want) {
				t.Fatalf("diagnostic = %q, want %q", diagnostics[0], tc.want)
			}
		})
	}
}

func TestLoaderIsolatesMalformedProfilesAndKeepsLexicalDuplicateWinner(t *testing.T) {
	directory := t.TempDir()
	writeProfile(t, directory, "01-first.yaml", minimalProfile("shared", "deepseek", "deepseek-chat", "0.2"))
	writeProfile(t, directory, "02-second.yaml", minimalProfile("shared", "deepseek", "deepseek-reasoner", "0.8"))
	writeProfile(t, directory, "03-valid.yaml", minimalProfile("valid", "minimax", "MiniMax-M2.7", "0.4"))
	writeProfile(t, directory, "04-bad.yaml", "name: bad\nprovider:\n  name: minimax\n  model: x\nunknown: true\n")

	registry, diagnostics, err := (Loader{}).LoadDirectory(directory, map[string]struct{}{"deepseek": {}, "minimax": {}})
	if err != nil {
		t.Fatalf("LoadDirectory() error = %v", err)
	}
	if registry.Len() != 2 {
		t.Fatalf("registry.Len() = %d, want 2", registry.Len())
	}
	shared, _ := registry.Get("shared")
	if shared.Provider.Model != "deepseek-chat" {
		t.Fatalf("duplicate winner model = %q, want first lexical file", shared.Provider.Model)
	}
	if len(diagnostics) != 2 {
		t.Fatalf("diagnostics = %v, want duplicate and malformed diagnostics", diagnostics)
	}
	joined := diagnostics[0].Error() + " " + diagnostics[1].Error()
	for _, want := range []string{"02-second.yaml", "duplicate", "04-bad.yaml", "unknown"} {
		if !strings.Contains(joined, want) {
			t.Errorf("diagnostics %q do not contain %q", joined, want)
		}
	}
}

func TestLoaderRejectsInvalidProviderChoicesPerFile(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want string
	}{
		{"missing_profile_name", "provider:\n  name: deepseek\n  model: deepseek-chat\n", "name"},
		{"missing_provider", "name: bad\nprovider:\n  model: deepseek-chat\n", "provider.name"},
		{"missing_model", "name: bad\nprovider:\n  name: deepseek\n", "provider.model"},
		{"undeclared", minimalProfile("bad", "minimax", "MiniMax-M2.7", "0.7"), "undeclared"},
		{"nan_temperature", minimalProfile("bad", "deepseek", "deepseek-chat", ".nan"), "temperature"},
		{"infinite_temperature", minimalProfile("bad", "deepseek", "deepseek-chat", ".inf"), "temperature"},
		{"legacy_api_key", "name: bad\nprovider:\n  name: deepseek\n  model: deepseek-chat\n  api_key: secret\n", "api_key"},
		{"legacy_base_url", "name: bad\nprovider:\n  name: deepseek\n  model: deepseek-chat\n  base_url: https://example.invalid\n", "base_url"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			directory := t.TempDir()
			writeProfile(t, directory, "bad.yaml", tc.yaml)
			registry, diagnostics, err := (Loader{}).LoadDirectory(directory, map[string]struct{}{"deepseek": {}})
			if err != nil {
				t.Fatalf("LoadDirectory() fatal error = %v", err)
			}
			if registry.Len() != 0 || len(diagnostics) != 1 {
				t.Fatalf("registry=%d diagnostics=%v, want one isolated diagnostic", registry.Len(), diagnostics)
			}
			if !strings.Contains(strings.ToLower(diagnostics[0].Error()), tc.want) {
				t.Fatalf("diagnostic = %q, want text %q", diagnostics[0], tc.want)
			}
			if strings.Contains(diagnostics[0].Error(), "secret") || strings.Contains(diagnostics[0].Error(), "https://example.invalid") {
				t.Fatalf("diagnostic leaked input: %q", diagnostics[0])
			}
		})
	}
}

func TestLoaderReloadBuildsIndependentSnapshot(t *testing.T) {
	directory := t.TempDir()
	path := writeProfile(t, directory, "agent.yaml", minimalProfile("agent", "deepseek", "deepseek-chat", "0.2"))
	providers := map[string]struct{}{"deepseek": {}, "minimax": {}}
	first, diagnostics, err := (Loader{}).LoadDirectory(directory, providers)
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("first LoadDirectory() = %v, %v", diagnostics, err)
	}
	if err := os.WriteFile(path, []byte(minimalProfile("agent", "minimax", "MiniMax-M2.7", "0.8")), 0o600); err != nil {
		t.Fatalf("rewrite profile: %v", err)
	}
	second, diagnostics, err := (Loader{}).LoadDirectory(directory, providers)
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("second LoadDirectory() = %v, %v", diagnostics, err)
	}
	oldProfile, _ := first.Get("agent")
	newProfile, _ := second.Get("agent")
	if oldProfile.Provider.Name != "deepseek" || newProfile.Provider.Name != "minimax" {
		t.Fatalf("snapshots crossed: old=%#v new=%#v", oldProfile.Provider, newProfile.Provider)
	}
}

func TestRegistryReturnsIndependentProfileCopies(t *testing.T) {
	directory := t.TempDir()
	writeProfile(t, directory, "agent.yaml", `name: agent
provider:
  name: deepseek
  model: deepseek-chat
tools: [read_file]
channels:
  - name: cli
    config:
      prompt: "> "
`)
	registry, diagnostics, err := (Loader{}).LoadDirectory(directory, map[string]struct{}{"deepseek": {}})
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("LoadDirectory() = %v, %v", diagnostics, err)
	}

	first, _ := registry.Get("agent")
	first.Provider.Model = "mutated"
	first.Tools[0] = "mutated"
	first.Channels[0].Config["prompt"] = "mutated"

	second, _ := registry.Get("agent")
	if second.Provider.Model != "deepseek-chat" || second.Tools[0] != "read_file" || second.Channels[0].Config["prompt"] != "> " {
		t.Fatalf("registry snapshot was mutated: %#v", second)
	}
	listed := registry.List()
	listed[0].Provider.Model = "listed-mutation"
	third, _ := registry.Get("agent")
	if third.Provider.Model != "deepseek-chat" {
		t.Fatalf("List() exposed mutable snapshot: %#v", third)
	}
}

func writeProfile(t *testing.T, directory, name, content string) string {
	t.Helper()
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	return path
}

func minimalProfile(name, provider, model, temperature string) string {
	return "name: " + name + "\nprovider:\n  name: " + provider + "\n  model: " + model + "\n  temperature: " + temperature + "\n"
}
