package client

import (
	"strings"
	"testing"
	"time"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
)

func TestGenerateMockData(t *testing.T) {
	c := newTestClient(t)
	now := time.Date(2026, time.September, 14, 12, 0, 0, 0, time.UTC)

	summary, err := c.generateMockData(t.Context(), now, "demo-test-run")
	if err != nil {
		t.Fatal(err)
	}
	if summary.RunID != "demo-test-run" || summary.WindowStart != now.Add(-30*24*time.Hour) || summary.WindowEnd != now {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if summary.Users != mockDataUserCount || summary.Devices != mockDataDeviceCount ||
		summary.DeviceScans != mockDataDeviceScanCount || summary.LocalAgentAuditLogs != mockDataLocalAuditCount ||
		summary.MCPAuditLogs != mockDataMCPAuditCount || summary.LLMAuditLogs != mockDataLLMAuditCount {
		t.Fatalf("unexpected counts in summary: %+v", summary)
	}
	if summary.EnforcementDecisions != mockDataEnforcementCount {
		t.Fatalf("unexpected enforcement count in summary: %+v", summary)
	}

	users, err := c.Users(t.Context(), types.UserQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != mockDataUserCount {
		t.Fatalf("got %d users, want %d", len(users), mockDataUserCount)
	}
	for _, user := range users {
		if !strings.HasPrefix(user.Username, "demo-test-run-user-") || !strings.HasPrefix(user.DisplayName, "Demo ") {
			t.Fatalf("unexpected mock user: %+v", user)
		}
	}
	licensedUsers, err := c.UserCount(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if licensedUsers != mockDataUserCount {
		t.Fatalf("got %d users counted toward the license limit, want %d", licensedUsers, mockDataUserCount)
	}
	enrolledDevices, err := c.DeviceCount(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if enrolledDevices != 0 {
		t.Fatalf("mock devices consumed %d licensed seats", enrolledDevices)
	}

	assertMockDataRowCount(t, c, &types.DeviceScan{}, "", nil, mockDataDeviceScanCount)
	assertMockDataRowCount(t, c, &types.MCPAuditLog{}, "source_type = ?", apitypes.AuditLogSourceTypeLocalAgentToolCall, mockDataLocalAuditCount)
	assertMockDataRowCount(t, c, &types.MCPAuditLog{}, "source_type = ?", apitypes.AuditLogSourceTypeMCP, mockDataMCPAuditCount)
	assertMockDataRowCount(t, c, &types.LLMAuditLog{}, "", nil, mockDataLLMAuditCount)
	assertMockDataRowCount(t, c, &types.EnforcementDecisionLog{}, "", nil, mockDataEnforcementCount)
	assertMockDataRowCount(t, c, &types.EnforcementDecisionLog{}, "decision = ?", apitypes.EnforcementDecisionDeny, mockDataEnforcementCount/4)

	var scan types.DeviceScan
	if err := c.db.WithContext(t.Context()).Preload("Clients").Preload("MCPServers").Preload("Skills").Preload("Files").First(&scan).Error; err != nil {
		t.Fatal(err)
	}
	if len(scan.Clients) == 0 || len(scan.MCPServers) == 0 || len(scan.Skills) == 0 || len(scan.Files) == 0 {
		t.Fatalf("mock scan is missing associations: %+v", scan)
	}
}

func TestGenerateMockDataAppendsAndRollsBackCollisions(t *testing.T) {
	c := newTestClient(t)
	now := time.Date(2026, time.September, 14, 12, 0, 0, 0, time.UTC)

	if _, err := c.generateMockData(t.Context(), now, "demo-first"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.generateMockData(t.Context(), now, "demo-second"); err != nil {
		t.Fatal(err)
	}
	assertMockDataRowCount(t, c, &types.User{}, "", nil, 2*mockDataUserCount)
	assertMockDataRowCount(t, c, &types.DeviceScan{}, "", nil, 2*mockDataDeviceScanCount)
	assertMockDataRowCount(t, c, &types.MCPAuditLog{}, "", nil, 2*(mockDataLocalAuditCount+mockDataMCPAuditCount))
	assertMockDataRowCount(t, c, &types.LLMAuditLog{}, "", nil, 2*mockDataLLMAuditCount)
	assertMockDataRowCount(t, c, &types.EnforcementDecisionLog{}, "", nil, 2*mockDataEnforcementCount)

	if _, err := c.generateMockData(t.Context(), now, "demo-second"); err == nil {
		t.Fatal("expected duplicate run identifiers to fail")
	}
	assertMockDataRowCount(t, c, &types.User{}, "", nil, 2*mockDataUserCount)
	assertMockDataRowCount(t, c, &types.DeviceScan{}, "", nil, 2*mockDataDeviceScanCount)
	assertMockDataRowCount(t, c, &types.MCPAuditLog{}, "", nil, 2*(mockDataLocalAuditCount+mockDataMCPAuditCount))
	assertMockDataRowCount(t, c, &types.LLMAuditLog{}, "", nil, 2*mockDataLLMAuditCount)
	assertMockDataRowCount(t, c, &types.EnforcementDecisionLog{}, "", nil, 2*mockDataEnforcementCount)
}

func assertMockDataRowCount(t *testing.T, c *Client, model any, query string, value any, want int) {
	t.Helper()
	db := c.db.WithContext(t.Context()).Model(model)
	if query != "" {
		db = db.Where(query, value)
	}
	var got int64
	if err := db.Count(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got != int64(want) {
		t.Fatalf("got %d rows for %T, want %d", got, model, want)
	}
}
