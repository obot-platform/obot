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
	Event     string `json:"event"`
	EventID   string `json:"eventID"`
	SessionID string `json:"sessionID"`
	CWD       string `json:"cwd"`
	Tool      struct {
		Name     string          `json:"name"`
		Type     string          `json:"type"`
		Input    json.RawMessage `json:"input"`
		Response json.RawMessage `json:"response"`
		Error    auditHookError  `json:"error"`
	} `json:"tool"`
	Error      auditHookError    `json:"error"`
	DurationMs int64             `json:"durationMs"`
	CreatedAt  auditFlexibleTime `json:"createdAt"`
	Client     struct {
		Version string `json:"version"`
	} `json:"client"`
}

func (p codexHookPayload) auditHookPayload() auditHookPayload {
	errText := p.Error.String()
	if errText == "" {
		errText = p.Tool.Error.String()
	}
	return auditHookPayload{
		EventID:         p.EventID,
		CreatedAt:       p.CreatedAt.Time,
		ClientVersion:   p.Client.Version,
		ToolName:        p.Tool.Name,
		ToolType:        firstNonEmpty(p.Tool.Type, "tool"),
		Outcome:         auditOutcome(p.Event, errText, ""),
		DurationMs:      p.DurationMs,
		SessionID:       p.SessionID,
		CWD:             p.CWD,
		SourceHookEvent: p.Event,
		ClientEventID:   p.EventID,
		Request:         p.Tool.Input,
		Response:        p.Tool.Response,
		Error:           errText,
	}
}

type cursorHookPayload struct {
	EventID       string          `json:"eventID"`
	EventName     string          `json:"eventName"`
	SessionID     string          `json:"sessionID"`
	Conversation  string          `json:"conversationID"`
	ClientVersion string          `json:"clientVersion"`
	ToolName      string          `json:"toolName"`
	ToolType      string          `json:"toolType"`
	Args          json.RawMessage `json:"args"`
	Response      json.RawMessage `json:"response"`
	Result        json.RawMessage `json:"result"`
	Error         auditHookError  `json:"error"`
	DurationMs    int64           `json:"durationMs"`
	Workspace     struct {
		Path string `json:"path"`
		CWD  string `json:"cwd"`
	} `json:"workspace"`
	CreatedAt auditFlexibleTime `json:"createdAt"`
}

func (p cursorHookPayload) auditHookPayload() auditHookPayload {
	response := p.Response
	if len(response) == 0 {
		response = p.Result
	}
	return auditHookPayload{
		EventID:         p.EventID,
		CreatedAt:       p.CreatedAt.Time,
		ClientVersion:   p.ClientVersion,
		ToolName:        p.ToolName,
		ToolType:        firstNonEmpty(p.ToolType, "tool"),
		Outcome:         auditOutcome(p.EventName, p.Error.String(), ""),
		DurationMs:      p.DurationMs,
		SessionID:       p.SessionID,
		ConversationID:  p.Conversation,
		CWD:             p.Workspace.CWD,
		Workspace:       p.Workspace.Path,
		SourceHookEvent: p.EventName,
		ClientEventID:   p.EventID,
		Request:         p.Args,
		Response:        response,
		Error:           p.Error.String(),
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
