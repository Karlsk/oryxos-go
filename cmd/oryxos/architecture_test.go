// Package main provides the OryxOS command entry point.
package main

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"golang.org/x/mod/modfile"

	"github.com/Karlsk/oryxos-go/internal/config"
	"github.com/Karlsk/oryxos-go/internal/observability"
	"github.com/Karlsk/oryxos-go/internal/web"
)

func TestArchitectureForbiddenRuntimeImports(t *testing.T) {
	imports := repositoryImports(t)
	for _, path := range imports["internal/runtime"] {
		if hasImportPrefix(path, "github.com/gin-gonic/gin") || hasImportPrefix(path, "github.com/spf13/cobra") || hasImportPrefix(path, "gorm.io/gorm") || hasImportPrefix(path, "github.com/cloudwego/eino-ext") {
			t.Fatalf("internal/runtime imports forbidden dependency %q", path)
		}
	}
	for _, path := range imports["internal/web"] {
		if hasImportPrefix(path, "github.com/cloudwego/eino-ext") || hasImportPrefix(path, "github.com/Karlsk/oryxos-go/internal/provider") || hasImportPrefix(path, "github.com/Karlsk/oryxos-go/internal/store") {
			t.Fatalf("internal/web imports forbidden dependency %q", path)
		}
	}
}

func TestArchitectureDependencyFamiliesAtEdges(t *testing.T) {
	allowed := []struct {
		prefix string
		edge   string
	}{
		{"github.com/gin-gonic/gin", "internal/web"},
		{"github.com/spf13/cobra", "cmd/oryxos"},
		{"gopkg.in/yaml.v3", "internal/config"},
		{"github.com/robfig/cron/v3", "internal/scheduler"},
		{"github.com/cloudwego/eino-ext", "internal/provider"},
		{"github.com/modelcontextprotocol/go-sdk/mcp", "internal/tool/mcp"},
		{"gorm.io/gorm", "internal/store"},
		{"github.com/glebarez/sqlite", "internal/store"},
		{"modernc.org/sqlite", "internal/store"},
	}
	for packagePath, paths := range repositoryImports(t) {
		for _, imported := range paths {
			for _, rule := range allowed {
				if hasImportPrefix(imported, rule.prefix) && packagePath != rule.edge && !strings.HasPrefix(packagePath, rule.edge+"/") {
					t.Fatalf("package %s imports %s outside permitted edge %s", packagePath, imported, rule.edge)
				}
			}
		}
	}
}

func TestArchitectureNoForbiddenRuntimeImports(t *testing.T) {
	for packagePath, paths := range repositoryImports(t) {
		for _, imported := range paths {
			for _, forbidden := range []string{"github.com/mattn/go-sqlite3", "github.com/prometheus/", "github.com/cloudwego/eino/adk"} {
				if hasImportPrefix(imported, forbidden) {
					t.Fatalf("package %s imports forbidden dependency %s", packagePath, imported)
				}
			}
		}
	}
	if err := validateFoundationResiliencePolicy(repositoryRoot(t)); err != nil {
		t.Fatal(err)
	}
}

func TestArchitectureNoForbiddenRuntimeImportsRejectsResilienceFixtures(t *testing.T) {
	for _, test := range []struct {
		name    string
		path    string
		content string
	}{
		{
			name:    "named retry construction",
			path:    "internal/app/retry.go",
			content: "package app\n\nfunc newRetryPolicy() {}\n",
		},
		{
			name:    "circuit breaker state",
			path:    "internal/observability/breaker.go",
			content: "package observability\n\nvar circuitBreakerState int\n",
		},
		{
			name:    "transparent repeated do loop",
			path:    "internal/web/client.go",
			content: "package web\n\ntype client interface { Do() }\n\nfunc send(c client) { for { c.Do() } }\n",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeArchitectureSource(t, root, test.path, test.content)
			if err := validateFoundationResiliencePolicy(root); err == nil {
				t.Fatalf("validateFoundationResiliencePolicy() error = nil, want rejection for %s", test.path)
			}
		})
	}
}

func TestArchitectureNoForbiddenRuntimeImportsAllowsFutureToolExecutorPolicy(t *testing.T) {
	root := t.TempDir()
	writeArchitectureSource(t, root, "internal/tool/executor.go", "package tool\n\nvar retryBackoffPolicy int\n")
	if err := validateFoundationResiliencePolicy(root); err != nil {
		t.Fatalf("validateFoundationResiliencePolicy() error = %v, want future ToolExecutor retry policy excluded", err)
	}
}

func TestRepositoryImportsRestrictToSourceRoots(t *testing.T) {
	root := t.TempDir()
	writeArchitectureSource(t, root, "cmd/example/main.go", "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println() }\n")
	writeArchitectureSource(t, root, "internal/example/example.go", "package example\n\nimport \"strings\"\n\nvar _ = strings.TrimSpace\n")
	writeArchitectureSource(t, root, ".superpowers/snapshot.go", "this is not Go source")

	imports, err := sourceImports(root)
	if err != nil {
		t.Fatalf("sourceImports() error = %v, want artifacts outside cmd/internal ignored", err)
	}
	if got := imports["cmd/example"]; !contains(got, "fmt") {
		t.Fatalf("cmd imports = %v, want fmt", got)
	}
	if got := imports["internal/example"]; !contains(got, "strings") {
		t.Fatalf("internal imports = %v, want strings", got)
	}
	if _, ok := imports[".superpowers"]; ok {
		t.Fatalf("irrelevant artifact imports = %v, want excluded", imports[".superpowers"])
	}
}

func TestNoDirectJSONOutsideResponder(t *testing.T) {
	if err := validateNoDirectJSONOutsideResponder(repositoryRoot(t), filepath.WalkDir); err != nil {
		t.Fatal(err)
	}
}

func TestNoDirectJSONOutsideResponderRejectsNegativeFixtures(t *testing.T) {
	for _, test := range []struct {
		name string
		path string
	}{
		{name: "unrelated api directory", path: "internal/feature/api/handler.go"},
		{name: "nested responder directory", path: "internal/web/api/nested/handler.go"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeArchitectureSource(t, root, test.path, "package fixture\n\nfunc respond(c interface{ JSON() }) { c.JSON() }\n")
			if err := validateNoDirectJSONOutsideResponder(root, filepath.WalkDir); err == nil {
				t.Fatalf("validateNoDirectJSONOutsideResponder() error = nil, want rejection for %s", test.path)
			}
		})
	}

	t.Run("exact responder package allowed", func(t *testing.T) {
		root := t.TempDir()
		writeArchitectureSource(t, root, "internal/web/api/result.go", "package api\n\nfunc respond(c interface{ JSON() }) { c.JSON() }\n")
		if err := validateNoDirectJSONOutsideResponder(root, filepath.WalkDir); err != nil {
			t.Fatalf("validateNoDirectJSONOutsideResponder() error = %v, want exact responder package allowed", err)
		}
	})

	t.Run("parse errors fail closed", func(t *testing.T) {
		root := t.TempDir()
		writeArchitectureSource(t, root, "internal/web/broken.go", "package web\nfunc")
		if err := validateNoDirectJSONOutsideResponder(root, filepath.WalkDir); err == nil {
			t.Fatal("validateNoDirectJSONOutsideResponder() error = nil, want parse error")
		}
	})

	t.Run("walk callback errors fail closed", func(t *testing.T) {
		walkFailed := errors.New("walk failed")
		walk := func(root string, callback fs.WalkDirFunc) error {
			return callback(filepath.Join(root, "blocked.go"), nil, walkFailed)
		}
		if err := validateNoDirectJSONOutsideResponder(t.TempDir(), walk); !errors.Is(err, walkFailed) {
			t.Fatalf("validateNoDirectJSONOutsideResponder() error = %v, want errors.Is(walk failed)", err)
		}
	})
}

func TestFoundationRoutesAreOnlyHealthInfo(t *testing.T) {
	server := web.NewServer(config.ServerConfig{
		ListenAddress: "127.0.0.1:0", ReadHeaderTimeout: 1, ReadTimeout: 1, WriteTimeout: 1, IdleTimeout: 1, ShutdownTimeout: 1,
	}, observability.NewObserver(), observability.NewLogger(os.Stderr, slog.LevelInfo), "test")
	var routes []string
	for _, route := range server.Routes() {
		routes = append(routes, route.Method+" "+route.Path)
	}
	sort.Strings(routes)
	want := []string{"GET /api/v1/health", "GET /api/v1/info"}
	if strings.Join(routes, "\n") != strings.Join(want, "\n") {
		t.Fatalf("routes = %v, want %v", routes, want)
	}
}

func TestQualityToolPins(t *testing.T) {
	root := repositoryRoot(t)

	goMod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}

	lintConfig, err := os.ReadFile(filepath.Join(root, ".golangci.yml"))
	if err != nil {
		t.Fatalf("read .golangci.yml: %v", err)
	}
	if !regexp.MustCompile(`(?m)^version: ["']2["']$`).Match(lintConfig) {
		t.Error(".golangci.yml must declare version: \"2\"")
	}

	workflow, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "verify.yml"))
	if err != nil {
		t.Fatalf("read .github/workflows/verify.yml: %v", err)
	}
	workflowText := string(workflow)
	if err := validateQualityToolPins(string(goMod), workflowText); err != nil {
		t.Error(err)
	}
	for _, required := range []string{
		"actions/checkout@v6.0.2",
		"actions/setup-go@v7.0.0",
		"golangci/golangci-lint-action@v9.3.0",
		"version: v2.12.2",
	} {
		if !strings.Contains(workflowText, required) {
			t.Errorf("workflow missing exact pin %q", required)
		}
	}
}

func validateQualityToolPins(goMod, workflow string) error {
	parsed, err := modfile.Parse("go.mod", []byte(goMod), nil)
	if err != nil {
		return fmt.Errorf("parse go.mod: %w", err)
	}
	requires := map[string]string{}
	for _, require := range parsed.Require {
		requires[require.Mod.Path] = require.Mod.Version
	}
	for path, version := range map[string]string{"github.com/securego/gosec/v2": "v2.28.0", "golang.org/x/tools": "v0.49.0", "golang.org/x/vuln": "v1.7.0"} {
		if requires[path] != version {
			return fmt.Errorf("go.mod requires %s %s, got %q", path, version, requires[path])
		}
	}
	tools := map[string]bool{}
	for _, tool := range parsed.Tool {
		tools[tool.Path] = true
	}
	for _, path := range []string{"github.com/securego/gosec/v2/cmd/gosec", "golang.org/x/tools/cmd/goimports", "golang.org/x/vuln/cmd/govulncheck"} {
		if !tools[path] {
			return fmt.Errorf("go.mod missing tool directive %q", path)
		}
	}
	for _, match := range regexp.MustCompile(`(?m)^\s*-?\s*uses:\s*([^\s#]+)`).FindAllStringSubmatch(workflow, -1) {
		parts := strings.SplitN(match[1], "@", 2)
		if len(parts) != 2 || !regexp.MustCompile(`^(v\d+\.\d+\.\d+|[0-9a-f]{40})$`).MatchString(parts[1]) {
			return fmt.Errorf("workflow contains floating or unpinned action ref %q", match[1])
		}
	}
	return nil
}

func TestQualityToolPinsRejectMalformedFixtures(t *testing.T) {
	validModule := "module example.com/test\n\ngo 1.26.0\n\nrequire (\n\tgithub.com/securego/gosec/v2 v2.28.0 // indirect\n\tgolang.org/x/tools v0.49.0 // indirect\n\tgolang.org/x/vuln v1.7.0 // indirect\n)\n\ntool (\n\tgithub.com/securego/gosec/v2/cmd/gosec\n\tgolang.org/x/tools/cmd/goimports\n\tgolang.org/x/vuln/cmd/govulncheck\n)\n"
	validWorkflow := "- uses: actions/checkout@v6.0.2\n"
	for _, test := range []struct {
		name      string
		goMod     string
		workflow  string
		wantError string
	}{
		{name: "comment does not satisfy module pin", goMod: strings.Replace(validModule, "\tgolang.org/x/tools v0.49.0 // indirect", "\t// golang.org/x/tools v0.49.0 // indirect", 1), workflow: validWorkflow, wantError: "golang.org/x/tools v0.49.0"},
		{name: "unpinned action", goMod: validModule, workflow: "- uses: actions/checkout\n", wantError: "actions/checkout"},
		{name: "major-only action", goMod: validModule, workflow: "- uses: actions/checkout@v6\n", wantError: "actions/checkout@v6"},
		{name: "unsupported non-version action", goMod: validModule, workflow: "- uses: actions/checkout@release-candidate\n", wantError: "actions/checkout@release-candidate"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := validateQualityToolPins(test.goMod, test.workflow); err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("validateQualityToolPins() error = %v, want %q", err, test.wantError)
			}
		})
	}
	if err := validateQualityToolPins(validModule, validWorkflow); err != nil {
		t.Fatalf("validateQualityToolPins() valid fixture error = %v", err)
	}
}

func validateFoundationResiliencePolicy(root string) error {
	for _, packagePath := range []string{"cmd/oryxos", "internal/app", "internal/config", "internal/observability", "internal/web"} {
		sourceRoot := filepath.Join(root, filepath.FromSlash(packagePath))
		err := filepath.WalkDir(sourceRoot, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly|parser.SkipObjectResolution)
			if err != nil {
				return fmt.Errorf("parse resilience policy source %s: %w", path, err)
			}
			for _, imported := range file.Imports {
				if resilienceIdentifier(strings.Trim(imported.Path.Value, "\"")) {
					return fmt.Errorf("foundation package %s imports retry/backoff/circuit-breaker dependency %s", packagePath, imported.Path.Value)
				}
			}

			file, err = parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
			if err != nil {
				return fmt.Errorf("parse resilience policy source %s: %w", path, err)
			}
			var violation error
			ast.Inspect(file, func(node ast.Node) bool {
				if violation != nil {
					return false
				}
				switch typed := node.(type) {
				case *ast.Ident:
					if resilienceIdentifier(typed.Name) {
						violation = fmt.Errorf("foundation package %s constructs retry/backoff/circuit-breaker policy in %s via %q", packagePath, path, typed.Name)
						return false
					}
				case *ast.ForStmt:
					if loopRepeatsOutboundAttempt(typed.Body) {
						violation = fmt.Errorf("foundation package %s repeats outbound attempts in loop in %s", packagePath, path)
						return false
					}
				case *ast.RangeStmt:
					if loopRepeatsOutboundAttempt(typed.Body) {
						violation = fmt.Errorf("foundation package %s repeats outbound attempts in loop in %s", packagePath, path)
						return false
					}
				}
				return true
			})
			return violation
		})
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func resilienceIdentifier(value string) bool {
	normalized := strings.NewReplacer("_", "", "-", "", ".", "", "/", "").Replace(strings.ToLower(value))
	return strings.Contains(normalized, "retry") || strings.Contains(normalized, "backoff") || strings.Contains(normalized, "circuitbreaker") || strings.Contains(normalized, "breaker")
}

func loopRepeatsOutboundAttempt(body *ast.BlockStmt) bool {
	repeats := false
	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if ok && (selector.Sel.Name == "Do" || selector.Sel.Name == "RoundTrip") {
			repeats = true
			return false
		}
		return true
	})
	return repeats
}

type walkDirFunc func(string, fs.WalkDirFunc) error

func validateNoDirectJSONOutsideResponder(root string, walk walkDirFunc) error {
	internalRoot := filepath.Join(root, "internal")
	responderDirectory := filepath.Join(internalRoot, "web", "api")
	if err := walk(internalRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry == nil {
			return fmt.Errorf("walk %s: missing directory entry", path)
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || filepath.Clean(filepath.Dir(path)) == responderDirectory {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		var violation error
		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if selector.Sel.Name == "JSON" || selector.Sel.Name == "AbortWithStatusJSON" {
				violation = fmt.Errorf("direct JSON response call outside responder: %s", path)
				return false
			}
			return true
		})
		return violation
	}); err != nil {
		return fmt.Errorf("scan direct JSON response ownership: %w", err)
	}
	return nil
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func repositoryImports(t *testing.T) map[string][]string {
	t.Helper()
	imports, err := sourceImports(repositoryRoot(t))
	if err != nil {
		t.Fatalf("parse repository imports: %v", err)
	}
	return imports
}

func sourceImports(root string) (map[string][]string, error) {
	imports := make(map[string][]string)
	for _, sourceRoot := range []string{"cmd", "internal"} {
		sourcePath := filepath.Join(root, sourceRoot)
		if err := filepath.WalkDir(sourcePath, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return walkErr
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if err != nil {
				return err
			}
			packagePath := filepath.ToSlash(strings.TrimPrefix(filepath.Dir(path), root+string(filepath.Separator)))
			for _, imported := range file.Imports {
				imports[packagePath] = append(imports[packagePath], strings.Trim(imported.Path.Value, "\""))
			}
			return nil
		}); err != nil {
			return nil, err
		}
	}
	return imports, nil
}

func writeArchitectureSource(t *testing.T, root, path, content string) {
	t.Helper()
	fullPath := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o750); err != nil {
		t.Fatalf("create source directory %s: %v", path, err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write source %s: %v", path, err)
	}
}

func hasImportPrefix(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}
