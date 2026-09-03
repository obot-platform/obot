package client

import (
	"fmt"
	"testing"
	"time"

	clienttypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
)

func TestProductTelemetryDailyCounts(t *testing.T) {
	c := newTestClient(t)
	start := time.Date(2026, time.September, 2, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)

	users := []types.User{
		{Username: "active", HashedUsername: "active"},
		{Username: "internal", HashedUsername: "internal", Internal: true},
		{Username: "outside", HashedUsername: "outside"},
	}
	if err := c.db.WithContext(t.Context()).Create(&users).Error; err != nil {
		t.Fatalf("create users: %v", err)
	}
	activities := []types.APIActivity{
		{UserID: fmt.Sprint(users[0].ID), Date: start},
		{UserID: fmt.Sprint(users[1].ID), Date: start},
		{UserID: fmt.Sprint(users[2].ID), Date: end},
		{UserID: "anonymous", Date: start},
	}
	if err := c.db.WithContext(t.Context()).Create(&activities).Error; err != nil {
		t.Fatalf("create API activities: %v", err)
	}

	mcpLogs := []types.MCPAuditLog{
		{CreatedAt: start, SourceType: clienttypes.AuditLogSourceTypeMCP, MCPFields: &types.MCPAuditLogFields{CallType: "tools/call"}},
		{CreatedAt: start, SourceType: clienttypes.AuditLogSourceTypeMCP, MCPFields: &types.MCPAuditLogFields{CallType: "resources/read"}},
		{CreatedAt: end, SourceType: clienttypes.AuditLogSourceTypeMCP, MCPFields: &types.MCPAuditLogFields{CallType: "tools/call"}},
		{CreatedAt: start, SourceType: clienttypes.AuditLogSourceTypeLocalAgentToolCall},
	}
	if err := c.db.WithContext(t.Context()).Create(&mcpLogs).Error; err != nil {
		t.Fatalf("create MCP audit logs: %v", err)
	}

	llmLogs := []types.LLMAuditLog{{ID: "inside", CreatedAt: start}, {ID: "outside", CreatedAt: end}}
	if err := c.db.WithContext(t.Context()).Create(&llmLogs).Error; err != nil {
		t.Fatalf("create LLM audit logs: %v", err)
	}
	deviceScans := []types.DeviceScan{{CreatedAt: start, DeviceID: "inside"}, {CreatedAt: end, DeviceID: "outside"}}
	if err := c.db.WithContext(t.Context()).Create(&deviceScans).Error; err != nil {
		t.Fatalf("create device scans: %v", err)
	}
	decisions := []types.EnforcementDecisionLog{{CreatedAt: start}, {CreatedAt: end}}
	if err := c.db.WithContext(t.Context()).Create(&decisions).Error; err != nil {
		t.Fatalf("create enforcement decisions: %v", err)
	}

	tests := []struct {
		name  string
		count func() (int64, error)
		want  int64
	}{
		{name: "active users", count: func() (int64, error) { return c.ActiveUserCountByDate(t.Context(), start, end) }, want: 1},
		{name: "MCP tool calls", count: func() (int64, error) { return c.MCPToolCallCount(t.Context(), start, end) }, want: 1},
		{name: "LLM audit logs", count: func() (int64, error) { return c.LLMAuditLogCount(t.Context(), start, end) }, want: 1},
		{name: "device scans", count: func() (int64, error) { return c.DeviceScanCount(t.Context(), start, end) }, want: 1},
		{name: "enforcement decisions", count: func() (int64, error) { return c.EnforcementDecisionCount(t.Context(), start, end) }, want: 1},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := testCase.count()
			if err != nil || got != testCase.want {
				t.Fatalf("count = %d, %v, want %d", got, err, testCase.want)
			}
		})
	}
}
