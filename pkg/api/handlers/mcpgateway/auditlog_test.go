package mcpgateway

import (
	"encoding/json"
	"testing"
)

func TestAuditLogInputAcceptsFlatMCPFields(t *testing.T) {
	var logs []auditLogInput
	err := json.Unmarshal([]byte(`[{
		"mcpID": "mcp-1",
		"requestID": "req-1",
		"callType": "tools/call",
		"requestBody": {"jsonrpc":"2.0"},
		"metadata": {"mcpServerDisplayName":"Search"}
	}]`), &logs)
	if err != nil {
		t.Fatalf("failed to unmarshal audit log input: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("got %d logs, want 1", len(logs))
	}

	log := logs[0]
	if log.MCPID != "mcp-1" {
		t.Fatalf("MCPID = %q, want mcp-1", log.MCPID)
	}
	if log.RequestID != "req-1" {
		t.Fatalf("RequestID = %q, want req-1", log.RequestID)
	}
	if log.CallType != "tools/call" {
		t.Fatalf("CallType = %q, want tools/call", log.CallType)
	}
	if len(log.RequestBody) == 0 {
		t.Fatalf("RequestBody was not populated")
	}
	if log.hasNestedSourceFields() {
		t.Fatalf("flat input must not populate nested source fields")
	}
}

func TestAuditLogInputDetectsNestedSourceFields(t *testing.T) {
	var logs []auditLogInput
	err := json.Unmarshal([]byte(`[{
		"mcp": {"mcpID": "mcp-1"},
		"callType": "tools/call"
	}]`), &logs)
	if err != nil {
		t.Fatalf("failed to unmarshal audit log input: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("got %d logs, want 1", len(logs))
	}
	if !logs[0].hasNestedSourceFields() {
		t.Fatalf("nested source fields must be detected")
	}
}
