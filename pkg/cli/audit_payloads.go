package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"
)

type auditHookPayload struct {
	EventID         string
	CreatedAt       time.Time
	ClientVersion   string
	ToolName        string
	ToolType        string
	Outcome         string
	DurationMs      int64
	SessionID       string
	ConversationID  string
	CWD             string
	Workspace       string
	SourceHookEvent string
	ClientEventID   string
	Request         json.RawMessage
	Response        json.RawMessage
	Error           string
}

type claudeCodeHookPayload struct {
	EventID       string            `json:"eventID"`
	HookEventName string            `json:"hook_event_name"`
	SessionID     string            `json:"session_id"`
	TranscriptID  string            `json:"transcript_id"`
	CWD           string            `json:"cwd"`
	ToolName      string            `json:"tool_name"`
	ToolInput     json.RawMessage   `json:"tool_input"`
	ToolResponse  json.RawMessage   `json:"tool_response"`
	Error         auditHookError    `json:"error"`
	DurationMs    int64             `json:"duration_ms"`
	Timestamp     auditFlexibleTime `json:"timestamp"`
	Client        struct {
		Version string `json:"version"`
	} `json:"client"`
}

func (p claudeCodeHookPayload) auditHookPayload() auditHookPayload {
	return auditHookPayload{
		EventID:         p.EventID,
		CreatedAt:       p.Timestamp.Time,
		ClientVersion:   p.Client.Version,
		ToolName:        p.ToolName,
		ToolType:        "tool",
		Outcome:         auditOutcome(p.HookEventName, p.Error.String(), ""),
		DurationMs:      p.DurationMs,
		SessionID:       p.SessionID,
		ConversationID:  p.TranscriptID,
		CWD:             p.CWD,
		SourceHookEvent: p.HookEventName,
		ClientEventID:   p.EventID,
		Request:         p.ToolInput,
		Response:        p.ToolResponse,
		Error:           p.Error.String(),
	}
}

type codexHookPayload struct {
	HookEventName string          `json:"hook_event_name"`
	SessionID     string          `json:"session_id"`
	CWD           string          `json:"cwd"`
	ToolName      string          `json:"tool_name"`
	ToolInput     json.RawMessage `json:"tool_input"`
	ToolResponse  json.RawMessage `json:"tool_response"`
	ToolUseID     string          `json:"tool_use_id"`
}

func (p codexHookPayload) auditHookPayload() auditHookPayload {
	return auditHookPayload{
		EventID:         p.ToolUseID,
		ToolName:        p.ToolName,
		ToolType:        "tool",
		Outcome:         auditOutcome(p.HookEventName, "", ""),
		SessionID:       p.SessionID,
		CWD:             p.CWD,
		SourceHookEvent: p.HookEventName,
		ClientEventID:   p.ToolUseID,
		Request:         p.ToolInput,
		Response:        p.ToolResponse,
	}
}

type cursorHookPayload struct {
	HookEventName string          `json:"hook_event_name"`
	Conversation  string          `json:"conversation_id"`
	GenerationID  string          `json:"generation_id"`
	SessionID     string          `json:"session_id"`
	ClientVersion string          `json:"cursor_version"`
	Workspace     []string        `json:"workspace_roots"`
	ToolName      string          `json:"tool_name"`
	ToolInput     json.RawMessage `json:"tool_input"`
	ToolOutput    json.RawMessage `json:"tool_output"`
	ResultJSON    json.RawMessage `json:"result_json"`
	ToolUseID     string          `json:"tool_use_id"`
	ErrorMessage  string          `json:"error_message"`
	FailureType   string          `json:"failure_type"`
	DurationMs    int64           `json:"duration_ms"`
	Duration      int64           `json:"duration"`
	CWD           string          `json:"cwd"`
}

func (p cursorHookPayload) auditHookPayload() auditHookPayload {
	response := p.ToolOutput
	if len(response) == 0 {
		response = p.ResultJSON
	}
	errorText := p.ErrorMessage
	if errorText == "" {
		errorText = p.FailureType
	}
	workspace := ""
	if len(p.Workspace) > 0 {
		workspace = p.Workspace[0]
	}
	duration := p.DurationMs
	if duration == 0 {
		duration = p.Duration
	}
	return auditHookPayload{
		EventID:         p.ToolUseID,
		ClientVersion:   p.ClientVersion,
		ToolName:        p.ToolName,
		ToolType:        "tool",
		Outcome:         auditOutcome(p.HookEventName, errorText, ""),
		DurationMs:      duration,
		SessionID:       firstNonEmpty(p.SessionID, p.Conversation),
		ConversationID:  p.Conversation,
		CWD:             p.CWD,
		Workspace:       workspace,
		SourceHookEvent: p.HookEventName,
		ClientEventID:   firstNonEmpty(p.ToolUseID, p.GenerationID),
		Request:         p.ToolInput,
		Response:        response,
		Error:           errorText,
	}
}

type vscodeHookPayload struct {
	HookEventName string            `json:"hook_event_name"`
	SessionID     string            `json:"session_id"`
	CWD           string            `json:"cwd"`
	ToolName      string            `json:"tool_name"`
	ToolInput     json.RawMessage   `json:"tool_input"`
	ToolResponse  json.RawMessage   `json:"tool_response"`
	ToolUseID     string            `json:"tool_use_id"`
	Error         auditHookError    `json:"error"`
	Timestamp     auditFlexibleTime `json:"timestamp"`
}

func (p vscodeHookPayload) auditHookPayload() auditHookPayload {
	return auditHookPayload{
		CreatedAt:       p.Timestamp.Time,
		ToolName:        p.ToolName,
		ToolType:        "tool",
		Outcome:         auditOutcome(p.HookEventName, p.Error.String(), ""),
		SessionID:       p.SessionID,
		CWD:             p.CWD,
		SourceHookEvent: p.HookEventName,
		ClientEventID:   p.ToolUseID,
		Request:         p.ToolInput,
		Response:        p.ToolResponse,
		Error:           p.Error.String(),
	}
}

type auditFlexibleTime struct {
	Time time.Time
}

func (t *auditFlexibleTime) UnmarshalJSON(data []byte) error {
	if bytes.Equal(data, []byte("null")) || len(data) == 0 {
		return nil
	}
	var raw any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&raw); err != nil {
		return err
	}
	parsed, ok := parseAuditTime(raw)
	if ok {
		t.Time = parsed.UTC()
	}
	return nil
}

type auditHookError struct {
	Message string
	Raw     json.RawMessage
}

func (e *auditHookError) UnmarshalJSON(data []byte) error {
	if bytes.Equal(data, []byte("null")) || len(data) == 0 {
		return nil
	}
	e.Raw = append(e.Raw[:0], data...)
	var msg string
	if err := json.Unmarshal(data, &msg); err == nil {
		e.Message = strings.TrimSpace(msg)
		return nil
	}
	var object struct {
		Message string `json:"message"`
		Error   string `json:"error"`
		Detail  string `json:"detail"`
	}
	if err := json.Unmarshal(data, &object); err == nil {
		e.Message = firstNonEmpty(object.Message, object.Error, object.Detail)
	}
	return nil
}

func (e auditHookError) String() string {
	if e.Message != "" {
		return e.Message
	}
	if len(e.Raw) == 0 {
		return ""
	}
	return string(e.Raw)
}
