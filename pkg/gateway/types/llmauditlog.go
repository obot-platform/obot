package types

import "time"

type LLMAuditLog struct {
	ID              string    `gorm:"primaryKey"`
	CreatedAt       time.Time `gorm:"primaryKey"`
	Duration        int64
	UserID          string
	ModelProvider   string
	ModelID         string
	TargetModel     string
	RequestHeaders  string
	RequestBody     string
	ResponseHeaders string
	ResponseBody    string
	ResponseText    string
	ResponseStatus  int
	Outcome         string
	Error           string
	InputTokens     int
	OutputTokens    int
	RequestID       string
	UserAgent       string
	ClientIP        string
}

func (LLMAuditLog) TableName() string {
	return "llm_audit_logs"
}
