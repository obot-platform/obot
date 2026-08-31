package types

import "encoding/json"

const (
	MCPTesterClientName               = "obot-mcp-tester"
	MCPTesterMaxModelBoundContentSize = 100 * 1024
	MCPTesterMaxRounds                = 10
)

type MCPTesterChatRole string

const (
	MCPTesterChatRoleUser      MCPTesterChatRole = "user"
	MCPTesterChatRoleAssistant MCPTesterChatRole = "assistant"
	MCPTesterChatRoleTool      MCPTesterChatRole = "tool"
)

type MCPTesterContentType string

const (
	MCPTesterContentTypeText     MCPTesterContentType = "text"
	MCPTesterContentTypeResource MCPTesterContentType = "resource"
)

// MCPTesterChatRequest is the complete, page-held context for one stateless
// MCP tester model continuation. The selected MCP server and model are always
// server-owned and therefore deliberately absent from this contract.
type MCPTesterChatRequest struct {
	Messages []MCPTesterChatMessage `json:"messages"`
	Tools    []MCPTesterTool        `json:"tools,omitempty"`
	Round    int                    `json:"round"`
}

type MCPTesterChatMessage struct {
	Role       MCPTesterChatRole    `json:"role"`
	Content    []MCPTesterContent   `json:"content,omitempty"`
	ToolCalls  []MCPTesterToolCall  `json:"toolCalls,omitempty"`
	ToolResult *MCPTesterToolResult `json:"toolResult,omitempty"`
}

type MCPTesterContent struct {
	Type     MCPTesterContentType `json:"type"`
	Text     string               `json:"text"`
	URI      string               `json:"uri,omitempty"`
	MIMEType string               `json:"mimeType,omitempty"`
}

type MCPTesterTool struct {
	Name         string          `json:"name"`
	Description  string          `json:"description,omitempty"`
	InputSchema  json.RawMessage `json:"inputSchema"`
	OutputSchema json.RawMessage `json:"outputSchema,omitempty"`
}

type MCPTesterToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type MCPTesterToolResultStatus string

const (
	MCPTesterToolResultStatusSuccess   MCPTesterToolResultStatus = "success"
	MCPTesterToolResultStatusError     MCPTesterToolResultStatus = "error"
	MCPTesterToolResultStatusRejected  MCPTesterToolResultStatus = "rejected"
	MCPTesterToolResultStatusCancelled MCPTesterToolResultStatus = "cancelled"
)

type MCPTesterToolResult struct {
	CallID  string                    `json:"callID"`
	Status  MCPTesterToolResultStatus `json:"status"`
	Content json.RawMessage           `json:"content"`
}

type MCPTesterStreamEventType string

const (
	MCPTesterStreamEventAssistantMessageStart MCPTesterStreamEventType = "assistant_message_start"
	MCPTesterStreamEventTextDelta             MCPTesterStreamEventType = "text_delta"
	MCPTesterStreamEventToolCalls             MCPTesterStreamEventType = "tool_calls"
	MCPTesterStreamEventCompletion            MCPTesterStreamEventType = "completion"
	MCPTesterStreamEventError                 MCPTesterStreamEventType = "error"
)

type MCPTesterCompletionReason string

const (
	MCPTesterCompletionReasonStop      MCPTesterCompletionReason = "stop"
	MCPTesterCompletionReasonToolCalls MCPTesterCompletionReason = "tool_calls"
	MCPTesterCompletionReasonMaxTokens MCPTesterCompletionReason = "max_tokens"
)

type MCPTesterErrorCode string

const (
	MCPTesterErrorAccessDenied        MCPTesterErrorCode = "access_denied"
	MCPTesterErrorInvalidRequest      MCPTesterErrorCode = "invalid_request"
	MCPTesterErrorModelUnavailable    MCPTesterErrorCode = "model_unavailable"
	MCPTesterErrorPolicyDenied        MCPTesterErrorCode = "policy_denied"
	MCPTesterErrorProvider            MCPTesterErrorCode = "provider_error"
	MCPTesterErrorUnsupportedResponse MCPTesterErrorCode = "unsupported_response"
	MCPTesterErrorCancelled           MCPTesterErrorCode = "cancelled"
)

// MCPTesterStreamEvent is the only event shape emitted to the browser. Exactly
// one payload field is populated for each Type.
type MCPTesterStreamEvent struct {
	Type   MCPTesterStreamEventType  `json:"type"`
	Delta  string                    `json:"delta,omitempty"`
	Calls  []MCPTesterToolCall       `json:"calls,omitempty"`
	Reason MCPTesterCompletionReason `json:"reason,omitempty"`
	Error  *MCPTesterStreamError     `json:"error,omitempty"`
}

type MCPTesterStreamError struct {
	Code      MCPTesterErrorCode `json:"code"`
	Message   string             `json:"message"`
	Retryable bool               `json:"retryable,omitempty"`
}

// MCPTesterErrorResponse is returned when a request fails before the event
// stream can start. Once streaming begins the same error shape is carried by
// an MCPTesterStreamEvent instead.
type MCPTesterErrorResponse struct {
	Error MCPTesterStreamError `json:"error"`
}
