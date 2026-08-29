package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/Karlsk/oryxos-go/internal/observability"
	"github.com/Karlsk/oryxos-go/internal/web/api"
	"github.com/gin-gonic/gin"
)

const oneMiB = 1 << 20

func TestRequestIDReuseAndGeneration(t *testing.T) {
	validID := regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)
	for _, test := range []struct {
		name     string
		incoming string
		wantID   string
	}{
		{name: "reuses valid", incoming: "req_ABC-1", wantID: "req_ABC-1"},
		{name: "generates missing"},
		{name: "replaces malformed", incoming: "bad id!"},
	} {
		t.Run(test.name, func(t *testing.T) {
			observer, logs, router := boundaryRouter(t, func(c *gin.Context) {
				api.Success(c, http.StatusOK, "ok", "ok", "ready")
			})
			request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/request-id", nil)
			if test.incoming != "" {
				request.Header.Set("X-Request-ID", test.incoming)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			body := responseObject(t, response)
			id := response.Header().Get("X-Request-ID")
			if test.wantID != "" && id != test.wantID {
				t.Fatalf("response request ID = %q, want %q", id, test.wantID)
			}
			if test.wantID == "" && !validID.MatchString(id) {
				t.Fatalf("generated request ID = %q, want 1..128 safe ASCII bytes", id)
			}
			if got, _ := body["request_id"].(string); got != id {
				t.Fatalf("body request ID = %q, want %q", got, id)
			}
			if got := logRecord(t, logs)["request_id"]; got != id {
				t.Fatalf("access log request ID = %v, want %q", got, id)
			}
			if got := observer.Snapshot().HTTPRequests[0].Route; got != "/:operation" {
				t.Fatalf("observed route = %q, want matched route template", got)
			}
		})
	}
}

func TestMiddlewareOrderAndFinalObservation(t *testing.T) {
	observer, logs, router := boundaryRouter(t, func(*gin.Context) { panic("panic fixture") })
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/panic", nil))

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	snapshot := observer.Snapshot()
	if len(snapshot.HTTPRequests) != 1 {
		t.Fatalf("observations = %d, want 1", len(snapshot.HTTPRequests))
	}
	if got := snapshot.HTTPRequests[0]; got.Route != "/:operation" || got.Status != http.StatusInternalServerError {
		t.Fatalf("final observation = %#v, want matched template with final 500", got)
	}
	if got := logRecord(t, logs)["status"]; got != float64(http.StatusInternalServerError) {
		t.Fatalf("access log status = %v, want 500", got)
	}
}

func TestRequestBodyLimit(t *testing.T) {
	_, _, router := boundaryRouter(t, func(c *gin.Context) {
		_, err := io.ReadAll(c.Request.Body)
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			api.Error(c, http.StatusRequestEntityTooLarge, "payload_too_large", "payload too large", nil)
			return
		}
		if err != nil {
			api.Error(c, http.StatusInternalServerError, "internal", "internal server error", nil)
			return
		}
		api.Success(c, http.StatusOK, "ok", "ok", "read")
	})
	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/decode", strings.NewReader(strings.Repeat("x", oneMiB+1)))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusRequestEntityTooLarge)
	}
	body := responseObject(t, response)
	if body["code"] != "payload_too_large" || body["message"] != "payload too large" {
		t.Fatalf("body = %#v, want payload_too_large Result envelope", body)
	}
	if strings.Contains(response.Body.String(), "http: request body too large") {
		t.Fatal("response exposed standard-library plain-text body-limit error")
	}
}

func TestRecoveryUsesSafeEnvelope(t *testing.T) {
	t.Run("preserves committed response", func(t *testing.T) {
		observer, logs, router := boundaryRouter(t, func(c *gin.Context) {
			c.Writer.WriteHeader(http.StatusAccepted)
			_, _ = c.Writer.Write([]byte("committed body"))
			panic("committed secret")
		})
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/committed-panic", nil))

		if response.Code != http.StatusAccepted || response.Body.String() != "committed body" {
			t.Fatalf("response = %d %q, want one committed 202 response", response.Code, response.Body.String())
		}
		if strings.Contains(response.Body.String(), "committed secret") || strings.Contains(logs.String(), "committed secret") {
			t.Fatal("panic text escaped the committed response or logs")
		}
		if got := countLogMessages(t, logs, "http.recovered_panic"); got != 1 {
			t.Fatalf("recovered panic records = %d, want 1", got)
		}
		if got := observer.Snapshot().HTTPRequests[0].Status; got != http.StatusAccepted {
			t.Fatalf("observed status = %d, want committed 202", got)
		}
		if got := logRecord(t, logs)["status"]; got != float64(http.StatusAccepted) {
			t.Fatalf("access log status = %v, want 202", got)
		}
	})

	observer, logs, router := boundaryRouter(t, func(*gin.Context) { panic("top-secret panic") })
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/safe-panic", nil))

	body := responseObject(t, response)
	if response.Code != http.StatusInternalServerError || body["code"] != "internal" || body["message"] != "internal server error" {
		t.Fatalf("response = %d %#v, want safe 500/internal envelope", response.Code, body)
	}
	for _, forbidden := range []string{"top-secret panic", "goroutine ", "stack"} {
		if strings.Contains(response.Body.String(), forbidden) || strings.Contains(logs.String(), forbidden) {
			t.Fatalf("unsafe panic detail %q escaped response or log", forbidden)
		}
	}
	if got := observer.Snapshot().HTTPRequests[0].Status; got != http.StatusInternalServerError {
		t.Fatalf("observed status = %d, want 500", got)
	}
	if got := logRecord(t, logs)["status"]; got != float64(http.StatusInternalServerError) {
		t.Fatalf("logged status = %v, want 500", got)
	}
}

func TestAccessObservationPostHandlerContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	observer := observability.NewObserver()
	var logs bytes.Buffer
	router := gin.New()
	router.Use(AccessObservation(observer, observability.NewLogger(&logs, slog.LevelInfo)))
	router.GET("/emergency", func(c *gin.Context) {
		api.Success(c, http.StatusOK, "ok", "ok", "ready")
	})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/emergency", nil))
	body := responseObject(t, response)
	id := response.Header().Get("X-Request-ID")
	if got, _ := body["request_id"].(string); got != id || id == "" {
		t.Fatalf("response IDs header/body = %q/%q, want one emergency ID", id, got)
	}
	if got := logRecord(t, &logs)["request_id"]; got != id {
		t.Fatalf("post-handler access log ID = %v, want %q", got, id)
	}
	if got := observer.Snapshot().HTTPRequests[0]; got.Route != "/emergency" || got.Status != http.StatusOK {
		t.Fatalf("post-handler observation = %#v, want final /emergency 200", got)
	}
}

func boundaryRouter(t *testing.T, handler gin.HandlerFunc) (observability.Observer, *bytes.Buffer, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	observer := observability.NewObserver()
	var logs bytes.Buffer
	router := gin.New()
	router.Use(
		RequestID(),
		RequestBodyLimit(oneMiB),
		AccessObservation(observer, observability.NewLogger(&logs, slog.LevelInfo)),
		Recovery(observability.NewLogger(&logs, slog.LevelInfo)),
	)
	router.Any("/:operation", handler)
	return observer, &logs, router
}

func responseObject(t *testing.T, response *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return body
}

func logRecord(t *testing.T, logs *bytes.Buffer) map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(logs.String()), "\n")
	if len(lines) == 0 || lines[0] == "" {
		t.Fatal("no access log record")
	}
	var record map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &record); err != nil {
		t.Fatalf("decode access log: %v", err)
	}
	return record
}

func countLogMessages(t *testing.T, logs *bytes.Buffer, want string) int {
	t.Helper()
	count := 0
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("decode log: %v", err)
		}
		if record["msg"] == want {
			count++
		}
	}
	return count
}
