package types

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

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

	if log.Local == nil || log.Local.EventID != "evt-1" {
		t.Errorf("EventID not preserved: %v", log.LocalFields().EventID)
	}
	if log.Local == nil || log.Local.DeviceID != "dev-1" {
		t.Errorf("DeviceID not preserved: %v", log.LocalFields().DeviceID)
	}
	if log.CallIdentifier != "Bash" || log.CallType != "command" {
		t.Errorf("tool mapping wrong: callIdentifier=%q callType=%q", log.CallIdentifier, log.CallType)
	}
	if log.Local == nil || log.MCP != nil {
		t.Errorf("local events must set only the Local source fields, got local=%v mcp=%v", log.Local, log.MCP)
	}
	if !log.ResponseReceived {
		t.Errorf("generic events must be marked ResponseReceived to bypass the merge path")
	}
	if log.UserID != "" || log.ReceivedAt != nil {
		t.Errorf("UserID and ReceivedAt are server-assigned and must not be copied from the event, got userID=%q receivedAt=%v", log.UserID, log.ReceivedAt)
	}

	// The server assigns the user from the authenticated request.
	log.UserID = event.UserID

	got := ConvertAuditEvent(log)
	if !reflect.DeepEqual(event, got) {
		t.Errorf("round trip mismatch:\n  want: %+v\n  got:  %+v", event, got)
	}
}

func TestConvertMCPAuditLogSourceVariants(t *testing.T) {
	mcpLog := ConvertMCPAuditLog(MCPAuditLog{
		CreatedAt:  time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC),
		SourceType: types2.AuditLogSourceTypeMCP,
		CallType:   "tools/call",
		MCP: &MCPAuditLogFields{
			MCPID:                "mcp-1",
			MCPServerDisplayName: "server",
			ResponseStatus:       200,
		},
	})

	if mcpLog.MCP == nil || mcpLog.Local != nil {
		t.Fatalf("MCP conversion source fields = mcp:%v local:%v, want only MCP", mcpLog.MCP, mcpLog.Local)
	}
	if mcpLog.MCP.MCPID != "mcp-1" || mcpLog.MCP.ResponseStatus != 200 {
		t.Fatalf("MCP fields not converted: %+v", mcpLog.MCP)
	}

	localLog := ConvertMCPAuditLog(MCPAuditLog{
		CreatedAt:  time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC),
		SourceType: types2.AuditLogSourceTypeLocalAgent,
		CallType:   "command",
		Local: &LocalAuditLog{
			ErrorDetail: "full error",
			RawEvent:    json.RawMessage(`{"hook":"post"}`),
		},
	})

	if localLog.Local == nil || localLog.MCP != nil {
		t.Fatalf("local conversion source fields = mcp:%v local:%v, want only Local", localLog.MCP, localLog.Local)
	}
	if localLog.Local.ErrorDetail != "full error" || string(localLog.Local.RawEvent) != `{"hook":"post"}` {
		t.Fatalf("local fields not converted: %+v", localLog.Local)
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
	if log.Local == nil || log.Local.ErrorDetail != fullError {
		t.Errorf("ErrorDetail must hold the full error text")
	}

	if got := ConvertAuditEvent(log); got.Error != fullError {
		t.Errorf("converted event must surface the full error, got %d bytes", len(got.Error))
	}
}

func TestAuditEventErrorSummaryTruncationPreservesUTF8(t *testing.T) {
	fullError := strings.Repeat("x", maxErrorSummaryBytes-1) + "€" + "tail"

	event := types2.AuditEvent{
		EventID:    "evt-3",
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

	if !utf8.ValidString(log.Error) {
		t.Fatalf("Error summary must remain valid UTF-8")
	}
	if len(log.Error) != maxErrorSummaryBytes-1 {
		t.Errorf("Error summary length = %d, want %d", len(log.Error), maxErrorSummaryBytes-1)
	}
	if log.Local == nil || log.Local.ErrorDetail != fullError {
		t.Errorf("ErrorDetail must hold the full error text")
	}

	if got := ConvertAuditEvent(log); got.Error != fullError {
		t.Errorf("converted event must surface the full error, got %d bytes", len(got.Error))
	}
}
