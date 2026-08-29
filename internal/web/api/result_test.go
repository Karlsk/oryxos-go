package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Karlsk/oryxos-go/internal/observability"
	"github.com/gin-gonic/gin"
)

func TestResultSuccessEnvelope(t *testing.T) {
	response := performRequest(t, "req_success-1", func(c *gin.Context) {
		Success(c, http.StatusOK, "ok", "ok", map[string]string{"status": "ready"})
	})

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	body := decodeObject(t, response)
	if body["code"] != "ok" || body["message"] != "ok" {
		t.Fatalf("descriptor = %#v, want ok/ok", body)
	}
	data, ok := body["data"].(map[string]any)
	if !ok || data["status"] != "ready" {
		t.Fatalf("data = %#v, want status ready", body["data"])
	}
	assertMatchingRequestID(t, response, body, "req_success-1")
}

func TestResultErrorEnvelope(t *testing.T) {
	response := performRequest(t, "req_error-1", func(c *gin.Context) {
		Error(c, http.StatusBadRequest, "invalid_argument", "invalid argument", map[string]any{
			"field": "page",
			"rule":  "out_of_range",
		})
	})

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	body := decodeObject(t, response)
	if body["code"] != "invalid_argument" || body["message"] != "invalid argument" {
		t.Fatalf("descriptor = %#v, want invalid argument", body)
	}
	if _, ok := body["data"]; ok {
		t.Fatalf("error data = %#v, want omitted", body["data"])
	}
	details, ok := body["details"].(map[string]any)
	if !ok || len(details) != 2 || details["field"] != "page" || details["rule"] != "out_of_range" {
		t.Fatalf("details = %#v, want exact safe details", body["details"])
	}
	assertMatchingRequestID(t, response, body, "req_error-1")
}

func TestResultInvalidDescriptorFallback(t *testing.T) {
	tests := []struct {
		name  string
		write func(*gin.Context)
	}{
		{"success", func(c *gin.Context) { Success(c, http.StatusAccepted, "accepted", "accepted", "ignored") }},
		{"page", func(c *gin.Context) {
			Page(c, http.StatusCreated, "created", "created", PageResult[string]{Page: 1, PageSize: 1})
		}},
		{"error", func(c *gin.Context) { Error(c, http.StatusTeapot, "teapot", "teapot", nil) }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := performRequest(t, "req_fallback-1", func(c *gin.Context) {
				test.write(c)
				Error(c, http.StatusBadRequest, "invalid_argument", "invalid argument", map[string]any{"field": "page", "rule": "required"})
			})
			assertInternalFallback(t, response)
		})
	}
}

func TestResultInvalidNestedDetailsFallback(t *testing.T) {
	tests := []struct {
		name    string
		details map[string]any
	}{
		{"nested", map[string]any{"field": "page", "rule": map[string]any{"secret": "top-secret"}}},
		{"extra", map[string]any{"field": "page", "rule": "required", "secret": "top-secret"}},
		{"missing", map[string]any{"field": "page"}},
		{"field", map[string]any{"field": "Page", "rule": "required"}},
		{"rule", map[string]any{"field": "page", "rule": "unknown"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := performRequest(t, "req_details-1", func(c *gin.Context) {
				Error(c, http.StatusBadRequest, "invalid_argument", "invalid argument", test.details)
			})
			assertInternalFallback(t, response)
			if strings.Contains(response.Body.String(), "top-secret") {
				t.Fatal("unsafe detail serialized")
			}
		})
	}
}

func TestResultEmergencyRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	router := gin.New()
	var installedID string
	router.GET("/", func(c *gin.Context) {
		Success(c, http.StatusOK, "ok", "ok", "first")
		installedID = observability.CorrelationFromContext(c.Request.Context()).RequestID
		Success(c, http.StatusOK, "ok", "ok", "second")
	})
	router.ServeHTTP(recorder, request)

	body := decodeObject(t, recorder)
	if !regexp.MustCompile(`^req_[0-9a-f]{32}$`).MatchString(installedID) {
		t.Fatalf("installed request ID = %q, want req_ plus 32 lowercase hex characters", installedID)
	}
	assertMatchingRequestID(t, recorder, body, installedID)
	data, ok := body["data"].(string)
	if !ok || data != "first" {
		t.Fatalf("data = %#v, want first response only", body["data"])
	}
}

func TestEmergencyRequestIDFromReader(t *testing.T) {
	got := emergencyRequestIDFrom(bytes.NewReader([]byte{
		0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
		0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
	}))

	const want = "req_000102030405060708090a0b0c0d0e0f"
	if got != want {
		t.Fatalf("emergencyRequestIDFrom() = %q, want %q", got, want)
	}
}

func TestEmergencyRequestIDFromReaderPanicsOnEntropyFailure(t *testing.T) {
	defer func() {
		t.Helper()
		if got := recover(); got != "unable to generate emergency request ID" {
			t.Fatalf("panic = %#v, want stable non-secret message", got)
		}
	}()

	emergencyRequestIDFrom(failingReader{err: errors.New("entropy source unavailable")})
	t.Fatal("emergencyRequestIDFrom() returned after entropy failure")
}

type failingReader struct {
	err error
}

func (r failingReader) Read([]byte) (int, error) {
	return 0, r.err
}

func TestResultTimeUTC(t *testing.T) {
	value := time.Date(2026, time.August, 25, 9, 10, 11, 0, time.FixedZone("UTC+8", 8*60*60))
	got := FormatTimeUTC(value)
	if got != "2026-08-25T01:10:11Z" {
		t.Fatalf("FormatTimeUTC() = %q, want UTC RFC3339", got)
	}
}

func performRequest(t *testing.T, requestID string, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	request = request.WithContext(observability.WithCorrelation(request.Context(), observability.Correlation{RequestID: requestID}))
	router := gin.New()
	router.GET("/", handler)
	router.ServeHTTP(recorder, request)
	return recorder
}

func decodeObject(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode JSON: %v; body = %q", err, recorder.Body.String())
	}
	return body
}

func assertMatchingRequestID(t *testing.T, recorder *httptest.ResponseRecorder, body map[string]any, want string) {
	t.Helper()
	if got := recorder.Header().Get("X-Request-ID"); got != want {
		t.Fatalf("X-Request-ID = %q, want %q", got, want)
	}
	if got, _ := body["request_id"].(string); got != want {
		t.Fatalf("body request_id = %q, want %q", got, want)
	}
}

func assertInternalFallback(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	body := decodeObject(t, recorder)
	if body["code"] != "internal" || body["message"] != "internal server error" {
		t.Fatalf("fallback = %#v, want internal envelope", body)
	}
	if _, ok := body["data"]; ok {
		t.Fatalf("fallback data = %#v, want omitted", body["data"])
	}
	if _, ok := body["details"]; ok {
		t.Fatalf("fallback details = %#v, want omitted", body["details"])
	}
}
