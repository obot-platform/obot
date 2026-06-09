//nolint:revive
package types

import (
	"encoding/json"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
)

// LocalAgentAuditLog represents an audit log entry for local agent tool calls.
type LocalAgentAuditLog struct {
	ID                 uint            `json:"id" gorm:"primaryKey"`
	EventID            string          `json:"eventID" gorm:"uniqueIndex;not null"`
	CreatedAt          time.Time       `json:"createdAt" gorm:"index"`
	UserID             string          `json:"userID" gorm:"index"`
	ClientName         string          `json:"clientName" gorm:"index"`
	ClientVersion      string          `json:"clientVersion" gorm:"index"`
	ClientIP           string          `json:"clientIP" gorm:"index"`
	ToolName           string          `json:"toolName" gorm:"index"`
	ToolType           string          `json:"toolType" gorm:"index"`
	EventName          string          `json:"eventName" gorm:"index"`
	Success            *bool           `json:"success" gorm:"index"`
	Status             string          `json:"status" gorm:"index"`
	ExitCode           *int            `json:"exitCode" gorm:"index"`
	DurationMs         *int64          `json:"durationMs" gorm:"index"`
	SessionID          string          `json:"sessionID" gorm:"index"`
	ConversationID     string          `json:"conversationID" gorm:"index"`
	RequestID          string          `json:"requestID" gorm:"index"`
	WorkspaceHash      string          `json:"workspaceHash" gorm:"index"`
	WorkspaceBasename  string          `json:"workspaceBasename" gorm:"index"`
	Error              string          `json:"error"`
	PayloadTruncated   bool            `json:"payloadTruncated" gorm:"index"`
	RawClientHookEvent json.RawMessage `json:"rawClientHookEvent,omitempty"`
	RawToolInput       json.RawMessage `json:"rawToolInput,omitempty"`
	RawToolOutput      json.RawMessage `json:"rawToolOutput,omitempty"`
	RawError           json.RawMessage `json:"rawError,omitempty"`
	Encrypted          bool            `json:"encrypted"`
}

func ConvertLocalAgentAuditLog(a LocalAgentAuditLog) types2.LocalAgentAuditLog {
	return types2.LocalAgentAuditLog{
		LocalAgentAuditLogFields: types2.LocalAgentAuditLogFields{
			EventID:            a.EventID,
			ToolName:           a.ToolName,
			ToolType:           a.ToolType,
			EventName:          a.EventName,
			Success:            a.Success,
			Status:             a.Status,
			ExitCode:           a.ExitCode,
			DurationMs:         a.DurationMs,
			SessionID:          a.SessionID,
			ConversationID:     a.ConversationID,
			RequestID:          a.RequestID,
			WorkspaceHash:      a.WorkspaceHash,
			WorkspaceBasename:  a.WorkspaceBasename,
			Error:              a.Error,
			RawClientHookEvent: a.RawClientHookEvent,
			RawToolInput:       a.RawToolInput,
			RawToolOutput:      a.RawToolOutput,
			RawError:           a.RawError,
		},
		ID:        a.ID,
		CreatedAt: *types2.NewTime(a.CreatedAt),
		UserID:    a.UserID,
		ClientInfo: types2.ClientInfo{
			Name:    a.ClientName,
			Version: a.ClientVersion,
		},
		ClientIP:         a.ClientIP,
		PayloadTruncated: a.PayloadTruncated,
	}
}
