package client

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
)

func TestInsertMCPAuditLogsAllowsMultipleMCPRowsWithLocalAgentIndexes(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()
	now := time.Now().UTC()

	logs := []types.MCPAuditLog{
		{
			CreatedAt: now,
			UserID:    "user-1",
			ClientIP:  "127.0.0.1",
			MCPFields: &types.MCPAuditLogFields{
				MCPID:          "mcp-1",
				CallType:       "tools/call",
				CallIdentifier: "tool-1",
				RequestBody:    json.RawMessage(`{"name":"tool-1"}`),
			},
		},
		{
			CreatedAt: now.Add(time.Second),
			UserID:    "user-2",
			ClientIP:  "127.0.0.2",
			MCPFields: &types.MCPAuditLogFields{
				MCPID:          "mcp-2",
				CallType:       "tools/call",
				CallIdentifier: "tool-2",
				RequestBody:    json.RawMessage(`{"name":"tool-2"}`),
			},
		},
	}

	if err := c.insertMCPAuditLogs(ctx, logs); err != nil {
		t.Fatalf("insert MCP audit logs: %v", err)
	}

	if got := countAuditLogs(t, c); got != 2 {
		t.Fatalf("expected 2 audit logs, got %d", got)
	}
}

func TestInsertMCPAuditLogsMergesResponseOnlyRowWithGroupedFields(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	now := time.Now().UTC()

	request := types.MCPAuditLog{
		CreatedAt: now,
		UserID:    "user-1",
		MCPFields: &types.MCPAuditLogFields{
			MCPID:       "mcp-1",
			RequestID:   "request-1",
			SessionID:   "session-1",
			RequestBody: json.RawMessage(`{"name":"tool"}`),
		},
	}
	response := types.MCPAuditLog{
		CreatedAt: now.Add(250 * time.Millisecond),
		UserID:    "user-1",
		MCPFields: &types.MCPAuditLogFields{
			MCPID:            "mcp-1",
			RequestID:        "request-1",
			SessionID:        "session-1",
			ResponseReceived: true,
			ResponseBody:     json.RawMessage(`{"ok":true}`),
			ResponseStatus:   200,
		},
	}

	if err := c.insertMCPAuditLogs(ctx, []types.MCPAuditLog{request}); err != nil {
		t.Fatalf("insert request audit log: %v", err)
	}
	if err := c.insertMCPAuditLogs(ctx, []types.MCPAuditLog{response}); err != nil {
		t.Fatalf("insert response audit log: %v", err)
	}

	var got types.MCPAuditLog
	if err := c.db.WithContext(ctx).First(&got).Error; err != nil {
		t.Fatalf("load merged audit log: %v", err)
	}
	if got.MCP().ResponseReceived != true {
		t.Fatal("expected response_received to be true")
	}
	if string(got.MCP().ResponseBody) != `{"ok":true}` {
		t.Fatalf("expected response body to be merged, got %s", got.MCP().ResponseBody)
	}
	if got.MCP().ProcessingTimeMs != 250 {
		t.Fatalf("expected processing time 250ms, got %d", got.MCP().ProcessingTimeMs)
	}
}
