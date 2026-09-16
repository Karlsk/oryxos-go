package store

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// LlmCallRepository writes model-call outcomes using short context-aware inserts.
type LlmCallRepository struct {
	database *gorm.DB
}

// NewLlmCallRepository constructs a repository over a migrated database.
func NewLlmCallRepository(database *gorm.DB) (*LlmCallRepository, error) {
	if database == nil {
		return nil, fmt.Errorf("create llm call repository: database is nil")
	}
	return &LlmCallRepository{database: database}, nil
}

// Create validates and inserts one call record.
func (repository *LlmCallRepository) Create(ctx context.Context, call *LlmCall) error {
	if repository == nil || repository.database == nil {
		return fmt.Errorf("create llm call: repository is nil")
	}
	if ctx == nil {
		return fmt.Errorf("create llm call: context is nil")
	}
	if err := validateLlmCall(call); err != nil {
		return err
	}
	if err := repository.database.WithContext(ctx).Create(call).Error; err != nil {
		return fmt.Errorf("create llm call: %w", err)
	}
	return nil
}

func validateLlmCall(call *LlmCall) error {
	if call == nil {
		return fmt.Errorf("create llm call: record is nil")
	}
	if strings.TrimSpace(call.SessionID) == "" {
		return fmt.Errorf("create llm call: session_id is required")
	}
	if strings.TrimSpace(call.Provider) == "" {
		return fmt.Errorf("create llm call: provider is required")
	}
	if strings.TrimSpace(call.Model) == "" {
		return fmt.Errorf("create llm call: model is required")
	}
	if call.PromptTokens < 0 || call.CompletionTokens < 0 || call.TotalTokens < 0 {
		return fmt.Errorf("create llm call: token counts must be non-negative")
	}
	if call.DurationMS < 0 {
		return fmt.Errorf("create llm call: duration_ms must be non-negative")
	}
	if call.CreatedAt.IsZero() {
		return fmt.Errorf("create llm call: created_at is required")
	}
	if call.Success && call.ErrorMessage != nil {
		return fmt.Errorf("create llm call: successful record must not contain error_message")
	}
	return nil
}
