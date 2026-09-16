package store

import "time"

// LlmCall maps exactly to the core llm_calls audit table.
type LlmCall struct {
	ID               int64     `gorm:"column:id;primaryKey;autoIncrement"`
	SessionID        string    `gorm:"column:session_id;not null;index:idx_llm_calls_session_id"`
	Provider         string    `gorm:"column:provider;not null"`
	Model            string    `gorm:"column:model;not null"`
	PromptTokens     int       `gorm:"column:prompt_tokens;not null;default:0"`
	CompletionTokens int       `gorm:"column:completion_tokens;not null;default:0"`
	TotalTokens      int       `gorm:"column:total_tokens;not null;default:0"`
	Success          bool      `gorm:"column:success;not null"`
	ErrorMessage     *string   `gorm:"column:error_message"`
	DurationMS       int64     `gorm:"column:duration_ms;not null"`
	CreatedAt        time.Time `gorm:"column:created_at;not null"`
}

// TableName keeps GORM aligned with the hand-maintained migration.
func (LlmCall) TableName() string { return "llm_calls" }
