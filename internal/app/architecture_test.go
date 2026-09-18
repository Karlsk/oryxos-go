package app

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var placeholderCases = []struct {
	path string
	want string
}{
	{"internal/memory/doc.go", "// Package memory contains the Markdown-backed long-term memory store.\n//\n// The oryxos-init Skill creates this package as a compile-safe placeholder.\n// Memory behavior is implemented by a later feature specification.\npackage memory\n"},
	{"internal/tool/builtin/doc.go", "// Package builtin contains OryxOS built-in tool implementations.\n//\n// The oryxos-init Skill creates this package as a compile-safe placeholder.\n// Built-in tool behavior is implemented by a later feature specification.\npackage builtin\n"},
	{"internal/tool/mcp/doc.go", "// Package mcp contains the official MCP Go SDK client adapter.\n//\n// The oryxos-init Skill creates this package as a compile-safe placeholder.\n// MCP tool behavior is implemented by a later feature specification.\npackage mcp\n"},
	{"internal/sandbox/doc.go", "// Package sandbox contains application-level file, command, and URL validation.\n//\n// The oryxos-init Skill creates this package as a compile-safe placeholder.\n// Sandbox behavior is implemented by a later feature specification.\npackage sandbox\n"},
	{"internal/scheduler/doc.go", "// Package scheduler contains Profile schedule triggering for AgentService.\n//\n// The oryxos-init Skill creates this package as a compile-safe placeholder.\n// Scheduler behavior is implemented by a later feature specification.\npackage scheduler\n"},
	{"internal/channel/cli/doc.go", "// Package cli contains the core-phase command-line message Channel.\n//\n// The oryxos-init Skill creates this package as a compile-safe placeholder.\n// CLI Channel behavior is implemented by a later feature specification.\npackage cli\n"},
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

func TestArchitectureEinoImportsStayInsideProvider(t *testing.T) {
	internalRoot := filepath.Join(repositoryRoot(t), "internal")
	err := filepath.WalkDir(internalRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(internalRoot, path)
		if err != nil {
			return err
		}
		if strings.HasPrefix(relative, "provider"+string(filepath.Separator)) {
			return nil
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range parsed.Imports {
			if strings.Contains(imported.Path.Value, "github.com/cloudwego/eino") {
				t.Errorf("Eino import outside internal/provider: %s imports %s", relative, imported.Path.Value)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan internal imports: %v", err)
	}
}

func TestArchitectureRejectsAutomaticAgentAndForbiddenRuntimeDependencies(t *testing.T) {
	root := repositoryRoot(t)
	internalRoot := filepath.Join(root, "internal")
	err := filepath.WalkDir(internalRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(internalRoot, path)
		if err != nil {
			return err
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range parsed.Imports {
			name := strings.Trim(imported.Path.Value, `"`)
			if strings.Contains(name, "/adk") {
				t.Errorf("automatic Agent dependency is forbidden: %s imports %s", relative, name)
			}
			if strings.HasPrefix(relative, "runtime"+string(filepath.Separator)) {
				for _, forbidden := range []string{"github.com/gin-gonic/gin", "gorm.io/gorm", "github.com/glebarez/sqlite", "github.com/cloudwego/eino"} {
					if strings.HasPrefix(name, forbidden) {
						t.Errorf("runtime framework/storage import is forbidden: %s imports %s", relative, name)
					}
				}
			}
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(content)
		if strings.Contains(text, "AutoMigrate(") {
			t.Errorf("production AutoMigrate is forbidden: %s", relative)
		}
		if strings.Contains(text, "mattn/go-sqlite3") {
			t.Errorf("CGO SQLite driver is forbidden: %s", relative)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan architecture boundaries: %v", err)
	}
}
