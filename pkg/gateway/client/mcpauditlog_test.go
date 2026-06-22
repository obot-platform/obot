package client

import (
	"testing"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
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
			"source_type": nil,
			"event_type":  nil,
			"outcome":     nil,
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

func seedMixedAuditLogs(t *testing.T, c *Client, db *gatewaydb.DB) {
	t.Helper()
	now := time.Now().UTC()

	// Legacy MCP rows: generic columns NULL until the backfill runs.
	legacySuccess := insertAuditLogRow(t, c, types.MCPAuditLog{
		CreatedAt:      now,
		UserID:         "u1",
		CallType:       "tools/call",
		CallIdentifier: "search",

		MCP: &types.MCPAuditLogFields{
			MCPID:          "mcp-1",
			ResponseStatus: 200,
		},
	})
	nullGenericColumns(t, c, legacySuccess.ID)

	legacyError := insertAuditLogRow(t, c, types.MCPAuditLog{
		CreatedAt:      now,
		UserID:         "u1",
		CallType:       "resources/read",
		CallIdentifier: "file://x",
		Error:          "boom",

		MCP: &types.MCPAuditLogFields{
			MCPID:          "mcp-1",
			ResponseStatus: 500,
		},
	})
	nullGenericColumns(t, c, legacyError.ID)

	legacyInit := insertAuditLogRow(t, c, types.MCPAuditLog{
		CreatedAt: now,
		UserID:    "u2",
		CallType:  "initialize",

		MCP: &types.MCPAuditLogFields{
			MCPID:          "mcp-2",
			ResponseStatus: 200,
		},
	})
	nullGenericColumns(t, c, legacyInit.ID)

	// New-style local agent row.
	insertAuditLogRow(t, c, types.MCPAuditLog{
		CreatedAt:        now,
		SourceType:       types2.AuditLogSourceTypeLocalAgent,
		EventType:        types2.AuditLogEventTypeToolCall,
		Outcome:          types2.AuditLogOutcomeSuccess,
		UserID:           "u3",
		ClientName:       "claude-code",
		CallType:         "command",
		CallIdentifier:   "Bash",
		Local:            &types.LocalAuditLog{EventID: "evt-local-1", DeviceID: "dev-1"},
		ResponseReceived: true,
	})

	// New-style MCP row with generic columns populated at write time.
	insertAuditLogRow(t, c, types.MCPAuditLog{
		CreatedAt:      now,
		SourceType:     types2.AuditLogSourceTypeMCP,
		EventType:      types2.AuditLogEventTypeToolCall,
		Outcome:        types2.AuditLogOutcomeError,
		UserID:         "u1",
		CallType:       "tools/call",
		CallIdentifier: "search",
		Error:          "bad request",

		MCP: &types.MCPAuditLogFields{
			MCPID:          "mcp-1",
			ResponseStatus: 400,
		},
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
		wantEventType types2.AuditLogEventType
		wantOutcome   types2.AuditLogOutcome
	}{
		{
			name: "successful tool call",
			row: types.MCPAuditLog{
				CallType: "tools/call",
				MCP:      &types.MCPAuditLogFields{ResponseStatus: 200},
			},
			wantEventType: types2.AuditLogEventTypeToolCall,
			wantOutcome:   types2.AuditLogOutcomeSuccess,
		},
		{
			name: "failed resource read",
			row: types.MCPAuditLog{
				CallType: "resources/read",
				MCP:      &types.MCPAuditLogFields{ResponseStatus: 500},
			},
			wantEventType: types2.AuditLogEventTypeResourceRead,
			wantOutcome:   types2.AuditLogOutcomeError,
		},
		{
			name: "prompt get with error message",
			row: types.MCPAuditLog{
				CallType: "prompts/get", Error: "boom",
				MCP: &types.MCPAuditLogFields{ResponseStatus: 200},
			},
			wantEventType: types2.AuditLogEventTypePromptGet,
			wantOutcome:   types2.AuditLogOutcomeError,
		},
		{
			name: "other call type",
			row: types.MCPAuditLog{
				CallType: "initialize",
				MCP:      &types.MCPAuditLogFields{ResponseStatus: 200},
			},
			wantEventType: types2.AuditLogEventTypeMCPRequest,
			wantOutcome:   types2.AuditLogOutcomeSuccess,
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
		SourceType: types2.AuditLogSourceTypeLocalAgent,
		EventType:  types2.AuditLogEventTypeToolCall,
		Outcome:    types2.AuditLogOutcomeError,
		CallType:   "command",
	})

	runAuditLogBackfill(t, c, db)

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var row types.MCPAuditLog
			if err := c.db.WithContext(ctx).First(&row, ids[i]).Error; err != nil {
				t.Fatalf("failed to fetch row: %v", err)
			}
			if row.SourceType != types2.AuditLogSourceTypeMCP {
				t.Errorf("SourceType = %q, want %q", row.SourceType, types2.AuditLogSourceTypeMCP)
			}
			if row.EventType != tt.wantEventType {
				t.Errorf("EventType = %q, want %q", row.EventType, tt.wantEventType)
			}
			if row.Outcome != tt.wantOutcome {
				t.Errorf("Outcome = %q, want %q", row.Outcome, tt.wantOutcome)
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
	if row.SourceType != types2.AuditLogSourceTypeLocalAgent ||
		row.EventType != types2.AuditLogEventTypeToolCall ||
		row.Outcome != types2.AuditLogOutcomeError {
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
		{
			name: "no filters",
			opts: MCPAuditLogOptions{},
			want: 5,
		},
		{
			name: "mcp source includes backfilled rows",
			opts: MCPAuditLogOptions{
				SourceType: []string{string(types2.AuditLogSourceTypeMCP)},
			},
			want: 4,
		},
		{
			name: "local agent source",
			opts: MCPAuditLogOptions{
				SourceType: []string{string(types2.AuditLogSourceTypeLocalAgent)},
			},
			want: 1,
		},
		{
			name: "tool_call matches backfilled and new rows",
			opts: MCPAuditLogOptions{
				EventType: []string{string(types2.AuditLogEventTypeToolCall)},
			},
			want: 3,
		},
		{
			name: "resource_read matches backfilled call type",
			opts: MCPAuditLogOptions{
				EventType: []string{string(types2.AuditLogEventTypeResourceRead)},
			},
			want: 1,
		},
		{
			name: "mcp_request matches backfilled other call types",
			opts: MCPAuditLogOptions{
				EventType: []string{string(types2.AuditLogEventTypeMCPRequest)},
			},
			want: 1,
		},
		{
			name: "success outcome",
			opts: MCPAuditLogOptions{
				Outcome: []string{string(types2.AuditLogOutcomeSuccess)},
			},
			want: 3,
		},
		{
			name: "error outcome",
			opts: MCPAuditLogOptions{
				Outcome: []string{string(types2.AuditLogOutcomeError)},
			},
			want: 2,
		},
		{
			name: "device id",
			opts: MCPAuditLogOptions{
				DeviceID: []string{"dev-1"},
			},
			want: 1,
		},
		{
			name: "combined source and outcome",
			opts: MCPAuditLogOptions{
				SourceType: []string{string(types2.AuditLogSourceTypeMCP)},
				Outcome:    []string{string(types2.AuditLogOutcomeError)},
			},
			want: 2,
		},
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

func TestGetMCPAuditLogUsesSourceType(t *testing.T) {
	c, _ := newTestClientWithDB(t)
	now := time.Now().UTC()

	mcpRow := insertAuditLogRow(t, c, types.MCPAuditLog{
		CreatedAt:  now,
		SourceType: types2.AuditLogSourceTypeMCP,
		EventType:  types2.AuditLogEventTypeToolCall,
		CallType:   "tools/call",

		MCP: &types.MCPAuditLogFields{
			MCPID:          "mcp-1",
			ResponseStatus: 204,
		},
	})

	localRow := insertAuditLogRow(t, c, types.MCPAuditLog{
		CreatedAt:  now,
		SourceType: types2.AuditLogSourceTypeLocalAgent,
		EventType:  types2.AuditLogEventTypeToolCall,
		CallType:   "command",

		Local: &types.LocalAuditLog{
			RawEvent: []byte(`{"hook":"post"}`),
		},
	})

	gotMCP, err := c.GetMCPAuditLog(t.Context(), mcpRow.ID, true)
	if err != nil {
		t.Fatalf("GetMCPAuditLog() MCP row error: %v", err)
	}
	if gotMCP.SourceType != types2.AuditLogSourceTypeMCP || gotMCP.MCP == nil {
		t.Fatalf("MCP row source = %q mcp:%v, want MCP source fields", gotMCP.SourceType, gotMCP.MCP)
	}
	if gotMCP.MCP.MCPID != "mcp-1" || gotMCP.MCP.ResponseStatus != 204 {
		t.Fatalf("MCP fields were not hydrated: %+v", gotMCP.MCP)
	}
	apiMCP := types.ConvertMCPAuditLog(*gotMCP)
	if apiMCP.MCP == nil || apiMCP.Local != nil {
		t.Fatalf("MCP conversion source fields = mcp:%v local:%v, want only MCP", apiMCP.MCP, apiMCP.Local)
	}

	gotLocal, err := c.GetMCPAuditLog(t.Context(), localRow.ID, true)
	if err != nil {
		t.Fatalf("GetMCPAuditLog() local row error: %v", err)
	}
	if gotLocal.SourceType != types2.AuditLogSourceTypeLocalAgent || gotLocal.Local == nil {
		t.Fatalf("local row source = %q local:%v, want Local source fields", gotLocal.SourceType, gotLocal.Local)
	}
	if string(gotLocal.Local.RawEvent) != `{"hook":"post"}` {
		t.Fatalf("local fields were not hydrated: %+v", gotLocal.Local)
	}
	apiLocal := types.ConvertMCPAuditLog(*gotLocal)
	if apiLocal.Local == nil || apiLocal.MCP != nil {
		t.Fatalf("local conversion source fields = mcp:%v local:%v, want only Local", apiLocal.MCP, apiLocal.Local)
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
		CreatedAt:  time.Now().UTC(),
		SourceType: types2.AuditLogSourceTypeLocalAgent,
		Local:      &types.LocalAuditLog{EventID: "evt-dup"},
	})

	err := c.db.WithContext(t.Context()).Create(&types.MCPAuditLog{
		CreatedAt:  time.Now().UTC(),
		SourceType: types2.AuditLogSourceTypeLocalAgent,
		Local:      &types.LocalAuditLog{EventID: "evt-dup"},
	}).Error
	if err == nil {
		t.Fatalf("expected duplicate event_id insert to fail")
	}

	// Multiple NULL event IDs (historical rows) must coexist.
	first := insertAuditLogRow(t, c, types.MCPAuditLog{
		CreatedAt: time.Now().UTC(),
	})
	nullGenericColumns(t, c, first.ID)
	second := insertAuditLogRow(t, c, types.MCPAuditLog{
		CreatedAt: time.Now().UTC(),
	})
	nullGenericColumns(t, c, second.ID)
}

func TestInsertMCPAuditLogsDedupesLocalEventID(t *testing.T) {
	c, _ := newTestClientWithDB(t)
	now := time.Now().UTC()

	err := c.insertMCPAuditLogs(t.Context(), []types.MCPAuditLog{
		{
			CreatedAt:        now,
			SourceType:       types2.AuditLogSourceTypeLocalAgent,
			Local:            &types.LocalAuditLog{EventID: "evt-dup"},
			ResponseReceived: true,
		},
		{
			CreatedAt:        now.Add(time.Second),
			SourceType:       types2.AuditLogSourceTypeLocalAgent,
			Local:            &types.LocalAuditLog{EventID: "evt-dup"},
			ResponseReceived: true,
		},
		{
			CreatedAt:        now.Add(2 * time.Second),
			SourceType:       types2.AuditLogSourceTypeLocalAgent,
			Local:            &types.LocalAuditLog{EventID: "evt-next"},
			ResponseReceived: true,
		},
	})
	if err != nil {
		t.Fatalf("insertMCPAuditLogs() error: %v", err)
	}

	var count int64
	if err := c.db.WithContext(t.Context()).Model(&types.MCPAuditLog{}).Count(&count).Error; err != nil {
		t.Fatalf("failed to count audit logs: %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}

	var row types.MCPAuditLog
	if err := c.db.WithContext(t.Context()).Where("event_id = 'evt-dup'").First(&row).Error; err != nil {
		t.Fatalf("failed to fetch deduped row: %v", err)
	}
	gotEventID := ""
	if row.Local != nil {
		gotEventID = row.Local.EventID
	}
	if gotEventID != "evt-dup" {
		t.Errorf("EventID = %v, want evt-dup", gotEventID)
	}
}

func TestLogMCPAuditEntryForcesMCPClassification(t *testing.T) {
	c, _ := newTestClientWithDB(t)

	c.LogMCPAuditEntry(types.MCPAuditLog{
		CreatedAt:   time.Now().UTC(),
		SourceType:  types2.AuditLogSourceTypeLocalAgent,
		EventType:   types2.AuditLogEventTypeResourceRead,
		Outcome:     types2.AuditLogOutcomeError,
		CallType:    "tools/call",
		RequestBody: []byte(`{}`),

		MCP: &types.MCPAuditLogFields{
			ResponseStatus: 200,
		},
		ResponseReceived: true,
	})
	if err := c.persistAuditLogs(); err != nil {
		t.Fatalf("persistAuditLogs() error: %v", err)
	}

	var row types.MCPAuditLog
	if err := c.db.WithContext(t.Context()).First(&row).Error; err != nil {
		t.Fatalf("failed to fetch audit log: %v", err)
	}
	if row.SourceType != types2.AuditLogSourceTypeMCP {
		t.Errorf("SourceType = %q, want %q", row.SourceType, types2.AuditLogSourceTypeMCP)
	}
	if row.EventType != types2.AuditLogEventTypeToolCall {
		t.Errorf("EventType = %q, want %q", row.EventType, types2.AuditLogEventTypeToolCall)
	}
	if row.Outcome != types2.AuditLogOutcomeSuccess {
		t.Errorf("Outcome = %q, want %q", row.Outcome, types2.AuditLogOutcomeSuccess)
	}
	if row.ReceivedAt == nil {
		t.Error("ReceivedAt must be assigned by LogMCPAuditEntry")
	}
}
