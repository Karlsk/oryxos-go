package store

import "time"

// ToolInvocation maps exactly to the core tool_invocations audit table.
type ToolInvocation struct {
	ID           int64     `gorm:"column:id;primaryKey;autoIncrement"`
	SessionID    string    `gorm:"column:session_id;not null;index:idx_tool_invocations_session_id"`
	ToolName     string    `gorm:"column:tool_name;not null"`
	InputJSON    string    `gorm:"column:input_json;not null"`
	ResultJSON   string    `gorm:"column:result_json;not null"`
	Success      bool      `gorm:"column:success;not null"`
	ErrorMessage *string   `gorm:"column:error_message"`
	DurationMS   int64     `gorm:"column:duration_ms;not null"`
	CreatedAt    time.Time `gorm:"column:created_at;not null"`
}

// TableName keeps GORM aligned with the hand-maintained migration.
func (ToolInvocation) TableName() string { return "tool_invocations" }
