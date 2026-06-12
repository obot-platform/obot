package types

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
)

func TestAuditEventRoundTrip(t *testing.T) {
	event := types2.AuditEvent{
		EventID:    "evt-1",
		SourceType: types2.AuditLogSourceTypeLocalAgent,
		EventType:  types2.AuditLogEventTypeToolCall,
		CreatedAt:  *types2.NewTime(time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)),
		UserID:     "u1",
		DeviceID:   "dev-1",
		Client:     types2.ClientInfo{Name: "claude-code", Version: "1.2.3"},
		Tool:       types2.ToolInfo{Name: "Bash", Type: "command"},
		Outcome:    types2.AuditLogOutcomeSuccess,
		DurationMs: 42,
		SessionID:  "sess-1",
		Request:    json.RawMessage(`{"command":"ls"}`),
		Response:   json.RawMessage(`{"output":"file.txt"}`),
		Error:      "",
		RawEvent:   json.RawMessage(`{"hook_event_name":"PostToolUse"}`),
		Context: &types2.AuditLogContext{
			ConversationID: "conv-1",
			CWD:            "/home/user/project",
			Workspace:      "project",
			GitRemote:      "git@example.com:org/project.git",
			GitBranch:      "main",
			Hostname:       "laptop",
			OS:             "linux",
			Arch:           "arm64",
			Username:       "user",
		},
		PayloadMeta: map[string]types2.PayloadFieldMeta{
			"response": {Truncated: true, OriginalBytes: 2048, StoredBytes: 1024},
		},
	}

	log, err := MCPAuditLogFromAuditEvent(event)
	if err != nil {
		t.Fatalf("MCPAuditLogFromAuditEvent() error: %v", err)
	}

	if log.EventID == nil || *log.EventID != "evt-1" {
		t.Errorf("EventID not preserved: %v", log.EventID)
	}
	if log.CallIdentifier != "Bash" || log.CallType != "command" {
		t.Errorf("tool mapping wrong: callIdentifier=%q callType=%q", log.CallIdentifier, log.CallType)
	}
	if log.MCPID != "" || log.MCPServerDisplayName != "" {
		t.Errorf("MCP-specific fields must stay empty for local events")
	}
	if !log.ResponseReceived {
		t.Errorf("generic events must be marked ResponseReceived to bypass the merge path")
	}

	got := ConvertAuditEvent(log)
	if !reflect.DeepEqual(event, got) {
		t.Errorf("round trip mismatch:\n  want: %+v\n  got:  %+v", event, got)
	}
}

func TestAuditEventErrorSummaryTruncation(t *testing.T) {
	fullError := strings.Repeat("x", maxErrorSummaryBytes+100)

	event := types2.AuditEvent{
		EventID:    "evt-2",
		SourceType: types2.AuditLogSourceTypeLocalAgent,
		EventType:  types2.AuditLogEventTypeToolCall,
		CreatedAt:  *types2.NewTime(time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)),
		Outcome:    types2.AuditLogOutcomeError,
		Error:      fullError,
	}

	log, err := MCPAuditLogFromAuditEvent(event)
	if err != nil {
		t.Fatalf("MCPAuditLogFromAuditEvent() error: %v", err)
	}

	if len(log.Error) != maxErrorSummaryBytes {
		t.Errorf("Error summary length = %d, want %d", len(log.Error), maxErrorSummaryBytes)
	}
	if log.ErrorDetail != fullError {
		t.Errorf("ErrorDetail must hold the full error text")
	}

	if got := ConvertAuditEvent(log); got.Error != fullError {
		t.Errorf("converted event must surface the full error, got %d bytes", len(got.Error))
	}
}
