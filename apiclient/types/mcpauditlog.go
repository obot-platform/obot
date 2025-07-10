package types

import "encoding/json"

// MCPAuditLog represents an audit log entry for MCP API calls
type MCPAuditLog struct {
	ID                        uint            `json:"id"`
	CreatedAt                 Time            `json:"createdAt"`
	UserID                    string          `json:"userID"`
	MCPID                     string          `json:"mcpID"`
	MCPServerDisplayName      string          `json:"mcpServerDisplayName"`
	MCPServerCatalogEntryName string          `json:"mcpServerCatalogEntryName"`
	ClientInfo                ClientInfo      `json:"client"`
	ClientIP                  string          `json:"clientIP"`
	CallType                  string          `json:"callType"`
	CallIdentifier            string          `json:"callIdentifier,omitempty"`
	RequestBody               json.RawMessage `json:"requestBody,omitempty"`
	ResponseBody              json.RawMessage `json:"responseBody,omitempty"`
	ResponseStatus            int             `json:"responseStatus"`
	Error                     string          `json:"error,omitempty"`
	ProcessingTimeMs          int64           `json:"processingTimeMs"`
	SessionID                 string          `json:"sessionID,omitempty"`
	RequestID                 string          `json:"requestID,omitempty"`
	UserAgent                 string          `json:"userAgent,omitempty"`
	RequestHeaders            json.RawMessage `json:"requestHeaders,omitempty"`
	ResponseHeaders           json.RawMessage `json:"responseHeaders,omitempty"`
}

type MCPAuditLogResponse struct {
	MCPAuditLogList `json:",inline"`
	Total           int64 `json:"total"`
	Limit           int   `json:"limit"`
	Offset          int   `json:"offset"`
}

type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// MCPAuditLogList represents a list of MCP audit logs
type MCPAuditLogList List[MCPAuditLog]

// MCPUsageStats represents usage statistics for MCP servers
type MCPUsageStats struct {
	MCPID                     string                 `json:"mcpID"`
	MCPServerDisplayName      string                 `json:"mcpServerDisplayName"`
	MCPServerCatalogEntryName string                 `json:"mcpServerCatalogEntryName"`
	TimeStart                 Time                   `json:"timeStart"`
	TimeEnd                   Time                   `json:"timeEnd"`
	TotalCalls                int64                  `json:"totalCalls"`
	UniqueUsers               int64                  `json:"uniqueUsers"`
	ToolCalls                 []MCPToolCallStats     `json:"toolCalls,omitempty"`
	ResourceReads             []MCPResourceReadStats `json:"resourceReads,omitempty"`
	PromptReads               []MCPPromptReadStats   `json:"promptReads,omitempty"`
}

// MCPToolCallStats represents statistics for individual tool calls
type MCPToolCallStats struct {
	ToolName  string `json:"toolName"`
	CallCount int64  `json:"callCount"`
}

// MCPResourceReadStats represents statistics for individual resource reads
type MCPResourceReadStats struct {
	ResourceURI string `json:"resourceURI"`
	ReadCount   int64  `json:"readCount"`
}

// MCPPromptReadStats represents statistics for individual prompt reads
type MCPPromptReadStats struct {
	PromptName string `json:"promptName"`
	ReadCount  int64  `json:"readCount"`
}

// MCPUsageStatsList represents a list of MCP usage statistics
type MCPUsageStatsList List[MCPUsageStats]
