package types

import "encoding/json"

// LocalAgentAuditLogIngest represents a local agent tool-call audit event submitted by a client.
type LocalAgentAuditLogIngest struct {
	LocalAgentAuditLogFields `json:",inline"`
	CreatedAt                *Time                          `json:"createdAt,omitempty"`
	Client                   LocalAgentAuditLogIngestClient `json:"client"`
	PayloadTruncated         bool                           `json:"payloadTruncated,omitempty"`
}

type LocalAgentAuditLogIngestClient struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type LocalAgentAuditLogFields struct {
	EventID            string          `json:"eventID"`
	ToolName           string          `json:"toolName,omitempty"`
	ToolType           string          `json:"toolType,omitempty"`
	EventName          string          `json:"eventName"`
	Success            *bool           `json:"success,omitempty"`
	Status             string          `json:"status,omitempty"`
	ExitCode           *int            `json:"exitCode,omitempty"`
	DurationMs         *int64          `json:"durationMs,omitempty"`
	SessionID          string          `json:"sessionID,omitempty"`
	ConversationID     string          `json:"conversationID,omitempty"`
	RequestID          string          `json:"requestID,omitempty"`
	WorkspaceHash      string          `json:"workspaceHash,omitempty"`
	WorkspaceBasename  string          `json:"workspaceBasename,omitempty"`
	Error              string          `json:"error,omitempty"`
	RawClientHookEvent json.RawMessage `json:"rawClientHookEvent,omitempty"`
	RawToolInput       json.RawMessage `json:"rawToolInput,omitempty"`
	RawToolOutput      json.RawMessage `json:"rawToolOutput,omitempty"`
	RawError           json.RawMessage `json:"rawError,omitempty"`
}

// LocalAgentAuditLog represents a local agent tool-call audit event.
type LocalAgentAuditLog struct {
	LocalAgentAuditLogFields `json:",inline"`
	ID                       uint       `json:"id"`
	CreatedAt                Time       `json:"createdAt"`
	UserID                   string     `json:"userID"`
	ClientInfo               ClientInfo `json:"client"`
	ClientIP                 string     `json:"clientIP,omitempty"`
	PayloadTruncated         bool       `json:"payloadTruncated"`
}

type LocalAgentAuditLogIngestResponse struct {
	Accepted   int    `json:"accepted"`
	Inserted   int    `json:"inserted"`
	Duplicates int    `json:"duplicates"`
	IDs        []uint `json:"ids,omitempty"`
}

type LocalAgentAuditLogResponse struct {
	LocalAgentAuditLogList `json:",inline"`
	Total                  int64 `json:"total"`
	Limit                  int   `json:"limit"`
	Offset                 int   `json:"offset"`
}

type LocalAgentAuditLogList List[LocalAgentAuditLog]
