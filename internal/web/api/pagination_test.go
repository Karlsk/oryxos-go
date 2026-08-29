package api

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/Karlsk/oryxos-go/internal/observability"
	"github.com/gin-gonic/gin"
)

func TestPageResultEmptyItemsArray(t *testing.T) {
	response := performRequest(t, "req_page-1", func(c *gin.Context) {
		Page(c, http.StatusOK, "ok", "ok", PageResult[string]{Page: 1, PageSize: 20, Total: 0})
	})
	body := decodeObject(t, response)
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("data = %#v, want page object", body["data"])
	}
	items, ok := data["items"].([]any)
	if !ok || len(items) != 0 {
		t.Fatalf("items = %#v, want empty array", data["items"])
	}
	if data["page"] != float64(1) || data["page_size"] != float64(20) || data["total"] != float64(0) || data["total_pages"] != float64(0) {
		t.Fatalf("page data = %#v, want all canonical fields", data)
	}
}

func TestPageResultDefaultsAndBounds(t *testing.T) {
	page, pageSize, details := parsePagination(url.Values{})
	if page != 1 || pageSize != 20 || details != nil {
		t.Fatalf("defaults = (%d, %d, %#v), want (1, 20, nil)", page, pageSize, details)
	}

	tests := []struct {
		name  string
		query string
	}{
		{"empty", "page="},
		{"duplicate", "page=1&page=2"},
		{"signed", "page=+1"},
		{"decimal", "page=1.5"},
		{"zero", "page=0"},
		{"negative", "page=-1"},
		{"overflow", "page=999999999999999999999999"},
		{"page over limit", "page=10001"},
		{"page size over limit", "page_size=101"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := paginationResponse(t, test.query)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
			}
			body := decodeObject(t, response)
			if body["code"] != "invalid_argument" || body["message"] != "invalid argument" {
				t.Fatalf("response = %#v, want invalid argument", body)
			}
			details, ok := body["details"].(map[string]any)
			if !ok || len(details) != 2 {
				t.Fatalf("details = %#v, want safe field/rule", body["details"])
			}
		})
	}
}

func TestPageResultMaxInt64TotalPages(t *testing.T) {
	response := performRequest(t, "req_max-1", func(c *gin.Context) {
		Page(c, http.StatusOK, "ok", "ok", PageResult[string]{
			Page:     1,
			PageSize: 100,
			Total:    math.MaxInt64,
		})
	})
	var envelope Result[PageResult[string]]
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode page envelope: %v", err)
	}
	want := math.MaxInt64 / int64(100)
	if math.MaxInt64%int64(100) != 0 {
		want++
	}
	if got := envelope.Data.TotalPages; got != want {
		t.Fatalf("total_pages = %d, want %d", got, want)
	}
}

func TestPageResultInvalidFallback(t *testing.T) {
	tests := []struct {
		name string
		page PageResult[string]
	}{
		{"page low", PageResult[string]{Page: 0, PageSize: 1}},
		{"page high", PageResult[string]{Page: 10001, PageSize: 1}},
		{"size low", PageResult[string]{Page: 1, PageSize: 0}},
		{"size high", PageResult[string]{Page: 1, PageSize: 101}},
		{"negative total", PageResult[string]{Page: 1, PageSize: 1, Total: -1}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := performRequest(t, "req_bad-page", func(c *gin.Context) {
				Page(c, http.StatusOK, "ok", "ok", test.page)
			})
			assertInternalFallback(t, response)
		})
	}
}

func paginationResponse(t *testing.T, rawQuery string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/?"+rawQuery, nil)
	request = request.WithContext(observability.WithCorrelation(request.Context(), observability.Correlation{RequestID: "req_pagination-1"}))
	router := gin.New()
	router.GET("/", func(c *gin.Context) {
		_, _, details := parsePagination(c.Request.URL.Query())
		if details != nil {
			Error(c, http.StatusBadRequest, "invalid_argument", "invalid argument", details)
			return
		}
		Success(c, http.StatusOK, "ok", "ok", "parsed")
	})
	router.ServeHTTP(recorder, request)
	return recorder
}
