package types

import (
	"encoding/json"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
)

// MCPAuditLog represents an audit log entry for MCP API calls
type MCPAuditLog struct {
	ID                   uint            `json:"id" gorm:"primaryKey"`
	CreatedAt            time.Time       `json:"createdAt" gorm:"index"`
	UserID               string          `json:"userID" gorm:"index"`
	MCPID                string          `json:"mcpID" gorm:"index"`
	MCPServerDisplayName string          `json:"mcpServerDisplayName" gorm:"index"`
	ClientInfo           ClientInfo      `json:"clientInfo" gorm:"embedded"`
	ClientIP             string          `json:"clientIP"`
	CallType             string          `json:"callType" gorm:"index"`
	CallIdentifier       string          `json:"callIdentifier,omitempty" gorm:"index"`
	RequestBody          json.RawMessage `json:"requestBody,omitempty"`
	ResponseBody         json.RawMessage `json:"responseBody,omitempty"`
	ResponseStatus       int             `json:"responseStatus"`
	Error                string          `json:"error,omitempty"`
	ProcessingTimeMs     int64           `json:"processingTimeMs"`
	SessionID            string          `json:"sessionID,omitempty"`

	// Additional metadata
	RequestID       string          `json:"requestID,omitempty" gorm:"index"`
	UserAgent       string          `json:"userAgent,omitempty"`
	RequestHeaders  json.RawMessage `json:"requestHeaders,omitempty"`
	ResponseHeaders json.RawMessage `json:"responseHeaders,omitempty"`
}

type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// MCPUsageStats represents usage statistics for MCP servers
type MCPUsageStats struct {
	MCPID                string                 `json:"mcpID"`
	MCPServerDisplayName string                 `json:"mcpServerDisplayName"`
	TimeStart            time.Time              `json:"timeStart"`
	TimeEnd              time.Time              `json:"timeEnd"`
	TotalCalls           int64                  `json:"totalCalls"`
	UniqueUsers          int64                  `json:"uniqueUsers"`
	ToolCalls            []MCPToolCallStats     `json:"toolCalls,omitempty"`
	ResourceReads        []MCPResourceReadStats `json:"resourceReads,omitempty"`
	PromptReads          []MCPPromptReadStats   `json:"promptReads,omitempty"`
}

// MCPToolCallStats represents statistics for individual tool calls
type MCPToolCallStats struct {
	ToolName  string `json:"toolName"`
	CallCount int64  `json:"callCount"`
}

// MCPResourceReadStats represents statistics for individual resource reads
type MCPResourceReadStats struct {
	ResourceURI string `json:"resourceUri"`
	ReadCount   int64  `json:"readCount"`
}

// MCPPromptReadStats represents statistics for individual prompt reads
type MCPPromptReadStats struct {
	PromptName string `json:"promptName"`
	ReadCount  int64  `json:"readCount"`
}

// ConvertMCPAuditLog converts internal MCPAuditLog to API type
func ConvertMCPAuditLog(a MCPAuditLog) types2.MCPAuditLog {
	return types2.MCPAuditLog{
		ID:                   a.ID,
		CreatedAt:            *types2.NewTime(a.CreatedAt),
		UserID:               a.UserID,
		MCPID:                a.MCPID,
		MCPServerDisplayName: a.MCPServerDisplayName,
		ClientInfo:           types2.ClientInfo(a.ClientInfo),
		ClientIP:             a.ClientIP,
		CallType:             a.CallType,
		CallIdentifier:       a.CallIdentifier,
		RequestBody:          a.RequestBody,
		ResponseBody:         a.ResponseBody,
		ResponseStatus:       a.ResponseStatus,
		Error:                a.Error,
		ProcessingTimeMs:     a.ProcessingTimeMs,
		SessionID:            a.SessionID,
		RequestID:            a.RequestID,
		UserAgent:            a.UserAgent,
		RequestHeaders:       a.RequestHeaders,
		ResponseHeaders:      a.ResponseHeaders,
	}
}

// ConvertMCPUsageStats converts internal MCPUsageStats to API type
func ConvertMCPUsageStats(s MCPUsageStats) types2.MCPUsageStats {
	toolCalls := make([]types2.MCPToolCallStats, len(s.ToolCalls))
	for i, tc := range s.ToolCalls {
		toolCalls[i] = types2.MCPToolCallStats{
			ToolName:  tc.ToolName,
			CallCount: tc.CallCount,
		}
	}

	resourceReads := make([]types2.MCPResourceReadStats, len(s.ResourceReads))
	for i, rr := range s.ResourceReads {
		resourceReads[i] = types2.MCPResourceReadStats{
			ResourceURI: rr.ResourceURI,
			ReadCount:   rr.ReadCount,
		}
	}

	promptReads := make([]types2.MCPPromptReadStats, len(s.PromptReads))
	for i, pr := range s.PromptReads {
		promptReads[i] = types2.MCPPromptReadStats{
			PromptName: pr.PromptName,
			ReadCount:  pr.ReadCount,
		}
	}

	return types2.MCPUsageStats{
		MCPID:                s.MCPID,
		MCPServerDisplayName: s.MCPServerDisplayName,
		TimeStart:            *types2.NewTime(s.TimeStart),
		TimeEnd:              *types2.NewTime(s.TimeEnd),
		TotalCalls:           s.TotalCalls,
		UniqueUsers:          s.UniqueUsers,
		ToolCalls:            toolCalls,
		ResourceReads:        resourceReads,
		PromptReads:          promptReads,
	}
}
