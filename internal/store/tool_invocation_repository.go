package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/Karlsk/oryxos-go/internal/config"
	"gorm.io/gorm"
)

// ToolInvocationRepository writes one final outcome per logical Tool call.
type ToolInvocationRepository struct {
	database *gorm.DB
}

// NewToolInvocationRepository constructs a repository over a migrated database.
func NewToolInvocationRepository(database *gorm.DB) (*ToolInvocationRepository, error) {
	if database == nil {
		return nil, fmt.Errorf("create tool invocation repository: database is nil")
	}
	return &ToolInvocationRepository{database: database}, nil
}

// Create validates, sanitizes, and inserts one final Tool invocation record.
func (repository *ToolInvocationRepository) Create(ctx context.Context, invocation *ToolInvocation) error {
	if repository == nil || repository.database == nil {
		return fmt.Errorf("create tool invocation: repository is nil")
	}
	if ctx == nil {
		return fmt.Errorf("create tool invocation: context is nil")
	}
	if invocation != nil && invocation.ErrorMessage != nil {
		sanitized := config.SanitizeErrorString(*invocation.ErrorMessage)
		invocation.ErrorMessage = &sanitized
	}
	if err := validateToolInvocation(invocation); err != nil {
		return err
	}
	if err := repository.database.WithContext(ctx).Create(invocation).Error; err != nil {
		return fmt.Errorf("create tool invocation: %w", err)
	}
	return nil
}

func validateToolInvocation(invocation *ToolInvocation) error {
	if invocation == nil {
		return fmt.Errorf("create tool invocation: record is nil")
	}
	if strings.TrimSpace(invocation.SessionID) == "" {
		return fmt.Errorf("create tool invocation: session_id is required")
	}
	if strings.TrimSpace(invocation.ToolName) == "" {
		return fmt.Errorf("create tool invocation: tool_name is required")
	}
	if invocation.DurationMS < 0 {
		return fmt.Errorf("create tool invocation: duration_ms must be non-negative")
	}
	if invocation.CreatedAt.IsZero() {
		return fmt.Errorf("create tool invocation: created_at is required")
	}
	if invocation.Success && invocation.ErrorMessage != nil {
		return fmt.Errorf("create tool invocation: successful record must not contain error_message")
	}
	if !invocation.Success && (invocation.ErrorMessage == nil || strings.TrimSpace(*invocation.ErrorMessage) == "") {
		return fmt.Errorf("create tool invocation: failed record requires error_message")
	}
	return nil
}
