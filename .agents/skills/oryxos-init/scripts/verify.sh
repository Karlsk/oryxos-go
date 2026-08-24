#!/usr/bin/env bash
# Read-only verifier for an OryxOS foundation generated with the oryxos-init Skill.
set -euo pipefail

target_root="${1:-.}"
if target_root="$(cd "$target_root" 2>/dev/null && pwd -P 2>/dev/null)"; then
  :
else
  printf 'FAIL verifier.setup: target root is unavailable\n' >&2
  exit 1
fi
if tmpdir="$(mktemp -d 2>/dev/null)"; then
  :
else
  printf 'FAIL verifier.setup: could not create private diagnostics directory\n' >&2
  exit 1
fi
trap 'rm -rf "$tmpdir" >/dev/null 2>&1' EXIT

failures=0

pass() {
  printf 'PASS %s\n' "$1"
}

fail() {
  printf 'FAIL %s: %s\n' "$1" "$2" >&2
  failures=$((failures + 1))
}

require_file() {
  local name="$1"
  local relative_path="$2"
  if [[ -f "$target_root/$relative_path" ]]; then
    pass "$name"
  else
    fail "$name" "missing $relative_path"
  fi
}

require_dir() {
  local name="$1"
  local relative_path="$2"
  if [[ -d "$target_root/$relative_path" ]]; then
    pass "$name"
  else
    fail "$name" "missing directory $relative_path"
  fi
}

require_pattern() {
  local name="$1"
  local pattern="$2"
  shift 2
  local output="$tmpdir/$name.matches"

  if (($# == 0)); then
    fail "$name" "internal verifier error: no search path"
    return
  fi
  if "$RG_BIN" -q --pcre2 -- "$pattern" "$@" >"$output" 2>&1; then
    pass "$name"
  else
    local status=$?
    if ((status == 1)); then
      fail "$name" "required marker not found"
    else
      fail "$name" "search failed (exit $status)"
    fi
  fi
}

require_multiline_pattern() {
  local name="$1"
  local pattern="$2"
  shift 2
  local output="$tmpdir/$name.matches"

  if (($# == 0)); then
    fail "$name" "internal verifier error: no search path"
    return
  fi
  if "$RG_BIN" -q -U --pcre2 -- "$pattern" "$@" >"$output" 2>&1; then
    pass "$name"
  else
    local status=$?
    if ((status == 1)); then
      fail "$name" "required exact contract not found"
    else
      fail "$name" "search failed (exit $status)"
    fi
  fi
}

require_exact_match_count() {
  local name="$1"
  local expected="$2"
  local pattern="$3"
  shift 3
  local output="$tmpdir/$name.matches"
  local status=0
  local count=0

  if (($# == 0)); then
    fail "$name" "internal verifier error: no search path"
    return 1
  fi
  "$RG_BIN" --no-heading --no-filename -o --pcre2 -- "$pattern" "$@" >"$output" 2>&1 || status=$?
  if ((status > 1)); then
    fail "$name" "search failed (exit $status)"
    return 1
  fi
  count="$(wc -l <"$output" | tr -d '[:space:]')"
  if [[ "$count" == "$expected" ]]; then
    pass "$name"
    return 0
  fi
  fail "$name" "expected exactly $expected match(es), found $count"
  return 1
}

require_exact_placeholder() {
  local name="$1"
  local relative_path="$2"
  local package_name="$3"
  local package_summary="$4"
  local behavior_summary="$5"
  local expected="$tmpdir/$name.expected"

  [[ -f "$target_root/$relative_path" ]] || return
  printf '// Package %s %s.\n\n// The oryxos-init Skill creates this package as a compile-safe placeholder.\n// %s behavior is implemented by a later feature specification.\npackage %s\n' \
    "$package_name" "$package_summary" "$behavior_summary" "$package_name" >"$expected"
  if cmp -s "$expected" "$target_root/$relative_path"; then
    pass "$name"
  else
    fail "$name" "placeholder must contain only the exact package documentation and package clause"
  fi
}

reject_pattern() {
  local name="$1"
  local pattern="$2"
  shift 2
  local output="$tmpdir/$name.matches"

  if (($# == 0)); then
    fail "$name" "internal verifier error: no search path"
    return
  fi
  if "$RG_BIN" -l --pcre2 -- "$pattern" "$@" >"$output" 2>&1; then
    # Never print source text or a matching path: this helper also detects credentials.
    fail "$name" "forbidden match"
  else
    local status=$?
    if ((status == 1)); then
      pass "$name"
    else
      # Keep ripgrep diagnostics private: reject scans may include credential-bearing files.
      fail "$name" "search failed (exit $status)"
    fi
  fi
}

require_command() {
  local command_name="$1"
  local setup="$2"
  if command -v "$command_name" >/dev/null 2>&1; then
    return 0
  fi
  fail "tool.$command_name" "missing; $setup"
  return 1
}

run_gate() {
  local name="$1"
  shift
  local output="$tmpdir/$name.out"
  if "$@" >"$output" 2>&1; then
    pass "$name"
  else
    local status=$?
    fail "$name" "command failed (exit $status); rerun the documented gate locally"
  fi
}

prepare_contract_analyzer() {
  local source="$tmpdir/oryxos_contract_analyzer.go"
  local binary="$tmpdir/oryxos-contract-analyzer"
  local output="$tmpdir/contract.analyzer.build.out"

  cat >"$source" <<'GOEOF'
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var requiredTests = []string{
	"TestRouteInventory",
	"TestFoundationRoutesAreOnlyHealthInfo",
	"TestRootCommandHasExactlyTwelveLeaves",
	"TestFoundationCLIBehavior",
	"TestResultSuccessEnvelope",
	"TestResultErrorEnvelope",
	"TestResultInvalidDescriptorFallback",
	"TestResultInvalidNestedDetailsFallback",
	"TestResultEmergencyRequestID",
	"TestResultTimeUTC",
	"TestNoDirectJSONOutsideResponder",
	"TestPageResultEmptyItemsArray",
	"TestPageResultDefaultsAndBounds",
	"TestPageResultMaxInt64TotalPages",
	"TestPageResultInvalidFallback",
}

type selectedSource struct {
	path   string
	isTest bool
}

func main() {
	if len(os.Args) != 3 {
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "source":
		err = inspectSource(os.Args[2])
	case "events":
		err = inspectEvents(os.Args[2])
	default:
		os.Exit(2)
	}
	if err != nil {
		os.Exit(2)
	}
}

func emit(kind, name string, ok bool) {
	fmt.Printf("%s\t%s\t%t\n", kind, name, ok)
}

func inspectSource(root string) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	selected, err := selectBuildableSource(root)
	if err != nil {
		return err
	}
	fset := token.NewFileSet()
	production := make([]*ast.File, 0, len(selected))
	tests := make([]*ast.File, 0, len(selected))
	apiFiles := make([]*ast.File, 0)
	apiDir := filepath.Join(root, "internal", "web", "api")
	for _, source := range selected {
		file, parseErr := parser.ParseFile(fset, source.path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		if source.isTest {
			tests = append(tests, file)
			continue
		}
		production = append(production, file)
		if filepath.Dir(source.path) == apiDir {
			apiFiles = append(apiFiles, file)
		}
	}

	emit("API", "Result", exactStruct(apiFiles, "Result", []fieldContract{
		{name: "Code", kind: "string", tag: `json:"code"`},
		{name: "Message", kind: "string", tag: `json:"message"`},
		{name: "Data", kind: "pointer-T", tag: `json:"data,omitempty"`},
		{name: "Details", kind: "map-string-any", tag: `json:"details,omitempty"`},
		{name: "RequestID", kind: "string", tag: `json:"request_id"`},
	}))
	emit("API", "PageResult", exactStruct(apiFiles, "PageResult", []fieldContract{
		{name: "Items", kind: "slice-T", tag: `json:"items"`},
		{name: "Page", kind: "int", tag: `json:"page"`},
		{name: "PageSize", kind: "int", tag: `json:"page_size"`},
		{name: "Total", kind: "int64", tag: `json:"total"`},
		{name: "TotalPages", kind: "int64", tag: `json:"total_pages"`},
	}))

	directGETs, healthGETs, infoGETs, getSelectors := routeGETs(production)
	emit("ROUTE", "exact-direct-count", directGETs == 2)
	emit("ROUTE", "health", healthGETs == 1)
	emit("ROUTE", "info", infoGETs == 1)
	emit("ROUTE", "no-indirect", getSelectors == directGETs)

	for _, name := range requiredTests {
		count := 0
		valid := false
		for _, file := range tests {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Name.Name != name {
					continue
				}
				count++
				valid = validContractTest(fn)
			}
		}
		emit("TEST", name, count == 1 && valid)
	}
	return nil
}

func selectBuildableSource(root string) ([]selectedSource, error) {
	selected := make([]selectedSource, 0)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path == root {
				return nil
			}
			switch entry.Name() {
			case ".git", "docs", "vendor", "node_modules", ".cache", ".tools":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}
		match, matchErr := build.Default.MatchFile(filepath.Dir(path), entry.Name())
		if matchErr != nil {
			return matchErr
		}
		if match {
			selected = append(selected, selectedSource{
				path:   path,
				isTest: strings.HasSuffix(entry.Name(), "_test.go"),
			})
		}
		return nil
	})
	return selected, err
}

type fieldContract struct {
	name string
	kind string
	tag  string
}

func exactStruct(files []*ast.File, name string, fields []fieldContract) bool {
	count := 0
	valid := false
	for _, file := range files {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok || typeSpec.Name.Name != name {
					continue
				}
				count++
				valid = exactType(typeSpec, fields)
			}
		}
	}
	return count == 1 && valid
}

func exactType(spec *ast.TypeSpec, fields []fieldContract) bool {
	if spec.Assign.IsValid() || spec.TypeParams == nil || len(spec.TypeParams.List) != 1 {
		return false
	}
	typeParam := spec.TypeParams.List[0]
	if len(typeParam.Names) != 1 || typeParam.Names[0].Name != "T" || !ident(typeParam.Type, "any") {
		return false
	}
	structure, ok := spec.Type.(*ast.StructType)
	if !ok || structure.Incomplete || len(structure.Fields.List) != len(fields) {
		return false
	}
	for index, contract := range fields {
		field := structure.Fields.List[index]
		if len(field.Names) != 1 || field.Names[0].Name != contract.name || !typeMatches(field.Type, contract.kind) {
			return false
		}
		if field.Tag == nil {
			return false
		}
		tag, err := strconv.Unquote(field.Tag.Value)
		if err != nil || tag != contract.tag {
			return false
		}
	}
	return true
}

func ident(expression ast.Expr, name string) bool {
	value, ok := expression.(*ast.Ident)
	return ok && value.Name == name
}

func typeMatches(expression ast.Expr, kind string) bool {
	switch kind {
	case "string", "int", "int64":
		return ident(expression, kind)
	case "pointer-T":
		pointer, ok := expression.(*ast.StarExpr)
		return ok && ident(pointer.X, "T")
	case "slice-T":
		slice, ok := expression.(*ast.ArrayType)
		return ok && slice.Len == nil && ident(slice.Elt, "T")
	case "map-string-any":
		mapping, ok := expression.(*ast.MapType)
		return ok && ident(mapping.Key, "string") && ident(mapping.Value, "any")
	default:
		return false
	}
}

func routeGETs(files []*ast.File) (direct, health, info, selectors int) {
	for _, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.SelectorExpr:
				if value.Sel.Name == "GET" {
					selectors++
				}
			case *ast.CallExpr:
				selector, ok := value.Fun.(*ast.SelectorExpr)
				if !ok || selector.Sel.Name != "GET" {
					return true
				}
				direct++
				if len(value.Args) == 0 {
					return true
				}
				literal, ok := value.Args[0].(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					return true
				}
				path, err := strconv.Unquote(literal.Value)
				if err != nil {
					return true
				}
				switch path {
				case "/health":
					health++
				case "/info":
					info++
				}
			}
			return true
		})
	}
	return direct, health, info, selectors
}

func validContractTest(fn *ast.FuncDecl) bool {
	if fn.Recv != nil || fn.Type.TypeParams != nil || fn.Body == nil || len(fn.Body.List) == 0 {
		return false
	}
	if fn.Type.Results != nil && len(fn.Type.Results.List) != 0 {
		return false
	}
	if fn.Type.Params == nil || len(fn.Type.Params.List) != 1 {
		return false
	}
	parameter := fn.Type.Params.List[0]
	if len(parameter.Names) != 1 {
		return false
	}
	pointer, ok := parameter.Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	selector, ok := pointer.X.(*ast.SelectorExpr)
	if !ok || !ident(selector.X, "testing") || selector.Sel.Name != "T" {
		return false
	}
	skipped := false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		switch selector.Sel.Name {
		case "Skip", "Skipf", "SkipNow":
			skipped = true
		}
		return true
	})
	return !skipped
}

func inspectEvents(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	type event struct {
		Action string
		Test   string
	}
	runs := make(map[string]int, len(requiredTests))
	passes := make(map[string]int, len(requiredTests))
	required := make(map[string]struct{}, len(requiredTests))
	for _, name := range requiredTests {
		required[name] = struct{}{}
	}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		var value event
		if err := json.Unmarshal(scanner.Bytes(), &value); err != nil {
			return err
		}
		if _, ok := required[value.Test]; !ok {
			continue
		}
		switch value.Action {
		case "run":
			runs[value.Test]++
		case "pass":
			passes[value.Test]++
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	for _, name := range requiredTests {
		emit("EVENT", name, runs[name] == 1 && passes[name] == 1)
	}
	return nil
}
GOEOF

  if env GO111MODULE=off GOFLAGS= "$GO_BIN" build -o "$binary" "$source" >"$output" 2>&1; then
    CONTRACT_ANALYZER="$binary"
    return 0
  fi
  fail 'contract.semantic_analyzer' 'private semantic analyzer could not be prepared'
  return 1
}

require_analyzer_result() {
  local name="$1"
  local kind="$2"
  local subject="$3"
  local summary="$4"
  if grep -Fqx -- "$kind"$'\t'"$subject"$'\ttrue' "$summary"; then
    pass "$name"
  else
    fail "$name" 'semantic contract check failed'
  fi
}

run_contract_test_gate() {
  local regex="$1"
  local json_output="$tmpdir/contract.named_tests.json"
  local stderr_output="$tmpdir/contract.named_tests.stderr"
  local summary="$tmpdir/contract.named_tests.events"
  local test_status=0
  local analyzer_status=0

  env "GOFLAGS=$readonly_go_flags" "$GO_BIN" test -json ./... \
    -run "^($regex)$" -count=1 >"$json_output" 2>"$stderr_output" || test_status=$?
  "$CONTRACT_ANALYZER" events "$json_output" >"$summary" 2>"$tmpdir/contract.events.stderr" || analyzer_status=$?

  if ((test_status == 0 && analyzer_status == 0)); then
    pass 'contract.named_tests'
  elif ((test_status != 0)); then
    fail 'contract.named_tests' "command failed (exit $test_status); rerun the documented gate locally"
  else
    fail 'contract.named_tests' 'private test-event analysis failed'
  fi

  for test_name in "${contract_test_names[@]}"; do
    if ((analyzer_status == 0)) && grep -Fqx -- 'EVENT'$'\t'"$test_name"$'\ttrue' "$summary"; then
      pass "contract.execution.$test_name"
    else
      fail "contract.execution.$test_name" 'required test did not run and pass exactly once'
    fi
  done
}

run_go_tool_gate() {
  local name="$1"
  local tool_name="$2"
  local setup="$3"
  shift 3
  local output="$tmpdir/$name.out"
  if env "GOFLAGS=$readonly_go_flags" go tool "$tool_name" "$@" >"$output" 2>&1; then
    pass "$name"
    return
  else
    local status=$?
    if grep -Eqi 'no such tool|unknown tool|tool .* not found' "$output"; then
      fail "$name" "missing project-local tool; $setup"
    else
      fail "$name" "command failed (exit $status); rerun the documented gate locally"
    fi
  fi
}

run_goimports_gate() {
  local name="$1"
  shift
  local output="$tmpdir/$name.out"
  local setup="run manually: go get -tool golang.org/x/tools/cmd/goimports@v0.49.0; then run go tool goimports (project-local; do not use go install globally)"

  if env "GOFLAGS=$readonly_go_flags" go tool goimports -l "$@" >"$output" 2>&1; then
    if [[ -s "$output" ]]; then
      fail "$name" "formatting changes required"
    else
      pass "$name"
    fi
  else
    local status=$?
    if grep -Eqi 'no such tool|unknown tool|tool .* not found' "$output"; then
      fail "$name" "missing project-local tool; $setup"
    else
      fail "$name" "command failed (exit $status); rerun the documented gate locally"
    fi
  fi
}

run_lint_gate() {
  local local_binary="$target_root/.tools/bin/golangci-lint"
  local lint_binary=""
  local output="$tmpdir/quality.lint.version.out"
  local setup="run manually: GOBIN=\"$target_root/.tools/bin\" go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 && \"$target_root/.tools/bin/golangci-lint\" version"

  if [[ -x "$local_binary" ]]; then
    lint_binary="$local_binary"
  else
    lint_binary="$(command -v golangci-lint || true)"
  fi
  if [[ -z "$lint_binary" ]]; then
    fail 'tool.golangci-lint' "missing; $setup"
    return
  fi
  if "$lint_binary" version >"$output" 2>&1; then
    :
  else
    local status=$?
    fail 'tool.golangci-lint' "version check failed (exit $status); $setup"
    return
  fi
  if ! grep -Eq '(^|[^0-9])v?2\.12\.2([^0-9]|$)' "$output"; then
    fail 'tool.golangci-lint' "expected v2.12.2; $setup"
    return
  fi
  run_gate 'quality.lint' env "GOFLAGS=$readonly_go_flags" "$lint_binary" run
}

# Prefer an explicitly project-local ripgrep. The verifier never installs it.
RG_BIN="$target_root/.tools/bin/rg"
rg_setup="run manually: cargo install --root \"$target_root/.tools\" ripgrep --version 14.1.1 --features pcre2; then use \"$target_root/.tools/bin/rg\""
if [[ ! -x "$RG_BIN" ]]; then
  RG_BIN="$(command -v rg || true)"
fi
have_rg=true
if [[ -z "$RG_BIN" ]]; then
  fail 'tool.rg' "missing; $rg_setup"
  have_rg=false
elif ! printf 'pcre2-probe\n' | "$RG_BIN" --pcre2 'pcre2-probe' >/dev/null 2>&1; then
  fail 'tool.rg' "lacks PCRE2 support; $rg_setup"
  have_rg=false
fi

GO_BIN="$(command -v go || true)"
if [[ -z "$GO_BIN" && -x /usr/local/go/bin/go ]]; then
  GO_BIN=/usr/local/go/bin/go
  PATH="/usr/local/go/bin:$PATH"
fi
have_go=true
if [[ -z "$GO_BIN" ]]; then
  fail 'tool.go' 'missing; install Go 1.26+ through the project toolchain; this verifier will not install it'
  have_go=false
fi

require_file 'layout.go_mod' 'go.mod'
if [[ ! -f "$target_root/go.mod" ]]; then
  printf 'FAIL verifier: %d named check(s) failed\n' "$failures" >&2
  exit 1
fi
require_file 'layout.go_sum' 'go.sum'
require_file 'layout.main' 'cmd/oryxos/main.go'
require_dir 'layout.docs' 'docs'

for package_dir in \
  internal/app internal/config internal/profile internal/skill internal/bootstrap \
  internal/provider internal/runtime internal/memory internal/session internal/tool \
  internal/tool/builtin internal/tool/mcp internal/sandbox internal/scheduler \
  internal/channel/cli internal/web internal/web/api internal/web/middleware \
  internal/store internal/observability; do
  require_dir "layout.${package_dir//\//_}" "$package_dir"
done

for placeholder in \
  internal/profile/doc.go internal/skill/doc.go internal/bootstrap/doc.go \
  internal/provider/doc.go internal/runtime/doc.go internal/memory/doc.go \
  internal/session/doc.go internal/tool/doc.go internal/tool/builtin/doc.go \
  internal/tool/mcp/doc.go internal/sandbox/doc.go internal/scheduler/doc.go \
  internal/channel/cli/doc.go internal/store/doc.go; do
  require_file "placeholder.${placeholder//\//_}" "$placeholder"
done

require_exact_placeholder 'placeholder.profile_exact' 'internal/profile/doc.go' \
  'profile' 'contains Profile loading and runtime registration' 'Profile'
require_exact_placeholder 'placeholder.skill_exact' 'internal/skill/doc.go' \
  'skill' 'contains loading of Profile-referenced SKILL.md content' 'Skill'
require_exact_placeholder 'placeholder.bootstrap_exact' 'internal/bootstrap/doc.go' \
  'bootstrap' 'contains loading of Bootstrap prompt-context files' 'Bootstrap'
require_exact_placeholder 'placeholder.provider_exact' 'internal/provider/doc.go' \
  'provider' 'contains model-provider factories and adapters' 'Provider'
require_exact_placeholder 'placeholder.runtime_exact' 'internal/runtime/doc.go' \
  'runtime' 'contains the OryxOS Agent runtime and ReAct loop' 'Runtime'
require_exact_placeholder 'placeholder.memory_exact' 'internal/memory/doc.go' \
  'memory' 'contains the Markdown-backed long-term memory store' 'Memory'
require_exact_placeholder 'placeholder.session_exact' 'internal/session/doc.go' \
  'session' 'contains session resolution and persistence coordination' 'Session'
require_exact_placeholder 'placeholder.tool_exact' 'internal/tool/doc.go' \
  'tool' 'contains the OryxOS tool registry and execution boundary' 'Tool'
require_exact_placeholder 'placeholder.builtin_exact' 'internal/tool/builtin/doc.go' \
  'builtin' 'contains OryxOS built-in tool implementations' 'Built-in tool'
require_exact_placeholder 'placeholder.mcp_exact' 'internal/tool/mcp/doc.go' \
  'mcp' 'contains the official MCP Go SDK client adapter' 'MCP tool'
require_exact_placeholder 'placeholder.sandbox_exact' 'internal/sandbox/doc.go' \
  'sandbox' 'contains application-level file, command, and URL validation' 'Sandbox'
require_exact_placeholder 'placeholder.scheduler_exact' 'internal/scheduler/doc.go' \
  'scheduler' 'contains Profile schedule triggering for AgentService' 'Scheduler'
require_exact_placeholder 'placeholder.cli_exact' 'internal/channel/cli/doc.go' \
  'cli' 'contains the core-phase command-line message Channel' 'CLI Channel'
require_exact_placeholder 'placeholder.store_exact' 'internal/store/doc.go' \
  'store' 'contains GORM persistence, migrations, and core call-record stores' 'Store'

for artifact in \
  docs/DevelopmentGuide.md docs/ApiConvention.md .editorconfig Makefile \
  .golangci.yml .pre-commit-config.yaml .github/pull_request_template.md \
  .github/workflows/verify.yml; do
  require_file "quality.${artifact//\//_}" "$artifact"
done

if [[ "$have_rg" == true ]]; then
  require_pattern 'module.declaration' '^module[[:space:]]+[^[:space:]]+$' "$target_root/go.mod"
  require_pattern 'module.go_version' '^go[[:space:]]+1\.26([[:space:]]|$)' "$target_root/go.mod"
  require_pattern 'module.goimports_pin' 'golang\.org/x/tools[[:space:]]+v0\.49\.0' "$target_root/go.mod"
  require_pattern 'module.govulncheck_pin' 'golang\.org/x/vuln[[:space:]]+v1\.7\.0' "$target_root/go.mod"
  require_pattern 'module.gosec_pin' 'github\.com/securego/gosec/v2[[:space:]]+v2\.28\.0' "$target_root/go.mod"
  require_pattern 'module.tool_directive' '^tool[[:space:]]*\(' "$target_root/go.mod"
  require_pattern 'module.goimports_tool' 'golang\.org/x/tools/cmd/goimports' "$target_root/go.mod"
  require_pattern 'module.govulncheck_tool' 'golang\.org/x/vuln/cmd/govulncheck' "$target_root/go.mod"
  require_pattern 'module.gosec_tool' 'github\.com/securego/gosec/v2/cmd/gosec' "$target_root/go.mod"
  if [[ -f "$target_root/.golangci.yml" ]]; then
    require_pattern 'quality.lint_v2' '^version:[[:space:]]*.*2[[:space:]]*$' "$target_root/.golangci.yml"
  fi
  if [[ -f "$target_root/.github/workflows/verify.yml" ]]; then
    require_pattern 'quality.workflow_lint_pin' 'golangci/golangci-lint-action@v9\.3\.0' "$target_root/.github/workflows/verify.yml"
    require_pattern 'quality.workflow_lint_version' 'version:[[:space:]]*v2\.12\.2' "$target_root/.github/workflows/verify.yml"
  fi
  if [[ -f "$target_root/Makefile" ]]; then
    require_pattern 'quality.make_verify_target' '^verify:' "$target_root/Makefile"
    require_pattern 'quality.make_tidy_diff' 'go mod tidy -diff' "$target_root/Makefile"
  fi
fi

production_go_files=()
while IFS= read -r -d '' file_path; do
  production_go_files+=("$file_path")
done < <(
  find "$target_root" \
    \( -path "$target_root/.git" -o -path "$target_root/docs" -o -path "$target_root/vendor" \
       -o -path "$target_root/node_modules" -o -path "$target_root/.cache" -o -path "$target_root/.tools" \) -prune -o \
    -type f -name '*.go' ! -name '*_test.go' -print0 2>"$tmpdir/source.find.out"
)
all_go_files=()
while IFS= read -r -d '' file_path; do
  all_go_files+=("$file_path")
done < <(
  find "$target_root" \
    \( -path "$target_root/.git" -o -path "$target_root/docs" -o -path "$target_root/vendor" \
       -o -path "$target_root/node_modules" -o -path "$target_root/.cache" -o -path "$target_root/.tools" \) -prune -o \
    -type f -name '*.go' -print0 2>"$tmpdir/all-go.find.out"
)
test_go_files=()
while IFS= read -r -d '' file_path; do
  test_go_files+=("$file_path")
done < <(
  find "$target_root" \
    \( -path "$target_root/.git" -o -path "$target_root/docs" -o -path "$target_root/vendor" \
       -o -path "$target_root/node_modules" -o -path "$target_root/.cache" -o -path "$target_root/.tools" \) -prune -o \
    -type f -name '*_test.go' -print0 2>"$tmpdir/tests.find.out"
)
authorable_files=()
while IFS= read -r -d '' file_path; do
  authorable_files+=("$file_path")
done < <(
  find "$target_root" \
    \( -path "$target_root/.git" -o -path "$target_root/docs" -o -path "$target_root/vendor" \
       -o -path "$target_root/node_modules" -o -path "$target_root/.cache" -o -path "$target_root/.tools" \) -prune -o \
    -type f \( -name '*.go' -o -name '*.yaml' -o -name '*.yml' -o -name '*.toml' -o -name '*.json' \
      -o -name '*.env' -o -name '.env' -o -name '*.sh' -o -name 'Makefile' \) -print0 \
    2>"$tmpdir/authorable.find.out"
)

contract_test_names=(
  TestRouteInventory
  TestFoundationRoutesAreOnlyHealthInfo
  TestRootCommandHasExactlyTwelveLeaves
  TestFoundationCLIBehavior
  TestResultSuccessEnvelope
  TestResultErrorEnvelope
  TestResultInvalidDescriptorFallback
  TestResultInvalidNestedDetailsFallback
  TestResultEmergencyRequestID
  TestResultTimeUTC
  TestNoDirectJSONOutsideResponder
  TestPageResultEmptyItemsArray
  TestPageResultDefaultsAndBounds
  TestPageResultMaxInt64TotalPages
  TestPageResultInvalidFallback
)

CONTRACT_ANALYZER=""
semantic_summary="$tmpdir/contract.source.summary"
semantic_ready=false
if [[ "$have_go" == true ]] && prepare_contract_analyzer; then
  if "$CONTRACT_ANALYZER" source "$target_root" >"$semantic_summary" 2>"$tmpdir/contract.source.stderr"; then
    semantic_ready=true
  else
    fail 'contract.semantic_source' 'private semantic source analysis failed'
  fi
else
  fail 'contract.semantic_source' 'Go is required for semantic source analysis'
fi

if [[ "$semantic_ready" == true ]]; then
  require_analyzer_result 'api.result_contract' 'API' 'Result' "$semantic_summary"
  require_analyzer_result 'api.page_result_contract' 'API' 'PageResult' "$semantic_summary"
  require_analyzer_result 'routes.exact_get_registration_count' 'ROUTE' 'exact-direct-count' "$semantic_summary"
  require_analyzer_result 'routes.health_registration' 'ROUTE' 'health' "$semantic_summary"
  require_analyzer_result 'routes.info_registration' 'ROUTE' 'info' "$semantic_summary"
  require_analyzer_result 'routes.no_indirect_get_usage' 'ROUTE' 'no-indirect' "$semantic_summary"
  for test_name in "${contract_test_names[@]}"; do
    require_analyzer_result "contract.$test_name" 'TEST' "$test_name" "$semantic_summary"
  done
else
  fail 'api.result_contract' 'semantic contract check unavailable'
  fail 'api.page_result_contract' 'semantic contract check unavailable'
  fail 'routes.exact_get_registration_count' 'semantic contract check unavailable'
  fail 'routes.health_registration' 'semantic contract check unavailable'
  fail 'routes.info_registration' 'semantic contract check unavailable'
  fail 'routes.no_indirect_get_usage' 'semantic contract check unavailable'
  for test_name in "${contract_test_names[@]}"; do
    fail "contract.$test_name" 'semantic contract check unavailable'
  done
fi

if [[ "$have_rg" == true ]]; then
  if ((${#production_go_files[@]} == 0)); then
    fail 'source.production_go' 'no non-test Go source files found'
  else
    pass 'source.production_go'
    require_pattern 'api.success_responder' 'func[[:space:]]+Success\[T[[:space:]]+any\]' "$target_root/internal/web/api"
    require_pattern 'api.page_responder' 'func[[:space:]]+Page\[T[[:space:]]+any\]' "$target_root/internal/web/api"
    require_pattern 'api.error_responder' 'func[[:space:]]+Error[[:space:]]*\(' "$target_root/internal/web/api"
    require_pattern 'routes.gin_new' 'gin\.New[[:space:]]*\(' "$target_root/internal/web"
    require_pattern 'routes.method_not_allowed' 'HandleMethodNotAllowed[[:space:]]*=[[:space:]]*true' "$target_root/internal/web"
    reject_pattern 'routes.no_mutating_or_streaming_registration' '\.(POST|PUT|PATCH|DELETE|OPTIONS|HEAD|Any|Match|Handle|HandleMethod|Static|StaticFile|StaticFS)[[:space:]]*\(' "${production_go_files[@]}"
    reject_pattern 'routes.no_deferred_foundation_surface' '["`](/api/v1)?/(metrics|sessions|agents|profiles|memory|tools|openapi(\.json)?|swagger|events|stream|ws)["`]' "${production_go_files[@]}"
    reject_pattern 'runtime.no_gin_cobra_gorm_or_eino_ext' '"(github\.com/gin-gonic/gin|github\.com/spf13/cobra|gorm\.io/gorm|github\.com/cloudwego/eino-ext[^" ]*)"' "$target_root/internal/runtime"
    reject_pattern 'web.no_provider_or_store_implementation' '"[^" ]*(internal/provider|internal/store|eino-ext)[^" ]*"' "$target_root/internal/web"
    reject_pattern 'deps.no_mattn_sqlite3' 'github\.com/mattn/go-sqlite3' "$target_root/go.mod" "${production_go_files[@]}"
    reject_pattern 'deps.no_prometheus' 'github\.com/prometheus|prometheus/client_golang' "$target_root/go.mod" "${production_go_files[@]}"
    reject_pattern 'deps.no_eino_adk' 'github\.com/cloudwego/eino/adk|eino/adk' "$target_root/go.mod" "${production_go_files[@]}"
  fi

  # The responder is the sole JSON writer, so rescan the full internal tree without its package.
  internal_non_responder_go=()
  if [[ -d "$target_root/internal" ]]; then
    while IFS= read -r -d '' file_path; do
      internal_non_responder_go+=("$file_path")
    done < <(
      find "$target_root/internal" \
        \( -path "$target_root/internal/web/api" -o -path "$target_root/vendor" \) -prune -o \
        -type f -name '*.go' ! -name '*_test.go' -print0 2>"$tmpdir/internal.find.out"
    )
  fi
  if ((${#internal_non_responder_go[@]} > 0)); then
    reject_pattern 'web.no_direct_gin_json_outside_responder' '\.(JSON|AbortWithStatusJSON)[[:space:]]*\(' "${internal_non_responder_go[@]}"
  else
    fail 'web.no_direct_gin_json_outside_responder' 'no non-responder internal Go files found'
  fi

  if ((${#authorable_files[@]} > 0)); then
    single_quote="'"
    secret_pattern="(?i)(AKIA[0-9A-Z]{16}|(?:ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9_]{20,}|github_pat_[A-Za-z0-9_]{20,}|sk-[A-Za-z0-9_-]{16,}|-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----|\\b(api[_-]?key|secret|password|token|authorization)\\b[[:space:]]*[:=][[:space:]]*[${single_quote}\"][^${single_quote}\"\\$][^${single_quote}\"]{7,}[${single_quote}\"])"
    reject_pattern 'secrets.no_likely_committed_credentials' "$secret_pattern" "${authorable_files[@]}"
  else
    fail 'secrets.no_likely_committed_credentials' 'no authorable files found'
  fi
else
  fail 'static_checks' 'ripgrep is required for source checks; see tool.rg'
fi

if [[ "$have_go" == true ]]; then
  if ! cd "$target_root" 2>"$tmpdir/dynamic.cd.out"; then
    fail 'quality.gates' 'target root became unavailable before dynamic gates'
    printf 'FAIL verifier: %d named check(s) failed\n' "$failures" >&2
    exit 1
  fi
  existing_go_flags=()
  readonly_go_flags=()
  if [[ -n "${GOFLAGS:-}" ]]; then
    read -r -a existing_go_flags <<<"${GOFLAGS}"
    for go_flag in "${existing_go_flags[@]}"; do
      [[ "$go_flag" == -mod=* ]] || readonly_go_flags+=("$go_flag")
    done
  fi
  readonly_go_flags+=('-mod=readonly')
  readonly_go_flags="${readonly_go_flags[*]}"

  if ((${#all_go_files[@]} > 0)); then
    run_gate 'format.gofmt' bash -c 'test -z "$(gofmt -l "$@")"' -- "${all_go_files[@]}"
    run_goimports_gate 'format.goimports' "${all_go_files[@]}"
  else
    fail 'format.gofmt' 'no Go files found'
    fail 'format.goimports' 'no Go files found'
  fi

  run_gate 'module.tidy_diff' go mod tidy -diff
  run_gate 'quality.vet' env "GOFLAGS=$readonly_go_flags" go vet ./...
  contract_test_regex="$(IFS='|'; printf '%s' "${contract_test_names[*]}")"
  if [[ "$semantic_ready" == true ]]; then
    run_contract_test_gate "$contract_test_regex"
  else
    fail 'contract.named_tests' 'semantic analyzer unavailable'
    for test_name in "${contract_test_names[@]}"; do
      fail "contract.execution.$test_name" 'required test did not run and pass exactly once'
    done
  fi
  run_gate 'quality.test' env "GOFLAGS=$readonly_go_flags" go test ./...
  run_gate 'quality.race' env "GOFLAGS=$readonly_go_flags" go test -race ./...

  run_lint_gate
  run_go_tool_gate 'security.gosec' gosec \
    'run manually: go get -tool github.com/securego/gosec/v2/cmd/gosec@v2.28.0 (project-local; do not use go install globally)' \
    ./...
  run_go_tool_gate 'security.govulncheck' govulncheck \
    'run manually: go get -tool golang.org/x/vuln/cmd/govulncheck@v1.7.0 (project-local; do not use go install globally)' \
    ./...
  run_gate 'build.cgo_disabled' env "GOFLAGS=$readonly_go_flags" CGO_ENABLED=0 go build -o "$tmpdir/oryxos" ./cmd/oryxos
else
  fail 'quality.gates' 'Go is unavailable; format, module, test, lint, security, and build gates were not run'
fi

if ((failures == 0)); then
  printf 'PASS verifier: %s satisfies the OryxOS foundation contract\n' "$target_root"
  exit 0
fi

printf 'FAIL verifier: %d named check(s) failed\n' "$failures" >&2
exit 1
