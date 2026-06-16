package client

import (
	"encoding/json"
	"testing"
	"time"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	"github.com/obot-platform/obot/pkg/gateway/types"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
)

func newTestClientWithDB(t *testing.T) (*Client, *gatewaydb.DB) {
	t.Helper()

	services, err := sservices.New(sservices.Config{
		DSN: "sqlite://:memory:",
	})
	if err != nil {
		t.Fatalf("failed to create storage services: %v", err)
	}

	db, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	if err != nil {
		t.Fatalf("failed to create gateway db: %v", err)
	}
	if err := db.AutoMigrate(); err != nil {
		t.Fatalf("failed to auto-migrate: %v", err)
	}

	return &Client{db: db}, db
}

func insertAuditLogRow(t *testing.T, c *Client, row types.MCPAuditLog) types.MCPAuditLog {
	t.Helper()
	if err := c.db.WithContext(t.Context()).Create(&row).Error; err != nil {
		t.Fatalf("failed to insert audit log: %v", err)
	}
	return row
}

// nullGenericColumns simulates a row written before the generic audit-event
// columns existed by forcing them to SQL NULL.
func nullGenericColumns(t *testing.T, c *Client, id uint) {
	t.Helper()
	err := c.db.WithContext(t.Context()).Model(&types.MCPAuditLog{}).Where("id = ?", id).
		Updates(map[string]any{
			"event_id":    nil,
			"source_type": nil,
			"event_type":  nil,
			"outcome":     nil,
			"device_id":   nil,
			"received_at": nil,
		}).Error
	if err != nil {
		t.Fatalf("failed to null generic columns: %v", err)
	}
}

// runAuditLogBackfill re-runs the generic_audit_log_fields_backfill migration
// against rows seeded after the initial AutoMigrate.
func runAuditLogBackfill(t *testing.T, c *Client, db *gatewaydb.DB) {
	t.Helper()
	err := c.db.WithContext(t.Context()).
		Where("name = ?", "generic_audit_log_fields_backfill").
		Delete(&types.Migration{}).Error
	if err != nil {
		t.Fatalf("failed to reset backfill migration record: %v", err)
	}
	if err := db.AutoMigrate(); err != nil {
		t.Fatalf("failed to re-run migrations: %v", err)
	}
}

func TestInsertAuditEventsAcceptsDuplicatesAndRejectsInvalid(t *testing.T) {
	c, _ := newTestClientWithDB(t)

	event := apitypes.AuditEvent{
		EventID:    "evt-local-1",
		SourceType: apitypes.AuditLogSourceTypeLocalAgent,
		EventType:  apitypes.AuditLogEventTypeToolCall,
		CreatedAt:  apitypes.Time{Time: time.Now().UTC()},
		DeviceID:   "dev-1",
		Client:     apitypes.ClientInfo{Name: "codex", Version: "unknown"},
		Tool:       apitypes.ToolInfo{Name: "shell", Type: "tool"},
		Outcome:    apitypes.AuditLogOutcomeSuccess,
		DurationMs: 42,
		Request:    json.RawMessage(`{"cmd":"go test"}`),
		RawEvent:   json.RawMessage(`{"tool_name":"shell"}`),
	}
	invalid := event
	invalid.EventID = "evt-invalid"
	invalid.SourceType = "unsupported"

	statuses, err := c.InsertAuditEvents(t.Context(), "user-1", "203.0.113.10", []apitypes.AuditEvent{event, event, invalid})
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 3 {
		t.Fatalf("statuses len = %d, want 3", len(statuses))
	}
	if statuses[0].Status != apitypes.AuditEventSubmitStatusAccepted {
		t.Fatalf("first status = %#v, want accepted", statuses[0])
	}
	if statuses[1].Status != apitypes.AuditEventSubmitStatusDuplicate {
		t.Fatalf("second status = %#v, want duplicate", statuses[1])
	}
	if statuses[2].Status != apitypes.AuditEventSubmitStatusError || statuses[2].Error == "" {
		t.Fatalf("third status = %#v, want error with message", statuses[2])
	}

	logs, total, err := c.GetMCPAuditLogs(t.Context(), MCPAuditLogOptions{
		SourceType: []string{apitypes.AuditLogSourceTypeLocalAgent},
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(logs) != 1 {
		t.Fatalf("stored local logs total/len = %d/%d, want 1/1", total, len(logs))
	}
	row := logs[0]
	if row.UserID != "user-1" || row.EventID == nil || *row.EventID != event.EventID || row.ReceivedAt == nil {
		t.Fatalf("stored row missing server-assigned fields: %+v", row)
	}
	if row.ClientIP != "203.0.113.10" {
		t.Fatalf("stored row ClientIP = %q, want %q", row.ClientIP, "203.0.113.10")
	}
	if row.SourceType != apitypes.AuditLogSourceTypeLocalAgent ||
		row.EventType != apitypes.AuditLogEventTypeToolCall ||
		row.DeviceID != "dev-1" ||
		row.ClientName != "codex" ||
		row.CallIdentifier != "shell" ||
		row.ProcessingTimeMs != 42 {
		t.Fatalf("stored row mismatch: %+v", row)
	}
}

func seedMixedAuditLogs(t *testing.T, c *Client, db *gatewaydb.DB) {
	t.Helper()
	now := time.Now().UTC()

	// Legacy MCP rows: generic columns NULL until the backfill runs.
	legacySuccess := insertAuditLogRow(t, c, types.MCPAuditLog{
		CreatedAt:      now,
		UserID:         "u1",
		MCPID:          "mcp-1",
		CallType:       "tools/call",
		CallIdentifier: "search",
		ResponseStatus: 200,
	})
	nullGenericColumns(t, c, legacySuccess.ID)

	legacyError := insertAuditLogRow(t, c, types.MCPAuditLog{
		CreatedAt:      now,
		UserID:         "u1",
		MCPID:          "mcp-1",
		CallType:       "resources/read",
		CallIdentifier: "file://x",
		ResponseStatus: 500,
		Error:          "boom",
	})
	nullGenericColumns(t, c, legacyError.ID)

	legacyInit := insertAuditLogRow(t, c, types.MCPAuditLog{
		CreatedAt:      now,
		UserID:         "u2",
		MCPID:          "mcp-2",
		CallType:       "initialize",
		ResponseStatus: 200,
	})
	nullGenericColumns(t, c, legacyInit.ID)

	// New-style local agent row.
	insertAuditLogRow(t, c, types.MCPAuditLog{
		CreatedAt:        now,
		EventID:          new("evt-local-1"),
		SourceType:       types.AuditLogSourceTypeLocalAgent,
		EventType:        types.AuditLogEventTypeToolCall,
		Outcome:          types.AuditLogOutcomeSuccess,
		DeviceID:         "dev-1",
		UserID:           "u3",
		ClientName:       "claude-code",
		CallType:         "command",
		CallIdentifier:   "Bash",
		ResponseReceived: true,
	})

	// New-style MCP row with generic columns populated at write time.
	insertAuditLogRow(t, c, types.MCPAuditLog{
		CreatedAt:        now,
		EventID:          new("evt-mcp-1"),
		SourceType:       types.AuditLogSourceTypeMCP,
		EventType:        types.AuditLogEventTypeToolCall,
		Outcome:          types.AuditLogOutcomeError,
		UserID:           "u1",
		MCPID:            "mcp-1",
		CallType:         "tools/call",
		CallIdentifier:   "search",
		ResponseStatus:   400,
		Error:            "bad request",
		ResponseReceived: true,
	})

	runAuditLogBackfill(t, c, db)
}

func TestGenericAuditLogFieldsBackfillMigration(t *testing.T) {
	c, db := newTestClientWithDB(t)
	ctx := t.Context()

	tests := []struct {
		name          string
		row           types.MCPAuditLog
		wantEventType string
		wantOutcome   string
	}{
		{
			name:          "successful tool call",
			row:           types.MCPAuditLog{CallType: "tools/call", ResponseStatus: 200},
			wantEventType: types.AuditLogEventTypeToolCall,
			wantOutcome:   types.AuditLogOutcomeSuccess,
		},
		{
			name:          "failed resource read",
			row:           types.MCPAuditLog{CallType: "resources/read", ResponseStatus: 500},
			wantEventType: types.AuditLogEventTypeResourceRead,
			wantOutcome:   types.AuditLogOutcomeError,
		},
		{
			name:          "prompt get with error message",
			row:           types.MCPAuditLog{CallType: "prompts/get", ResponseStatus: 200, Error: "boom"},
			wantEventType: types.AuditLogEventTypePromptGet,
			wantOutcome:   types.AuditLogOutcomeError,
		},
		{
			name:          "other call type",
			row:           types.MCPAuditLog{CallType: "initialize", ResponseStatus: 200},
			wantEventType: types.AuditLogEventTypeMCPRequest,
			wantOutcome:   types.AuditLogOutcomeSuccess,
		},
	}

	ids := make([]uint, len(tests))
	for i, tt := range tests {
		tt.row.CreatedAt = time.Now().UTC()
		row := insertAuditLogRow(t, c, tt.row)
		nullGenericColumns(t, c, row.ID)
		ids[i] = row.ID
	}

	// A row that already has values must not be rewritten.
	preFilled := insertAuditLogRow(t, c, types.MCPAuditLog{
		CreatedAt:  time.Now().UTC(),
		SourceType: types.AuditLogSourceTypeLocalAgent,
		EventType:  types.AuditLogEventTypeToolCall,
		Outcome:    types.AuditLogOutcomeError,
		CallType:   "command",
	})

	runAuditLogBackfill(t, c, db)

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var row types.MCPAuditLog
			if err := c.db.WithContext(ctx).First(&row, ids[i]).Error; err != nil {
				t.Fatalf("failed to fetch row: %v", err)
			}
			if row.SourceType != types.AuditLogSourceTypeMCP {
				t.Errorf("SourceType = %q, want %q", row.SourceType, types.AuditLogSourceTypeMCP)
			}
			if row.EventType != tt.wantEventType {
				t.Errorf("EventType = %q, want %q", row.EventType, tt.wantEventType)
			}
			if row.Outcome != tt.wantOutcome {
				t.Errorf("Outcome = %q, want %q", row.Outcome, tt.wantOutcome)
			}
			if row.EventID != nil {
				t.Errorf("EventID must stay NULL on historical rows, got %q", *row.EventID)
			}
			if row.ReceivedAt != nil {
				t.Errorf("ReceivedAt must stay NULL on historical rows, got %v", *row.ReceivedAt)
			}
		})
	}

	var row types.MCPAuditLog
	if err := c.db.WithContext(ctx).First(&row, preFilled.ID).Error; err != nil {
		t.Fatalf("failed to fetch pre-filled row: %v", err)
	}
	if row.SourceType != types.AuditLogSourceTypeLocalAgent ||
		row.EventType != types.AuditLogEventTypeToolCall ||
		row.Outcome != types.AuditLogOutcomeError {
		t.Errorf("backfill must not rewrite populated rows, got %+v", row)
	}
}

func TestGenericAuditFilters(t *testing.T) {
	c, db := newTestClientWithDB(t)
	seedMixedAuditLogs(t, c, db)
	ctx := t.Context()

	tests := []struct {
		name string
		opts MCPAuditLogOptions
		want int64
	}{
		{name: "no filters", opts: MCPAuditLogOptions{}, want: 5},
		{name: "mcp source includes backfilled rows", opts: MCPAuditLogOptions{SourceType: []string{types.AuditLogSourceTypeMCP}}, want: 4},
		{name: "local agent source", opts: MCPAuditLogOptions{SourceType: []string{types.AuditLogSourceTypeLocalAgent}}, want: 1},
		{name: "tool_call matches backfilled and new rows", opts: MCPAuditLogOptions{EventType: []string{types.AuditLogEventTypeToolCall}}, want: 3},
		{name: "resource_read matches backfilled call type", opts: MCPAuditLogOptions{EventType: []string{types.AuditLogEventTypeResourceRead}}, want: 1},
		{name: "mcp_request matches backfilled other call types", opts: MCPAuditLogOptions{EventType: []string{types.AuditLogEventTypeMCPRequest}}, want: 1},
		{name: "success outcome", opts: MCPAuditLogOptions{Outcome: []string{types.AuditLogOutcomeSuccess}}, want: 3},
		{name: "error outcome", opts: MCPAuditLogOptions{Outcome: []string{types.AuditLogOutcomeError}}, want: 2},
		{name: "device id", opts: MCPAuditLogOptions{DeviceID: []string{"dev-1"}}, want: 1},
		{name: "combined source and outcome", opts: MCPAuditLogOptions{SourceType: []string{types.AuditLogSourceTypeMCP}, Outcome: []string{types.AuditLogOutcomeError}}, want: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs, total, err := c.GetMCPAuditLogs(ctx, tt.opts)
			if err != nil {
				t.Fatalf("GetMCPAuditLogs() error: %v", err)
			}
			if total != tt.want || int64(len(logs)) != tt.want {
				t.Errorf("got %d rows (total %d), want %d", len(logs), total, tt.want)
			}
		})
	}
}

func TestUsageStatsExcludeNonMCPRows(t *testing.T) {
	c, db := newTestClientWithDB(t)
	seedMixedAuditLogs(t, c, db)
	ctx := t.Context()

	stats, err := c.GetMCPUsageStats(ctx, MCPUsageStatsOptions{
		StartTime: time.Now().UTC().Add(-time.Hour),
		EndTime:   time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("GetMCPUsageStats() error: %v", err)
	}

	// 4 MCP rows (3 backfilled + 1 new-style); the local agent row must not count.
	if stats.TotalCalls != 4 {
		t.Errorf("TotalCalls = %d, want 4", stats.TotalCalls)
	}
	for _, item := range stats.Items {
		if item.MCPID == "" {
			t.Errorf("usage stats must not include non-MCP rows, got item %+v", item)
		}
	}
}

func TestEventIDUniqueness(t *testing.T) {
	c, _ := newTestClientWithDB(t)

	insertAuditLogRow(t, c, types.MCPAuditLog{
		CreatedAt: time.Now().UTC(),
		EventID:   new("evt-dup"),
	})

	err := c.db.WithContext(t.Context()).Create(&types.MCPAuditLog{
		CreatedAt: time.Now().UTC(),
		EventID:   new("evt-dup"),
	}).Error
	if err == nil {
		t.Fatalf("expected duplicate event_id insert to fail")
	}

	// Multiple NULL event IDs (historical rows) must coexist.
	first := insertAuditLogRow(t, c, types.MCPAuditLog{CreatedAt: time.Now().UTC()})
	nullGenericColumns(t, c, first.ID)
	second := insertAuditLogRow(t, c, types.MCPAuditLog{CreatedAt: time.Now().UTC()})
	nullGenericColumns(t, c, second.ID)
}
