package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLoggerJSONAndCorrelation(t *testing.T) {
	var output bytes.Buffer
	ctx := WithCorrelation(context.Background(), Correlation{
		RequestID: "req-1", SessionID: "session-1", ProfileName: "default", Channel: "http", ScheduleID: "daily",
	})
	Logger(ctx, NewLogger(&output, slog.LevelInfo)).Info("http.request_complete")

	records := jsonRecords(t, output.String())
	if len(records) != 1 {
		t.Fatalf("record count = %d, want 1", len(records))
	}
	for key, want := range map[string]string{
		"request_id": "req-1", "session_id": "session-1", "profile_name": "default", "channel": "http", "schedule_id": "daily",
	} {
		if got := records[0][key]; got != want {
			t.Errorf("record[%q] = %v, want %q", key, got, want)
		}
	}
	if got := records[0]["msg"]; got != "http.request_complete" {
		t.Errorf("message = %v, want http.request_complete", got)
	}
}

func TestLoggerPartialCorrelation(t *testing.T) {
	var output bytes.Buffer
	ctx := WithCorrelation(context.Background(), Correlation{RequestID: "req-1"})
	Logger(ctx, NewLogger(&output, slog.LevelInfo)).Info("http.request_complete")

	record := jsonRecords(t, output.String())[0]
	if got := record["request_id"]; got != "req-1" {
		t.Errorf("request_id = %v, want req-1", got)
	}
	for _, key := range []string{"session_id", "profile_name", "channel", "schedule_id"} {
		if _, exists := record[key]; exists {
			t.Errorf("unexpected %s in partial correlation record", key)
		}
	}
}

func TestObserverHTTPDurationAndStatus(t *testing.T) {
	observer := NewObserver()
	observer.ObserveHTTP(context.Background(), "GET", "/api/v1/health", 200, 20*time.Millisecond)
	observer.ObserveHTTP(context.Background(), "GET", "/api/v1/health", 200, 30*time.Millisecond)

	snapshot := observer.Snapshot()
	if len(snapshot.HTTPRequests) != 1 {
		t.Fatalf("request aggregate count = %d, want 1", len(snapshot.HTTPRequests))
	}
	request := snapshot.HTTPRequests[0]
	if request.Method != "GET" || request.Route != "/api/v1/health" || request.Status != 200 {
		t.Errorf("request aggregate = %#v, want GET /api/v1/health 200", request)
	}
	if request.Count != 2 || request.TotalDuration != 50*time.Millisecond {
		t.Errorf("request aggregate count/duration = %d/%s, want 2/50ms", request.Count, request.TotalDuration)
	}
}

func TestObserverUsesRouteTemplate(t *testing.T) {
	const rawRequestTarget = "/api/v1/sessions/secret-session?token=nope"
	const matchedRouteTemplate = "/api/v1/sessions/:id"

	observer := NewObserver()
	// Middleware derives matchedRouteTemplate from rawRequestTarget before this boundary.
	observer.ObserveHTTP(context.Background(), "GET", matchedRouteTemplate, 200, time.Millisecond)

	snapshot := observer.Snapshot()
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	if got := snapshot.HTTPRequests[0].Route; got != matchedRouteTemplate {
		t.Errorf("route = %q, want template", got)
	}
	for _, forbidden := range []string{rawRequestTarget, "secret-session", "token=nope"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Errorf("snapshot contains raw request value %q", forbidden)
		}
	}
}

func TestObserverReadiness(t *testing.T) {
	observer := NewObserver()
	if observer.Snapshot().Ready {
		t.Error("initial readiness = true, want false")
	}
	observer.SetReady(true)
	if !observer.Snapshot().Ready {
		t.Error("readiness after SetReady(true) = false")
	}
	observer.SetReady(false)
	if observer.Snapshot().Ready {
		t.Error("readiness after SetReady(false) = true")
	}
}

func TestLoggerSanitizesError(t *testing.T) {
	const keySecret = "top-secret"
	const urlSecret = "very-secret-token"
	var output bytes.Buffer
	logger := NewLogger(&output, slog.LevelInfo)
	logger.LogAttrs(context.Background(), slog.LevelError, "app.start_failed", slog.Any("error", errors.New("api_key="+keySecret)))
	logger.LogAttrs(context.Background(), slog.LevelError, "app.start_failed", slog.String("error", "request failed: https://hooks.example.invalid/a/"+urlSecret))

	assertNoSecretsInJSON(t, output.String(), keySecret, urlSecret)
}

func TestLoggerSanitizesDirectAndAttachedAttributes(t *testing.T) {
	const secret = "top-secret"
	var output bytes.Buffer
	logger := NewLogger(&output, slog.LevelInfo)
	logger.LogAttrs(context.Background(), slog.LevelInfo, "app.start_failed",
		slog.String("api_key", secret), slog.String("webhook_url", "https://hooks.example.invalid/"+secret),
		slog.String("mcp_auth", secret), slog.String("authorization", "Bearer "+secret),
		slog.Group("provider", slog.String("token", secret)),
	)
	logger.With(slog.String("api_key", secret), slog.Any("error", errors.New("api_key="+secret))).Info("app.start_failed")

	assertNoSecretsInJSON(t, output.String(), secret)
}

func TestLoggerSanitizesGroupedAttributes(t *testing.T) {
	const secret = "top-secret"
	var output bytes.Buffer
	logger := NewLogger(&output, slog.LevelInfo)
	logger.WithGroup("provider").With("api_key", secret).Info("app.start_failed")
	logger.LogAttrs(context.Background(), slog.LevelInfo, "app.start_failed", slog.Group("outer", slog.Group("inner", slog.String("token", secret))))
	logger.WithGroup("secret").With("value", secret).Info("app.start_failed")

	assertNoSecretsInJSON(t, output.String(), secret)
}

func TestLoggerConsoleFormatAndCorrelation(t *testing.T) {
	var output bytes.Buffer
	ctx := WithCorrelation(context.Background(), Correlation{
		RequestID: "req-1", SessionID: "session-1", ProfileName: "default", Channel: "http", ScheduleID: "daily",
	})
	Logger(ctx, NewConsoleLogger(&output, slog.LevelInfo)).Info("http.request_complete")

	line := strings.TrimSpace(output.String())
	if strings.HasPrefix(line, "{") {
		t.Fatalf("console line is JSON: %q", line)
	}
	for _, want := range []string{"INFO", "http.request_complete", "req-1", "session-1", "default", "http", "daily"} {
		if !strings.Contains(line, want) {
			t.Errorf("console line %q does not contain %q", line, want)
		}
	}
}

func TestLoggerConsoleModeRedacts(t *testing.T) {
	const secret = "top-secret"
	const urlSecret = "very-secret-token"
	var output bytes.Buffer
	logger := NewConsoleLogger(&output, slog.LevelInfo)
	logger.LogAttrs(context.Background(), slog.LevelError, "app.start_failed", slog.Any("error", errors.New("api_key="+secret)))
	logger.LogAttrs(context.Background(), slog.LevelInfo, "app.start_failed", slog.String("webhook_url", "https://hooks.example.invalid/a/"+urlSecret), slog.Group("provider", slog.String("token", secret)))
	logger.With(slog.String("api_key", secret)).Info("app.start_failed")

	for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		for _, forbidden := range []string{secret, urlSecret} {
			if strings.Contains(line, forbidden) {
				t.Errorf("console log contains secret %q: %q", forbidden, line)
			}
		}
	}
}

func TestLoggerFailsClosedForArbitraryErrorsAcrossSinks(t *testing.T) {
	const (
		bearerSecret = "bearer-secret"
		basicSecret  = "dXNlcjpiYXNpYy1zZWNyZXQ="
		pathSecret   = "webhook-path-secret"
		querySecret  = "query-secret"
		opaqueSecret = "AbCdEf0123456789AbCdEf"
		joinedSecret = "joined-secret"
	)

	for _, sink := range []struct {
		name      string
		newLogger func(*bytes.Buffer) *slog.Logger
	}{
		{name: "json", newLogger: func(output *bytes.Buffer) *slog.Logger { return NewLogger(output, slog.LevelInfo) }},
		{name: "console", newLogger: func(output *bytes.Buffer) *slog.Logger { return NewConsoleLogger(output, slog.LevelInfo) }},
	} {
		t.Run(sink.name, func(t *testing.T) {
			var output bytes.Buffer
			logger := sink.newLogger(&output)

			logger.LogAttrs(context.Background(), slog.LevelError, "app.shutdown_failed",
				slog.String("error", "upstream rejected Bearer "+bearerSecret),
				slog.Any("secondary_error", errors.New("upstream rejected Basic "+basicSecret)),
			)
			logger.LogAttrs(context.Background(), slog.LevelError, "app.shutdown_failed",
				slog.Any("error", errors.Join(errors.New("close failed"), errors.New(joinedSecret))),
				slog.String("endpoint", "https://hooks.example.invalid/a/"+pathSecret),
			)
			logger.With(
				slog.String("error", "upstream rejected Basic "+basicSecret),
				slog.String("endpoint", "https://api.example.invalid/v1?access_token="+querySecret),
			).Info("app.shutdown_failed")
			logger.WithGroup("transport").With(
				slog.Any("error", errors.New("upstream rejected Bearer "+bearerSecret)),
				slog.String("endpoint", "https://api.example.invalid/v1/"+opaqueSecret),
			).Info("app.shutdown_failed")

			for _, secret := range []string{bearerSecret, basicSecret, pathSecret, querySecret, opaqueSecret, joinedSecret} {
				if strings.Contains(output.String(), secret) {
					t.Errorf("%s log contains secret %q: %s", sink.name, secret, output.String())
				}
			}
			if !strings.Contains(output.String(), "[REDACTED]") {
				t.Fatalf("%s log contains no redaction marker: %s", sink.name, output.String())
			}
		})
	}
}

func TestLoggerRedactsCredentialURLsFromEveryJSONAttributePath(t *testing.T) {
	const credentialURL = "https://log-user:log-password@example.invalid/private"
	var output bytes.Buffer
	logger := NewLogger(&output, slog.LevelInfo)

	logger.LogAttrs(context.Background(), slog.LevelInfo, "app.start_failed", slog.String("endpoint", credentialURL))
	logger.With(slog.String("endpoint", credentialURL)).Info("app.start_failed")
	logger.WithGroup("transport").With("endpoint", credentialURL).Info("app.start_failed")

	assertCredentialURLFullyRedactedJSON(t, output.String(), credentialURL)
}

func TestLoggerRedactsCredentialURLsFromEveryConsoleAttributePath(t *testing.T) {
	const credentialURL = "https://log-user:log-password@example.invalid/private"
	var output bytes.Buffer
	logger := NewConsoleLogger(&output, slog.LevelInfo)

	logger.LogAttrs(context.Background(), slog.LevelInfo, "app.start_failed", slog.String("endpoint", credentialURL))
	logger.With(slog.String("endpoint", credentialURL)).Info("app.start_failed")
	logger.WithGroup("transport").With("endpoint", credentialURL).Info("app.start_failed")

	assertCredentialURLFullyRedactedConsole(t, output.String(), credentialURL)
}

func TestLoggerRedactsTypedCredentialURLsFromEveryAttributePath(t *testing.T) {
	credentialURL := mustParseURL(t, "https://log-user:log-password@example.invalid/private")
	safeURL := mustParseURL(t, "https://safe.invalid/public")

	for _, loggerFactory := range []struct {
		name       string
		newLogger  func(*bytes.Buffer) *slog.Logger
		assertLogs func(*testing.T, string, string)
	}{
		{
			name: "json",
			newLogger: func(output *bytes.Buffer) *slog.Logger {
				return NewLogger(output, slog.LevelInfo)
			},
			assertLogs: assertCredentialURLFullyRedactedJSON,
		},
		{
			name: "console",
			newLogger: func(output *bytes.Buffer) *slog.Logger {
				return NewConsoleLogger(output, slog.LevelInfo)
			},
			assertLogs: assertCredentialURLFullyRedactedConsole,
		},
	} {
		t.Run(loggerFactory.name, func(t *testing.T) {
			var output bytes.Buffer
			logger := loggerFactory.newLogger(&output)

			logger.LogAttrs(context.Background(), slog.LevelInfo, "app.start_failed",
				slog.Any("endpoint_value", credentialURL),
				slog.Any("safe_endpoint", safeURL),
			)
			logger.With(slog.Any("endpoint_pointer", &credentialURL)).Info("app.start_failed")
			logger.WithGroup("transport").WithGroup("retry").With(slog.Any("endpoint_group", &credentialURL)).Info("app.start_failed")

			loggerFactory.assertLogs(t, output.String(), credentialURL.String())
			if !strings.Contains(output.String(), safeURL.Host) {
				t.Errorf("safe typed URL was not preserved: %s", output.String())
			}
		})
	}
}

func TestConsoleHandlerPreservesAttributeGroupChainingOrder(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(NewConsoleHandler(&output, slog.LevelInfo))

	logger.With("component", "api").WithGroup("http").With("method", "GET").WithGroup("request").With("id", "req-1").Info("http.request_complete")

	line := strings.TrimSpace(output.String())
	assertConsoleAttributeOrder(t, line, "component=api", "http.method=GET", "http.request.id=req-1")
	for _, forbidden := range []string{"http.component=api", "http.request.component=api", "http.request.method=GET"} {
		if strings.Contains(line, forbidden) {
			t.Errorf("attribute was assigned to a later group %q: %s", forbidden, line)
		}
	}
}

func TestConsoleHandlerEscapesControlCharactersAcrossAttributePaths(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(NewConsoleHandler(&output, slog.LevelInfo)).
		With("attached\nkey", "attached\rvalue").
		WithGroup("group\nname").
		With("grouped\tkey", "grouped\nvalue")

	logger.LogAttrs(context.Background(), slog.LevelInfo, "event\nforged",
		slog.String("direct\rkey", "direct\nvalue"))

	got := output.String()
	if strings.Count(got, "\n") != 1 || !strings.HasSuffix(got, "\n") {
		t.Fatalf("console record spans physical lines: %q", got)
	}
	if strings.Contains(strings.TrimSuffix(got, "\n"), "\r") {
		t.Fatalf("console record contains raw carriage return: %q", got)
	}
	for _, escaped := range []string{`event\nforged`, `attached\nkey`, `attached\rvalue`, `group\nname`, `grouped\tkey`, `grouped\nvalue`, `direct\rkey`, `direct\nvalue`} {
		if !strings.Contains(got, escaped) {
			t.Errorf("console record %q does not contain escaped value %q", got, escaped)
		}
	}
}

func TestObserverConcurrentAccess(t *testing.T) {
	observer := NewObserver()
	const workers = 24
	start := make(chan struct{})
	var wait sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wait.Add(1)
		go func(worker int) {
			defer wait.Done()
			<-start
			for iteration := 0; iteration < 100; iteration++ {
				observer.ObserveHTTP(context.Background(), "GET", "/api/v1/health", 200+worker%2, time.Duration(iteration)*time.Microsecond)
				observer.SetReady(iteration%2 == 0)
				_ = observer.Snapshot()
			}
		}(worker)
	}
	close(start)
	wait.Wait()
	if len(observer.Snapshot().HTTPRequests) != 2 {
		t.Errorf("request aggregate count = %d, want 2", len(observer.Snapshot().HTTPRequests))
	}
}

func TestNoPrometheusSurface(t *testing.T) {
	moduleRoot := moduleRoot(t)
	assertNoPrometheusModuleDependency(t, filepath.Join(moduleRoot, "go.mod"))
	assertNoPrometheusImportsOrRegistration(t, filepath.Join(moduleRoot, "internal"))
}

func jsonRecords(t *testing.T, output string) []map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	records := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("unmarshal log line %q: %v", line, err)
		}
		records = append(records, record)
	}
	return records
}

func assertNoSecretsInJSON(t *testing.T, output string, secrets ...string) {
	t.Helper()
	for _, record := range jsonRecords(t, output) {
		encoded, err := json.Marshal(record)
		if err != nil {
			t.Fatalf("marshal parsed log record: %v", err)
		}
		for _, secret := range secrets {
			if strings.Contains(string(encoded), secret) {
				t.Errorf("log record contains secret %q: %s", secret, encoded)
			}
		}
	}
}

func assertCredentialURLFullyRedactedJSON(t *testing.T, output, credentialURL string) {
	t.Helper()
	for _, record := range jsonRecords(t, output) {
		encoded, err := json.Marshal(record)
		if err != nil {
			t.Fatalf("marshal parsed log record: %v", err)
		}
		assertCredentialURLFullyRedacted(t, string(encoded), credentialURL)
	}
}

func assertCredentialURLFullyRedactedConsole(t *testing.T, output, credentialURL string) {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		assertCredentialURLFullyRedacted(t, line, credentialURL)
	}
}

func assertCredentialURLFullyRedacted(t *testing.T, output, credentialURL string) {
	t.Helper()
	for _, forbidden := range []string{credentialURL, "log-user", "log-password", "example.invalid"} {
		if strings.Contains(output, forbidden) {
			t.Errorf("log output contains credential URL data %q: %s", forbidden, output)
		}
	}
	if !strings.Contains(output, "[REDACTED]") {
		t.Errorf("log output does not fully redact credential URL: %s", output)
	}
}

func mustParseURL(t *testing.T, rawURL string) url.URL {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse URL %q: %v", rawURL, err)
	}
	return *parsed
}

func assertConsoleAttributeOrder(t *testing.T, line string, attributes ...string) {
	t.Helper()
	previous := -1
	for _, attribute := range attributes {
		position := strings.Index(line, attribute)
		if position < 0 {
			t.Errorf("console line is missing %q: %s", attribute, line)
			continue
		}
		if position < previous {
			t.Errorf("console attribute %q is out of order: %s", attribute, line)
		}
		previous = position
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("could not find module root")
		}
		directory = parent
	}
}

func assertNoPrometheusModuleDependency(t *testing.T, goModPath string) {
	t.Helper()
	contents, err := os.ReadFile(goModPath)
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	for _, line := range strings.Split(string(contents), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && isPrometheusPath(fields[0]) {
			t.Errorf("go.mod contains forbidden Prometheus dependency %q", fields[0])
		}
	}
}

func assertNoPrometheusImportsOrRegistration(t *testing.T, internalDirectory string) {
	t.Helper()
	fset := token.NewFileSet()
	err := filepath.WalkDir(internalDirectory, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		prometheusAliases := make(map[string]struct{})
		for _, imported := range file.Imports {
			importPath := strings.Trim(imported.Path.Value, "\"")
			if !isPrometheusPath(importPath) {
				continue
			}
			t.Errorf("%s imports forbidden Prometheus package %q", path, importPath)
			alias := filepath.Base(importPath)
			if imported.Name != nil {
				alias = imported.Name.Name
			}
			prometheusAliases[alias] = struct{}{}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || !isPrometheusRegistration(selector.Sel.Name) {
				return true
			}
			identifier, ok := selector.X.(*ast.Ident)
			if ok {
				if _, exists := prometheusAliases[identifier.Name]; exists {
					t.Errorf("%s registers forbidden Prometheus surface through %s", path, identifier.Name)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("scan internal Go files: %v", err)
	}
}

func isPrometheusPath(path string) bool {
	return strings.Contains(strings.ToLower(path), "pro"+"metheus")
}

func isPrometheusRegistration(name string) bool {
	switch name {
	case "Register", "MustRegister", "NewRegistry", "Handler", "HandlerFor":
		return true
	default:
		return false
	}
}
