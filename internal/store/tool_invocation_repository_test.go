package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpenSQLiteAppliesToolInvocationsMigration(t *testing.T) {
	database, err := OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "oryxos.db"))
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	sqlDatabase, _ := database.DB()
	t.Cleanup(func() { _ = sqlDatabase.Close() })

	type column struct{ Name string }
	var columns []column
	if err := database.Raw("PRAGMA table_info(tool_invocations)").Scan(&columns).Error; err != nil {
		t.Fatalf("table_info: %v", err)
	}
	want := []string{"id", "session_id", "tool_name", "input_json", "result_json", "success", "error_message", "duration_ms", "created_at"}
	if len(columns) != len(want) {
		t.Fatalf("columns = %#v, want %v", columns, want)
	}
	for index := range want {
		if columns[index].Name != want[index] {
			t.Fatalf("column %d = %q, want %q", index, columns[index].Name, want[index])
		}
	}
	var indexCount int64
	if err := database.Raw("SELECT count(*) FROM sqlite_master WHERE type='index' AND name='idx_tool_invocations_session_id'").Scan(&indexCount).Error; err != nil || indexCount != 1 {
		t.Fatalf("index count = %d, %v; want 1", indexCount, err)
	}
	var llmTableCount int64
	if err := database.Raw("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='llm_calls'").Scan(&llmTableCount).Error; err != nil || llmTableCount != 1 {
		t.Fatalf("llm_calls count = %d, %v; want 1", llmTableCount, err)
	}
}

func TestToolInvocationRepositoryRoundTripsFinalOutcomes(t *testing.T) {
	database, err := OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "oryxos.db"))
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	sqlDatabase, _ := database.DB()
	t.Cleanup(func() { _ = sqlDatabase.Close() })
	repository, err := NewToolInvocationRepository(database)
	if err != nil {
		t.Fatalf("NewToolInvocationRepository() error = %v", err)
	}
	now := time.Date(2026, 9, 18, 1, 2, 3, 0, time.UTC)
	success := &ToolInvocation{SessionID: "s1", ToolName: "read_file", InputJSON: `{}`, ResultJSON: `"ok"`, Success: true, DurationMS: 2, CreatedAt: now}
	if err := repository.Create(context.Background(), success); err != nil {
		t.Fatalf("Create(success) error = %v", err)
	}
	failureText := "api_key=already-redacted"
	failure := &ToolInvocation{SessionID: "s2", ToolName: "http_get", InputJSON: `{}`, ResultJSON: `""`, Success: false, ErrorMessage: &failureText, DurationMS: 4, CreatedAt: now}
	if err := repository.Create(context.Background(), failure); err != nil {
		t.Fatalf("Create(failure) error = %v", err)
	}
	var got []ToolInvocation
	if err := database.Order("id").Find(&got).Error; err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if len(got) != 2 || got[0].ID == 0 || got[0].ErrorMessage != nil || got[1].ErrorMessage == nil || *got[1].ErrorMessage != "[REDACTED]" {
		t.Fatalf("rows = %#v, want success and failure", got)
	}
}

func TestToolInvocationRepositoryValidatesAndSanitizesFailure(t *testing.T) {
	database, err := OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "oryxos.db"))
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	sqlDatabase, _ := database.DB()
	t.Cleanup(func() { _ = sqlDatabase.Close() })
	repository, _ := NewToolInvocationRepository(database)
	now := time.Now().UTC()
	secretFailure := "api_key=very-secret-value"
	record := &ToolInvocation{SessionID: "s", ToolName: "http_get", InputJSON: `{}`, ResultJSON: `""`, Success: false, ErrorMessage: &secretFailure, CreatedAt: now}
	if err := repository.Create(context.Background(), record); err != nil {
		t.Fatalf("Create(failure) error = %v", err)
	}
	if record.ErrorMessage == nil || strings.Contains(*record.ErrorMessage, "very-secret-value") || !strings.Contains(*record.ErrorMessage, "[REDACTED]") {
		t.Fatalf("ErrorMessage = %v, want sanitized text", record.ErrorMessage)
	}

	cases := []*ToolInvocation{
		nil,
		{ToolName: "tool", InputJSON: `{}`, ResultJSON: `{}`, Success: true, CreatedAt: now},
		{SessionID: "s", InputJSON: `{}`, ResultJSON: `{}`, Success: true, CreatedAt: now},
		{SessionID: "s", ToolName: "tool", InputJSON: `{}`, ResultJSON: `{}`, Success: false, CreatedAt: now},
		{SessionID: "s", ToolName: "tool", InputJSON: `{}`, ResultJSON: `{}`, Success: true, ErrorMessage: &secretFailure, CreatedAt: now},
		{SessionID: "s", ToolName: "tool", InputJSON: `{}`, ResultJSON: `{}`, Success: true, DurationMS: -1, CreatedAt: now},
		{SessionID: "s", ToolName: "tool", InputJSON: `{}`, ResultJSON: `{}`, Success: true},
	}
	for index, candidate := range cases {
		if err := repository.Create(context.Background(), candidate); err == nil {
			t.Errorf("Create(case %d) error = nil, want validation error", index)
		}
	}
}
