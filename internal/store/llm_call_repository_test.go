package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpenSQLiteAppliesLlmCallsMigrationAndPragmas(t *testing.T) {
	database, err := OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "oryxos.db"))
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	sqlDatabase, err := database.DB()
	if err != nil {
		t.Fatalf("DB() error = %v", err)
	}
	t.Cleanup(func() { _ = sqlDatabase.Close() })

	type column struct {
		Name    string
		NotNull int `gorm:"column:notnull"`
	}
	var columns []column
	if err := database.Raw("PRAGMA table_info(llm_calls)").Scan(&columns).Error; err != nil {
		t.Fatalf("table_info: %v", err)
	}
	wantColumns := []string{"id", "session_id", "provider", "model", "prompt_tokens", "completion_tokens", "total_tokens", "success", "error_message", "duration_ms", "created_at"}
	if len(columns) != len(wantColumns) {
		t.Fatalf("columns = %#v, want %v", columns, wantColumns)
	}
	for index, want := range wantColumns {
		if columns[index].Name != want {
			t.Fatalf("column %d = %q, want %q", index, columns[index].Name, want)
		}
	}

	var journalMode string
	if err := database.Raw("PRAGMA journal_mode").Scan(&journalMode).Error; err != nil || strings.ToLower(journalMode) != "wal" {
		t.Fatalf("journal_mode = %q, %v; want wal", journalMode, err)
	}
	var busyTimeout int
	if err := database.Raw("PRAGMA busy_timeout").Scan(&busyTimeout).Error; err != nil || busyTimeout < 1000 {
		t.Fatalf("busy_timeout = %d, %v; want >= 1000", busyTimeout, err)
	}

	var indexCount int64
	if err := database.Raw("SELECT count(*) FROM sqlite_master WHERE type='index' AND name='idx_llm_calls_session_id'").Scan(&indexCount).Error; err != nil || indexCount != 1 {
		t.Fatalf("index count = %d, %v; want 1", indexCount, err)
	}
}

func TestLlmCallRepositoryRoundTripsSuccessAndFailure(t *testing.T) {
	database, err := OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "oryxos.db"))
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	sqlDatabase, _ := database.DB()
	t.Cleanup(func() { _ = sqlDatabase.Close() })
	repository, err := NewLlmCallRepository(database)
	if err != nil {
		t.Fatalf("NewLlmCallRepository() error = %v", err)
	}

	createdAt := time.Date(2026, 9, 15, 1, 2, 3, 0, time.UTC)
	success := &LlmCall{SessionID: "session-success", Provider: "deepseek", Model: "deepseek-chat", PromptTokens: 2, CompletionTokens: 3, TotalTokens: 5, Success: true, DurationMS: 7, CreatedAt: createdAt}
	if err := repository.Create(context.Background(), success); err != nil {
		t.Fatalf("Create(success) error = %v", err)
	}
	failureMessage := "[REDACTED]"
	failure := &LlmCall{SessionID: "session-failure", Provider: "minimax", Model: "MiniMax-M2.7", Success: false, ErrorMessage: &failureMessage, DurationMS: 9, CreatedAt: createdAt}
	if err := repository.Create(context.Background(), failure); err != nil {
		t.Fatalf("Create(failure) error = %v", err)
	}

	var got []LlmCall
	if err := database.Order("id").Find(&got).Error; err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if len(got) != 2 || got[0].ID == 0 || got[0].ErrorMessage != nil || got[1].ErrorMessage == nil || *got[1].ErrorMessage != failureMessage {
		t.Fatalf("rows = %#v, want success and nullable-error failure", got)
	}
}

func TestLlmCallRepositoryRejectsInvalidRecords(t *testing.T) {
	database, err := OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "oryxos.db"))
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	sqlDatabase, _ := database.DB()
	t.Cleanup(func() { _ = sqlDatabase.Close() })
	repository, _ := NewLlmCallRepository(database)
	now := time.Now().UTC()
	cases := []LlmCall{
		{Provider: "deepseek", Model: "m", Success: true, CreatedAt: now},
		{SessionID: "s", Model: "m", Success: true, CreatedAt: now},
		{SessionID: "s", Provider: "deepseek", Success: true, CreatedAt: now},
		{SessionID: "s", Provider: "deepseek", Model: "m", PromptTokens: -1, Success: true, CreatedAt: now},
		{SessionID: "s", Provider: "deepseek", Model: "m", Success: true, DurationMS: -1, CreatedAt: now},
		{SessionID: "s", Provider: "deepseek", Model: "m", Success: true},
	}
	for index := range cases {
		if err := repository.Create(context.Background(), &cases[index]); err == nil {
			t.Errorf("Create(case %d) error = nil, want validation error", index)
		}
	}
}
