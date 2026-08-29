package web

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Karlsk/oryxos-go/internal/config"
	"github.com/Karlsk/oryxos-go/internal/observability"
	"github.com/Karlsk/oryxos-go/internal/web/api"
	"github.com/gin-gonic/gin"
)

func TestNewServerDoesNotEmitGinDiagnostics(t *testing.T) {
	previousMode := gin.Mode()
	previousWriter := gin.DefaultWriter
	previousErrorWriter := gin.DefaultErrorWriter
	t.Cleanup(func() {
		gin.SetMode(previousMode)
		gin.DefaultWriter = previousWriter
		gin.DefaultErrorWriter = previousErrorWriter
	})

	var diagnostics strings.Builder
	gin.DefaultWriter = &diagnostics
	gin.DefaultErrorWriter = &diagnostics
	gin.SetMode(gin.DebugMode)

	NewServer(testServerConfig(), observability.NewObserver(), testLogger(io.Discard), "test-version")

	if output := diagnostics.String(); output != "" {
		t.Fatalf("NewServer emitted Gin diagnostics: %q", output)
	}
}

func TestServerTimeouts(t *testing.T) {
	t.Parallel()

	cfg := testServerConfig()
	server := NewServer(cfg, observability.NewObserver(), testLogger(io.Discard), "test-version")

	if server.httpServer.ReadHeaderTimeout != cfg.ReadHeaderTimeout || server.httpServer.ReadHeaderTimeout == 0 {
		t.Fatalf("ReadHeaderTimeout = %s, want %s and nonzero", server.httpServer.ReadHeaderTimeout, cfg.ReadHeaderTimeout)
	}
	if server.httpServer.ReadTimeout != cfg.ReadTimeout || server.httpServer.ReadTimeout == 0 {
		t.Fatalf("ReadTimeout = %s, want %s and nonzero", server.httpServer.ReadTimeout, cfg.ReadTimeout)
	}
	if server.httpServer.WriteTimeout != cfg.WriteTimeout || server.httpServer.WriteTimeout == 0 {
		t.Fatalf("WriteTimeout = %s, want %s and nonzero", server.httpServer.WriteTimeout, cfg.WriteTimeout)
	}
	if server.httpServer.IdleTimeout != cfg.IdleTimeout || server.httpServer.IdleTimeout == 0 {
		t.Fatalf("IdleTimeout = %s, want %s and nonzero", server.httpServer.IdleTimeout, cfg.IdleTimeout)
	}
}

//nolint:noctx // httptest requests exercise handler-local context installation.
func TestHealthReadyEnvelope(t *testing.T) {
	t.Parallel()

	observer := observability.NewObserver()
	observer.SetReady(true)
	server := NewServer(testServerConfig(), observer, testLogger(io.Discard), "test-version")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)

	server.Handler().ServeHTTP(recorder, request)

	result := decodeResult(t, recorder)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if result.Code != "ok" {
		t.Fatalf("code = %q, want ok", result.Code)
	}
	var data struct {
		Status string `json:"status"`
	}
	decodeData(t, result, &data)
	if data.Status != "ready" {
		t.Fatalf("data.status = %q, want ready", data.Status)
	}
	assertRequestID(t, recorder, result.RequestID)
}

//nolint:noctx // httptest requests exercise handler-local context installation.
func TestHealthNotReadyEnvelope(t *testing.T) {
	t.Parallel()

	server := NewServer(testServerConfig(), observability.NewObserver(), testLogger(io.Discard), "test-version")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)

	server.Handler().ServeHTTP(recorder, request)

	result := decodeResult(t, recorder)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
	if result.Code != "not_ready" {
		t.Fatalf("code = %q, want not_ready", result.Code)
	}
	if result.Data != nil {
		t.Fatalf("data = %s, want omitted", string(*result.Data))
	}
	assertRequestID(t, recorder, result.RequestID)
}

//nolint:noctx // httptest requests exercise handler-local context installation.
func TestInfoEnvelope(t *testing.T) {
	t.Parallel()

	observer := observability.NewObserver()
	observer.SetReady(true)
	var logs bytes.Buffer
	server := NewServer(testServerConfig(), observer, testLogger(&logs), "v0.0.1")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/info", nil)
	request.Header.Set("X-Request-ID", "req_info-1")

	server.Handler().ServeHTTP(recorder, request)

	result := decodeResult(t, recorder)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if result.Code != "ok" {
		t.Fatalf("code = %q, want ok", result.Code)
	}
	var data map[string]any
	decodeData(t, result, &data)
	if len(data) != 3 || data["version"] != "v0.0.1" || data["mode"] != "foundation" || data["ready"] != true {
		t.Fatalf("info data = %#v, want only version, mode foundation, and ready true", data)
	}
	assertRequestID(t, recorder, result.RequestID)
	if result.RequestID != "req_info-1" {
		t.Fatalf("request ID = %q, want req_info-1", result.RequestID)
	}
	assertAccessLogRequestID(t, logs.Bytes(), result.RequestID)
}

//nolint:noctx // httptest requests exercise handler-local context installation.
func TestUnknownRouteEnvelope(t *testing.T) {
	t.Parallel()

	server := NewServer(testServerConfig(), observability.NewObserver(), testLogger(io.Discard), "test-version")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/not-registered", nil)

	server.Handler().ServeHTTP(recorder, request)

	result := decodeResult(t, recorder)
	if recorder.Code != http.StatusNotFound || result.Code != "not_found" {
		t.Fatalf("status/code = %d/%q, want 404/not_found", recorder.Code, result.Code)
	}
	assertRequestID(t, recorder, result.RequestID)
}

//nolint:noctx // httptest requests exercise handler-local context installation.
func TestMethodNotAllowedEnvelope(t *testing.T) {
	t.Parallel()

	server := NewServer(testServerConfig(), observability.NewObserver(), testLogger(io.Discard), "test-version")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/health", nil)

	server.Handler().ServeHTTP(recorder, request)

	result := decodeResult(t, recorder)
	if recorder.Code != http.StatusMethodNotAllowed || result.Code != "method_not_allowed" {
		t.Fatalf("status/code = %d/%q, want 405/method_not_allowed", recorder.Code, result.Code)
	}
	assertRequestID(t, recorder, result.RequestID)
}

func TestRouteInventory(t *testing.T) {
	t.Parallel()

	server := NewServer(testServerConfig(), observability.NewObserver(), testLogger(io.Discard), "test-version")
	routes := server.Routes()
	actual := make([]string, 0, len(routes))
	for _, route := range routes {
		actual = append(actual, route.Method+" "+route.Path)
	}
	sort.Strings(actual)
	want := []string{"GET /api/v1/health", "GET /api/v1/info"}
	if len(actual) != len(want) {
		t.Fatalf("route count = %d, want %d: %v", len(actual), len(want), actual)
	}
	for index := range want {
		if actual[index] != want[index] {
			t.Fatalf("routes = %v, want %v", actual, want)
		}
	}
}

//nolint:noctx // httptest requests exercise handler-local context installation.
func TestForbiddenFoundationRoutesAbsent(t *testing.T) {
	t.Parallel()

	server := NewServer(testServerConfig(), observability.NewObserver(), testLogger(io.Discard), "test-version")
	paths := []string{
		"/metrics",
		"/api/v1/openapi.json",
		"/openapi.json",
		"/swagger",
		"/swagger/index.html",
		"/api/v1/events",
		"/api/v1/stream",
		"/api/v1/ws",
	}
	for _, path := range paths {
		path := path
		t.Run(path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, path, nil)
			if path == "/api/v1/ws" {
				request.Header.Set("Connection", "Upgrade")
				request.Header.Set("Upgrade", "websocket")
			}

			server.Handler().ServeHTTP(recorder, request)

			result := decodeResult(t, recorder)
			if recorder.Code != http.StatusNotFound || result.Code != "not_found" {
				t.Fatalf("%s status/code = %d/%q, want 404/not_found", path, recorder.Code, result.Code)
			}
		})
	}
}

func testServerConfig() config.ServerConfig {
	return config.ServerConfig{
		ListenAddress:     "127.0.0.1:0",
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      5 * time.Minute,
		IdleTimeout:       time.Minute,
		ShutdownTimeout:   30 * time.Second,
	}
}

func testLogger(writer io.Writer) *slog.Logger {
	return observability.NewLogger(writer, slog.LevelDebug)
}

func decodeResult(t *testing.T, recorder *httptest.ResponseRecorder) api.Result[json.RawMessage] {
	t.Helper()
	var result api.Result[json.RawMessage]
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode result: %v; body = %q", err, recorder.Body.String())
	}
	return result
}

func decodeData(t *testing.T, result api.Result[json.RawMessage], target any) {
	t.Helper()
	if result.Data == nil {
		t.Fatal("data is omitted")
	}
	if err := json.Unmarshal(*result.Data, target); err != nil {
		t.Fatalf("decode data: %v", err)
	}
}

func assertRequestID(t *testing.T, recorder *httptest.ResponseRecorder, requestID string) {
	t.Helper()
	if requestID == "" {
		t.Fatal("response request_id is empty")
	}
	if header := recorder.Header().Get("X-Request-ID"); header != requestID {
		t.Fatalf("X-Request-ID = %q, want %q", header, requestID)
	}
}

func assertAccessLogRequestID(t *testing.T, rawLogs []byte, requestID string) {
	t.Helper()
	var record map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(rawLogs), &record); err != nil {
		t.Fatalf("decode access log: %v; logs = %q", err, string(rawLogs))
	}
	if record["msg"] != "http.request_complete" || record["request_id"] != requestID {
		t.Fatalf("access log = %#v, want http.request_complete with request_id %q", record, requestID)
	}
}
