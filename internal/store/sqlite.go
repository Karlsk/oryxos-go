package store

import (
	"context"
	_ "embed"
	"fmt"
	"net/url"
	"path/filepath"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

//go:embed migrations/001_llm_calls.sql
var llmCallsMigration string

//go:embed migrations/002_tool_invocations.sql
var toolInvocationsMigration string

// OpenSQLite opens the pure-Go SQLite database and applies repository-owned migrations.
func OpenSQLite(ctx context.Context, path string) (*gorm.DB, error) {
	if ctx == nil {
		return nil, fmt.Errorf("open sqlite: context is nil")
	}
	if path == "" {
		return nil, fmt.Errorf("open sqlite: path is empty")
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve sqlite path: %w", err)
	}
	dsn := (&url.URL{
		Scheme:   "file",
		Path:     filepath.ToSlash(absolutePath),
		RawQuery: "_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)",
	}).String()
	database, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := database.WithContext(ctx).Exec(llmCallsMigration).Error; err != nil {
		sqlDatabase, dbErr := database.DB()
		if dbErr == nil {
			_ = sqlDatabase.Close()
		}
		return nil, fmt.Errorf("apply sqlite migrations: %w", err)
	}
	if err := database.WithContext(ctx).Exec(toolInvocationsMigration).Error; err != nil {
		sqlDatabase, dbErr := database.DB()
		if dbErr == nil {
			_ = sqlDatabase.Close()
		}
		return nil, fmt.Errorf("apply sqlite migrations: %w", err)
	}
	return database, nil
}
