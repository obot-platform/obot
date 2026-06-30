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
	ReasoningEffort string
	RequestPath     string
	RequestMethod   string
	RequestHeaders  string
	RequestBody     string
	ResponseHeaders string
	ResponseBody    string
	ResponseText    string
	ResponseID      string
	ResponseStatus  int
	Outcome         string
	Error           string
	InputTokens     int
	OutputTokens    int
	RequestID       string
	Client          string
	ClientVersion   string
	ClientSessionID string
	ClientIP        string
}

func (LLMAuditLog) TableName() string {
	return "llm_audit_logs"
}
