package app

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

var placeholderCases = []struct {
	path string
	want string
}{
	{"internal/profile/doc.go", "// Package profile contains Profile loading and runtime registration.\n//\n// The oryxos-init Skill creates this package as a compile-safe placeholder.\n// Profile behavior is implemented by a later feature specification.\npackage profile\n"},
	{"internal/skill/doc.go", "// Package skill contains loading of Profile-referenced SKILL.md content.\n//\n// The oryxos-init Skill creates this package as a compile-safe placeholder.\n// Skill behavior is implemented by a later feature specification.\npackage skill\n"},
	{"internal/bootstrap/doc.go", "// Package bootstrap contains loading of Bootstrap prompt-context files.\n//\n// The oryxos-init Skill creates this package as a compile-safe placeholder.\n// Bootstrap behavior is implemented by a later feature specification.\npackage bootstrap\n"},
	{"internal/provider/doc.go", "// Package provider contains model-provider factories and adapters.\n//\n// The oryxos-init Skill creates this package as a compile-safe placeholder.\n// Provider behavior is implemented by a later feature specification.\npackage provider\n"},
	{"internal/runtime/doc.go", "// Package runtime contains the OryxOS Agent runtime and ReAct loop.\n//\n// The oryxos-init Skill creates this package as a compile-safe placeholder.\n// Runtime behavior is implemented by a later feature specification.\npackage runtime\n"},
	{"internal/memory/doc.go", "// Package memory contains the Markdown-backed long-term memory store.\n//\n// The oryxos-init Skill creates this package as a compile-safe placeholder.\n// Memory behavior is implemented by a later feature specification.\npackage memory\n"},
	{"internal/session/doc.go", "// Package session contains session resolution and persistence coordination.\n//\n// The oryxos-init Skill creates this package as a compile-safe placeholder.\n// Session behavior is implemented by a later feature specification.\npackage session\n"},
	{"internal/tool/doc.go", "// Package tool contains the OryxOS tool registry and execution boundary.\n//\n// The oryxos-init Skill creates this package as a compile-safe placeholder.\n// Tool behavior is implemented by a later feature specification.\npackage tool\n"},
	{"internal/tool/builtin/doc.go", "// Package builtin contains OryxOS built-in tool implementations.\n//\n// The oryxos-init Skill creates this package as a compile-safe placeholder.\n// Built-in tool behavior is implemented by a later feature specification.\npackage builtin\n"},
	{"internal/tool/mcp/doc.go", "// Package mcp contains the official MCP Go SDK client adapter.\n//\n// The oryxos-init Skill creates this package as a compile-safe placeholder.\n// MCP tool behavior is implemented by a later feature specification.\npackage mcp\n"},
	{"internal/sandbox/doc.go", "// Package sandbox contains application-level file, command, and URL validation.\n//\n// The oryxos-init Skill creates this package as a compile-safe placeholder.\n// Sandbox behavior is implemented by a later feature specification.\npackage sandbox\n"},
	{"internal/scheduler/doc.go", "// Package scheduler contains Profile schedule triggering for AgentService.\n//\n// The oryxos-init Skill creates this package as a compile-safe placeholder.\n// Scheduler behavior is implemented by a later feature specification.\npackage scheduler\n"},
	{"internal/channel/cli/doc.go", "// Package cli contains the core-phase command-line message Channel.\n//\n// The oryxos-init Skill creates this package as a compile-safe placeholder.\n// CLI Channel behavior is implemented by a later feature specification.\npackage cli\n"},
	{"internal/store/doc.go", "// Package store contains GORM persistence, migrations, and core call-record stores.\n//\n// The oryxos-init Skill creates this package as a compile-safe placeholder.\n// Store behavior is implemented by a later feature specification.\npackage store\n"},
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func TestArchitectureBusinessPlaceholdersExact(t *testing.T) {
	for _, tc := range placeholderCases {
		t.Run(tc.path, func(t *testing.T) {
			got, err := os.ReadFile(filepath.Join(repositoryRoot(t), tc.path))
			if err != nil {
				t.Fatalf("read placeholder: %v", err)
			}
			if string(got) != tc.want {
				t.Fatalf("placeholder mismatch\nwant:\n%s\ngot:\n%s", tc.want, got)
			}
		})
	}
}
