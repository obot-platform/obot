//nolint:revive
package types

import (
	"encoding/json"
	"fmt"
	"time"
	"unicode/utf8"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	AuditLogSourceTypeMCP        = "mcp"
	AuditLogSourceTypeLocalAgent = "local_agent"

	AuditLogEventTypeToolCall     = "tool_call"
	AuditLogEventTypeResourceRead = "resource_read"
	AuditLogEventTypePromptGet    = "prompt_get"
	AuditLogEventTypeMCPRequest   = "mcp_request"

	AuditLogOutcomeSuccess = "success"
	AuditLogOutcomeError   = "error"
)

// maxErrorSummaryBytes caps the plaintext, searchable Error column for events
// that carry a full error payload in the encrypted ErrorDetail field.
const maxErrorSummaryBytes = 1024

func truncateUTF8ByBytes(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}

	for maxBytes > 0 && !utf8.ValidString(s[:maxBytes]) {
		maxBytes--
	}

	return s[:maxBytes]
}

// MCPAuditLogFields are meaningful only for MCP gateway/shim rows.
type MCPAuditLogFields struct {
	APIKey                    string                                `json:"apiKey,omitempty"`
	MCPID                     string                                `json:"mcpID" gorm:"index"`
	PowerUserWorkspaceID      string                                `json:"powerUserWorkspaceID,omitempty" gorm:"index"`
	MCPServerDisplayName      string                                `json:"mcpServerDisplayName" gorm:"index"`
	MCPServerCatalogEntryName string                                `json:"mcpServerCatalogEntryName" gorm:"index"`
	ClientIP                  string                                `json:"clientIP" gorm:"index"`
	RequestMutated            bool                                  `json:"requestMutated"`
	MutatedRequestBody        json.RawMessage                       `json:"mutatedRequestBody,omitempty"`
	ResponseMutated           bool                                  `json:"responseMutated"`
	OriginalResponseBody      json.RawMessage                       `json:"originalResponseBody,omitempty"`
	ResponseStatus            int                                   `json:"responseStatus" gorm:"index"`
	WebhookStatuses           datatypes.JSONSlice[MCPWebhookStatus] `json:"webhookStatuses,omitempty"`
	RequestID                 string                                `json:"requestID,omitempty" gorm:"index"`
	UserAgent                 string                                `json:"userAgent,omitempty"`
	RequestHeaders            json.RawMessage                       `json:"requestHeaders,omitempty"`
	ResponseHeaders           json.RawMessage                       `json:"responseHeaders,omitempty"`
}

// LocalAuditLog contains fields introduced for local-agent audit events.
type LocalAuditLog struct {
	// ErrorDetail holds the full error text for events whose Error column is a
	// truncated summary. Encrypted at rest like the request/response payloads.
	ErrorDetail string `json:"errorDetail,omitempty"`
	// RawEvent preserves the original client payload (e.g. the local agent hook
	// JSON) for debugging parser drift. Encrypted at rest.
	RawEvent json.RawMessage `json:"rawEvent,omitempty"`

	// Context holds source-specific, non-indexed metadata (workspace, git
	// remote, hostname, etc.). See apiclient types.AuditLogContext for the
	// canonical shape.
	Context datatypes.JSON `json:"context,omitempty"`
	// PayloadMeta records per-payload-field truncation info, keyed by field
	// name ("request", "response", "error", "rawEvent").
	PayloadMeta datatypes.JSON `json:"payloadMeta,omitempty"`
}

// MCPAuditLog represents an audit log entry. Despite the name (kept for
// storage compatibility), it stores generic audit events distinguished by
// SourceType; MCP-specific fields are empty for non-MCP rows.
type MCPAuditLog struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time `json:"createdAt" gorm:"index"`

	// SourceType, EventType, and Outcome are backfilled on historical rows by a
	// startup migration; EventID and ReceivedAt remain NULL on rows that predate them.
	EventID    *string    `json:"eventID,omitempty" gorm:"uniqueIndex"`
	SourceType string     `json:"sourceType,omitempty" gorm:"index"`
	EventType  string     `json:"eventType,omitempty" gorm:"index"`
	ReceivedAt *time.Time `json:"receivedAt,omitempty"`
	DeviceID   string     `json:"deviceID,omitempty" gorm:"index"`
	Outcome    string     `json:"outcome,omitempty" gorm:"index"`
	UserID     string     `json:"userID" gorm:"index"`

	// MCP rows map their JSON-RPC call type and identifier into these same
	// indexed columns so local-agent events can reuse list, filter, and export paths.
	ClientName       string          `json:"clientName" gorm:"index"`
	ClientVersion    string          `json:"clientVersion" gorm:"index"`
	CallType         string          `json:"callType" gorm:"index"`
	CallIdentifier   string          `json:"callIdentifier,omitempty" gorm:"index"`
	RequestBody      json.RawMessage `json:"requestBody,omitempty"`
	ResponseBody     json.RawMessage `json:"responseBody,omitempty"`
	Error            string          `json:"error,omitempty"`
	ProcessingTimeMs int64           `json:"processingTimeMs" gorm:"index"`
	SessionID        string          `json:"sessionID,omitempty" gorm:"index"`

	// Exactly one source-specific struct should be non-nil after normalization.
	// Both are embedded into the same physical mcp_audit_logs table so existing
	// MCP rows and indexes remain column-compatible.
	MCP   *MCPAuditLogFields `json:"mcp,omitempty" gorm:"embedded"`
	Local *LocalAuditLog     `json:"local,omitempty" gorm:"embedded"`

	ResponseReceived bool `json:"responseReceived"`
	Encrypted        bool `json:"encrypted"`
}

func (a *MCPAuditLog) EnsureMCP() *MCPAuditLogFields {
	if a.MCP == nil {
		a.MCP = new(MCPAuditLogFields)
	}
	return a.MCP
}

func (a *MCPAuditLog) EnsureLocal() *LocalAuditLog {
	if a.Local == nil {
		a.Local = new(LocalAuditLog)
	}
	return a.Local
}

func (a MCPAuditLog) MCPFields() MCPAuditLogFields {
	if a.MCP == nil {
		return MCPAuditLogFields{}
	}
	return *a.MCP
}

func (a MCPAuditLog) LocalFields() LocalAuditLog {
	if a.Local == nil {
		return LocalAuditLog{}
	}
	return *a.Local
}

func (a *MCPAuditLog) NormalizeSourceFields() {
	switch a.SourceType {
	case AuditLogSourceTypeLocalAgent:
		a.EnsureLocal()
		a.MCP = nil
	case AuditLogSourceTypeMCP:
		a.EnsureMCP()
		a.Local = nil
	default:
		switch {
		case a.Local != nil && a.MCP == nil:
			a.SourceType = AuditLogSourceTypeLocalAgent
		default:
			a.SourceType = AuditLogSourceTypeMCP
			a.EnsureMCP()
			a.Local = nil
		}
	}
}

func (a *MCPAuditLog) BeforeSave(*gorm.DB) error {
	a.NormalizeSourceFields()
	return nil
}

func (a *MCPAuditLog) AfterFind(*gorm.DB) error {
	a.NormalizeSourceFields()
	return nil
}

// EventTypeForCallType maps an MCP call type to the generic audit event type.
func EventTypeForCallType(callType string) string {
	switch callType {
	case "tools/call":
		return AuditLogEventTypeToolCall
	case "resources/read":
		return AuditLogEventTypeResourceRead
	case "prompts/get":
		return AuditLogEventTypePromptGet
	default:
		return AuditLogEventTypeMCPRequest
	}
}

// OutcomeForResult maps an error string and response status to an outcome.
func OutcomeForResult(errMsg string, responseStatus int) string {
	// responseStatus==0 indicates we haven't observed a response yet (request-only row).
	if errMsg == "" && responseStatus == 0 {
		return ""
	}
	if errMsg == "" && responseStatus < 400 {
		return AuditLogOutcomeSuccess
	}
	return AuditLogOutcomeError
}

type MCPWebhookStatus struct {
	Type    string `json:"type,omitempty"`
	URL     string `json:"url,omitempty"`
	Method  string `json:"method,omitempty"`
	Name    string `json:"name,omitempty"`
	Tool    string `json:"tool,omitempty"`
	Status  string `json:"status,omitempty"`
	Message string `json:"message,omitempty"`
}

// MCPUsageStatItem represents usage statistics for MCP servers
type MCPUsageStatItem struct {
	MCPID                     string                 `json:"mcpID"`
	MCPServerDisplayName      string                 `json:"mcpServerDisplayName"`
	MCPServerCatalogEntryName string                 `json:"mcpServerCatalogEntryName"`
	ToolCalls                 []MCPToolCallStats     `json:"toolCalls,omitempty"`
	ResourceReads             []MCPResourceReadStats `json:"resourceReads,omitempty"`
	PromptReads               []MCPPromptReadStats   `json:"promptReads,omitempty"`
}

type MCPUsageStatsList struct {
	TotalCalls  int64              `json:"totalCalls"`
	UniqueUsers int64              `json:"uniqueUsers"`
	TimeStart   time.Time          `json:"timeStart"`
	TimeEnd     time.Time          `json:"timeEnd"`
	Items       []MCPUsageStatItem `json:"items"`
}

type MCPToolCallStatsItem struct {
	ToolName         string    `json:"toolName"`
	CreatedAt        time.Time `json:"createdAt"`
	UserID           string    `json:"userID"`
	ProcessingTimeMs int64     `json:"processingTimeMs"`
	ResponseStatus   int       `json:"responseStatus"`
	Error            string    `json:"error"`
}

// MCPToolCallStats represents statistics for individual tool calls
type MCPToolCallStats struct {
	ToolName  string                 `json:"-"`
	CallCount int64                  `json:"callCount"`
	Items     []MCPToolCallStatsItem `json:"items"`
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
	mcpFields := a.MCPFields()
	localFields := a.LocalFields()

	webhookStatus := make([]types2.WebhookStatus, len(mcpFields.WebhookStatuses))
	for i, ws := range mcpFields.WebhookStatuses {
		webhookStatus[i] = types2.WebhookStatus{
			Type:    ws.Type,
			Method:  ws.Method,
			URL:     ws.URL,
			Name:    ws.Name,
			Tool:    ws.Tool,
			Status:  ws.Status,
			Message: ws.Message,
		}
	}

	var eventID string
	if a.EventID != nil {
		eventID = *a.EventID
	}

	var receivedAt *types2.Time
	if a.ReceivedAt != nil {
		receivedAt = types2.NewTime(*a.ReceivedAt)
	}

	var context *types2.AuditLogContext
	if len(localFields.Context) > 0 {
		context = new(types2.AuditLogContext)
		if err := json.Unmarshal(localFields.Context, context); err != nil {
			context = nil
		}
	}

	var payloadMeta map[string]types2.PayloadFieldMeta
	if len(localFields.PayloadMeta) > 0 {
		if err := json.Unmarshal(localFields.PayloadMeta, &payloadMeta); err != nil {
			payloadMeta = nil
		}
	}

	apiLog := types2.MCPAuditLog{
		ID:         a.ID,
		EventID:    eventID,
		SourceType: a.SourceType,
		EventType:  a.EventType,
		CreatedAt:  *types2.NewTime(a.CreatedAt),
		ReceivedAt: receivedAt,
		UserID:     a.UserID,
		DeviceID:   a.DeviceID,
		Outcome:    a.Outcome,
		ClientInfo: types2.ClientInfo{
			Name:    a.ClientName,
			Version: a.ClientVersion,
		},
		CallType:         a.CallType,
		CallIdentifier:   a.CallIdentifier,
		RequestBody:      a.RequestBody,
		ResponseBody:     a.ResponseBody,
		Error:            a.Error,
		ProcessingTimeMs: a.ProcessingTimeMs,
		SessionID:        a.SessionID,
		ResponseReceived: a.ResponseReceived,
	}

	switch a.SourceType {
	case AuditLogSourceTypeLocalAgent:
		apiLog.Local = &types2.LocalAuditLog{
			ErrorDetail: localFields.ErrorDetail,
			RawEvent:    localFields.RawEvent,
			Context:     context,
			PayloadMeta: payloadMeta,
		}
	default:
		apiLog.MCP = &types2.MCPAuditLogMCP{
			MCPID:                     mcpFields.MCPID,
			APIKey:                    mcpFields.APIKey,
			PowerUserWorkspaceID:      mcpFields.PowerUserWorkspaceID,
			MCPServerDisplayName:      mcpFields.MCPServerDisplayName,
			MCPServerCatalogEntryName: mcpFields.MCPServerCatalogEntryName,
			ClientIP:                  mcpFields.ClientIP,
			RequestMutated:            mcpFields.RequestMutated,
			MutatedRequestBody:        mcpFields.MutatedRequestBody,
			ResponseMutated:           mcpFields.ResponseMutated,
			OriginalResponseBody:      mcpFields.OriginalResponseBody,
			ResponseStatus:            mcpFields.ResponseStatus,
			WebhookStatuses:           webhookStatus,
			RequestID:                 mcpFields.RequestID,
			UserAgent:                 mcpFields.UserAgent,
			RequestHeaders:            mcpFields.RequestHeaders,
			ResponseHeaders:           mcpFields.ResponseHeaders,
		}
	}

	return apiLog
}

// MCPAuditLogFromAuditEvent converts a canonical generic audit event into the
// internal storage type. The nested client/tool fields map onto the existing
// generic-named indexed columns; MCP-specific fields are left empty.
//
// UserID and ReceivedAt are deliberately never copied from the event: they are
// server-assigned (from the authenticated user and receipt time respectively),
// so client-provided values must not reach storage.
func MCPAuditLogFromAuditEvent(e types2.AuditEvent) (MCPAuditLog, error) {
	log := MCPAuditLog{
		CreatedAt:        e.CreatedAt.Time.UTC(),
		SourceType:       e.SourceType,
		EventType:        e.EventType,
		DeviceID:         e.DeviceID,
		Outcome:          e.Outcome,
		ClientName:       e.Client.Name,
		ClientVersion:    e.Client.Version,
		CallType:         e.Tool.Type,
		CallIdentifier:   e.Tool.Name,
		ProcessingTimeMs: e.DurationMs,
		SessionID:        e.SessionID,
		RequestBody:      e.Request,
		ResponseBody:     e.Response,
		Error:            e.Error,
		Local: &LocalAuditLog{
			RawEvent: e.RawEvent,
		},
		// Generic events arrive complete; never match them against the
		// request/response merge path used by two-phase MCP shim logs.
		ResponseReceived: true,
	}

	if e.EventID != "" {
		eventID := e.EventID
		log.EventID = &eventID
	}

	// Keep a size-capped plaintext summary in the searchable Error column and
	// the full text in the encrypted ErrorDetail field.
	if len(e.Error) > maxErrorSummaryBytes {
		log.Error = truncateUTF8ByBytes(e.Error, maxErrorSummaryBytes)
		log.EnsureLocal().ErrorDetail = e.Error
	}

	if e.Context != nil {
		b, err := json.Marshal(e.Context)
		if err != nil {
			return MCPAuditLog{}, fmt.Errorf("failed to marshal audit event context: %w", err)
		}
		log.EnsureLocal().Context = datatypes.JSON(b)
	}

	if len(e.PayloadMeta) > 0 {
		b, err := json.Marshal(e.PayloadMeta)
		if err != nil {
			return MCPAuditLog{}, fmt.Errorf("failed to marshal audit event payload metadata: %w", err)
		}
		log.EnsureLocal().PayloadMeta = datatypes.JSON(b)
	}

	return log, nil
}

// ConvertAuditEvent converts an internal audit log row to the canonical
// generic audit event shape.
func ConvertAuditEvent(a MCPAuditLog) types2.AuditEvent {
	apiLog := ConvertMCPAuditLog(a)
	localFields := apiLog.Local
	if localFields == nil {
		localFields = new(types2.LocalAuditLog)
	}

	event := types2.AuditEvent{
		EventID:    apiLog.EventID,
		SourceType: apiLog.SourceType,
		EventType:  apiLog.EventType,
		CreatedAt:  apiLog.CreatedAt,
		ReceivedAt: apiLog.ReceivedAt,
		UserID:     apiLog.UserID,
		DeviceID:   apiLog.DeviceID,
		Client:     apiLog.ClientInfo,
		Tool: types2.ToolInfo{
			Name: apiLog.CallIdentifier,
			Type: apiLog.CallType,
		},
		Outcome:     apiLog.Outcome,
		DurationMs:  apiLog.ProcessingTimeMs,
		SessionID:   apiLog.SessionID,
		Request:     apiLog.RequestBody,
		Response:    apiLog.ResponseBody,
		Error:       apiLog.Error,
		RawEvent:    localFields.RawEvent,
		Context:     localFields.Context,
		PayloadMeta: localFields.PayloadMeta,
	}

	if localFields.ErrorDetail != "" {
		event.Error = localFields.ErrorDetail
	}

	return event
}

// ConvertMCPUsageStats converts internal MCPUsageStatItem to API type
func ConvertMCPUsageStats(s MCPUsageStatItem) types2.MCPUsageStatItem {
	toolCalls := make([]types2.MCPToolCallStats, len(s.ToolCalls))
	for i, tc := range s.ToolCalls {
		items := make([]types2.MCPToolCallStatsItem, len(tc.Items))
		for j, item := range tc.Items {
			items[j] = types2.MCPToolCallStatsItem{
				CreatedAt:        *types2.NewTime(item.CreatedAt),
				UserID:           item.UserID,
				ProcessingTimeMs: item.ProcessingTimeMs,
				ResponseStatus:   item.ResponseStatus,
				Error:            item.Error,
			}
		}

		toolCalls[i] = types2.MCPToolCallStats{
			ToolName:  tc.ToolName,
			CallCount: tc.CallCount,
			Items:     items,
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

	return types2.MCPUsageStatItem{
		MCPID:                     s.MCPID,
		MCPServerDisplayName:      s.MCPServerDisplayName,
		MCPServerCatalogEntryName: s.MCPServerCatalogEntryName,
		ToolCalls:                 toolCalls,
		ResourceReads:             resourceReads,
		PromptReads:               promptReads,
	}
}
