package client

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

func TestScheduledMCPExportUsesServerUpdates(t *testing.T) {
	c := newTestClient(t)
	old := time.Now().UTC().Add(-48 * time.Hour)
	firstWrite := time.Now().UTC().Add(-time.Minute)
	beforeInsert := time.Now().UTC()
	request := types.MCPAuditLog{CreatedAt: old, UserID: "user", MCPFields: &types.MCPAuditLogFields{MCPID: "mcp", RequestID: "request", CallType: "tools/call"}}
	if err := c.insertMCPAuditLogs(t.Context(), []types.MCPAuditLog{request}); err != nil {
		t.Fatal(err)
	}
	var stored types.MCPAuditLog
	if err := c.db.WithContext(t.Context()).First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.UpdatedAt.Before(beforeInsert) || !stored.CreatedAt.Equal(old) {
		t.Fatalf("insert timestamps = %s, %s", stored.CreatedAt, stored.UpdatedAt)
	}
	if err := c.db.WithContext(t.Context()).Model(&stored).UpdateColumn("updated_at", firstWrite).Error; err != nil {
		t.Fatal(err)
	}
	cutoff := firstWrite.Add(time.Second)
	opts := MCPAuditLogOptions{StartTime: firstWrite, EndTime: cutoff, ScheduledExport: &AuditLogExportQuery{}}
	logs, _, err := c.GetMCPAuditLogs(t.Context(), opts)
	if err != nil || len(logs) != 1 {
		t.Fatalf("initial window logs=%v error=%v", logs, err)
	}
	responseWrite := time.Now().UTC()
	response := request
	response.MCPFields = &types.MCPAuditLogFields{MCPID: "mcp", RequestID: "request", ResponseReceived: true, ResponseStatus: 200}
	if err := c.insertMCPAuditLogs(t.Context(), []types.MCPAuditLog{response}); err != nil {
		t.Fatal(err)
	}
	opts.StartTime, opts.EndTime = cutoff.Add(-5*time.Second), responseWrite.Add(time.Second)
	logs, _, err = c.GetMCPAuditLogs(t.Context(), opts)
	if err != nil || len(logs) != 1 || !logs[0].MCP().ResponseReceived || logs[0].UpdatedAt.Before(responseWrite) || !logs[0].CreatedAt.Equal(old) || logs[0].ID != stored.ID {
		t.Fatalf("late response logs=%v error=%v", logs, err)
	}
	local := validLocalAgentAuditLog(old, "late-event", apitypes.AuditLogOutcomeStatusSuccess)
	if err := c.InsertLocalAgentAuditLogs(t.Context(), []types.MCPAuditLog{local}); err != nil {
		t.Fatal(err)
	}
	opts.SourceTypes = []apitypes.AuditLogSourceType{apitypes.AuditLogSourceTypeMCP, apitypes.AuditLogSourceTypeLocalAgentToolCall}
	logs, _, err = c.GetMCPAuditLogs(t.Context(), opts)
	if err != nil || len(logs) != 2 {
		t.Fatalf("late local event logs=%v error=%v", logs, err)
	}
	opts.ScheduledExport = nil
	logs, _, err = c.GetMCPAuditLogs(t.Context(), opts)
	if err != nil || len(logs) != 0 {
		t.Fatalf("event-time browsing/one-time export changed: logs=%v error=%v", logs, err)
	}
}

func TestScheduledMCPExportLegacyFallbackAndMutablePages(t *testing.T) {
	c := newTestClient(t)
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	rows := []types.MCPAuditLog{
		{
			CreatedAt: start,
			UpdatedAt: start,
		},
		{
			CreatedAt: start,
			UpdatedAt: start,
		},
		{
			CreatedAt: start,
			UpdatedAt: start,
		},
		{
			CreatedAt: end,
			UpdatedAt: end,
		},
	}
	if err := c.db.WithContext(t.Context()).Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	// These representations occur on rows predating the new nullable column and
	// rows written with explicit zero timestamps. Do not backfill them at migration.
	if err := c.db.WithContext(t.Context()).Model(&rows[0]).UpdateColumn("updated_at", nil).Error; err != nil {
		t.Fatal(err)
	}
	if err := c.db.WithContext(t.Context()).Model(&rows[1]).UpdateColumn("updated_at", time.Time{}).Error; err != nil {
		t.Fatal(err)
	}
	opts := MCPAuditLogOptions{StartTime: start, EndTime: end, Limit: 1, ScheduledExport: &AuditLogExportQuery{}}
	first, total, err := c.GetMCPAuditLogs(t.Context(), opts)
	if err != nil || len(first) != 1 || first[0].ID != rows[0].ID || total != 0 {
		t.Fatalf("first page=%v total=%d error=%v", first, total, err)
	}

	opts.ScheduledExport.AfterID = strconv.FormatUint(uint64(first[0].ID), 10)
	if err := c.db.WithContext(t.Context()).Model(&rows[0]).UpdateColumn("updated_at", end.Add(time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	for _, want := range rows[1:3] {
		logs, _, err := c.GetMCPAuditLogs(t.Context(), opts)
		if err != nil || len(logs) != 1 || logs[0].ID != want.ID {
			t.Fatalf("next page=%v want=%d error=%v", logs, want.ID, err)
		}

		opts.ScheduledExport.AfterID = strconv.FormatUint(uint64(logs[0].ID), 10)
	}
	logs, _, err := c.GetMCPAuditLogs(t.Context(), opts)
	if err != nil || len(logs) != 0 {
		t.Fatalf("exclusive end boundary failed: %v %v", logs, err)
	}
}

func TestScheduledLLMExportUpdateTimeAndPagination(t *testing.T) {
	c := newTestClient(t)
	start := time.Now().UTC()
	for _, id := range []string{"b", "a", "c"} {
		if err := c.InsertLLMAuditLog(t.Context(), &types.LLMAuditLog{ID: id, CreatedAt: start.Add(-24 * time.Hour), UserID: "user"}); err != nil {
			t.Fatal(err)
		}
	}
	var stored types.LLMAuditLog
	if err := c.db.WithContext(t.Context()).First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.UpdatedAt.Before(start) {
		t.Fatal("GORM did not stamp LLM insert")
	}
	if err := c.db.WithContext(t.Context()).Model(&types.LLMAuditLog{}).Where("1 = 1").UpdateColumn("updated_at", start).Error; err != nil {
		t.Fatal(err)
	}
	if err := c.db.WithContext(t.Context()).Model(&types.LLMAuditLog{ID: "c"}).UpdateColumns(map[string]any{"created_at": start, "updated_at": nil}).Error; err != nil {
		t.Fatal(err)
	}
	opts := LLMAuditLogOptions{StartTime: start, EndTime: start.Add(time.Hour), UserID: []string{"user"}, Limit: 1, ScheduledExport: &AuditLogExportQuery{}}
	for _, want := range []string{"a", "b", "c"} {
		logs, _, err := c.GetLLMAuditLogs(t.Context(), opts)
		if err != nil || len(logs) != 1 || logs[0].ID != want {
			t.Fatalf("page=%v want=%s error=%v", logs, want, err)
		}

		opts.ScheduledExport.AfterID = logs[0].ID
		if want == "a" {
			if err := c.db.WithContext(t.Context()).Model(&types.LLMAuditLog{ID: want}).UpdateColumn("updated_at", opts.EndTime).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	opts.ScheduledExport = nil
	logs, _, err := c.GetLLMAuditLogs(t.Context(), opts)
	if err != nil || len(logs) != 1 || logs[0].ID != "c" {
		t.Fatalf("one-time selection changed: %v %v", logs, err)
	}
}

func TestScheduledExportQueriesSkipTotalCount(t *testing.T) {
	c := newTestClient(t)
	if err := c.db.WithContext(t.Context()).Create(&types.MCPAuditLog{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := c.InsertLLMAuditLog(t.Context(), &types.LLMAuditLog{ID: "llm"}); err != nil {
		t.Fatal(err)
	}
	counter := newStatementCounter(t, c)
	for _, tc := range []struct {
		name  string
		query func(bool) (int64, error)
	}{
		{
			name: "MCP",
			query: func(scheduled bool) (int64, error) {
				opts := MCPAuditLogOptions{}
				if scheduled {
					opts.ScheduledExport = &AuditLogExportQuery{}
				}
				_, total, err := c.GetMCPAuditLogs(t.Context(), opts)
				return total, err
			},
		},
		{
			name: "LLM",
			query: func(scheduled bool) (int64, error) {
				opts := LLMAuditLogOptions{}
				if scheduled {
					opts.ScheduledExport = &AuditLogExportQuery{}
				}
				_, total, err := c.GetLLMAuditLogs(t.Context(), opts)
				return total, err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, scheduled := range []bool{false, true} {
				statements := counter.measure(func() {
					total, err := tc.query(scheduled)
					if err != nil {
						t.Fatal(err)
					}
					wantTotal := int64(1)
					if scheduled {
						wantTotal = 0
					}
					if total != wantTotal {
						t.Fatalf("scheduled=%v total=%d want=%d", scheduled, total, wantTotal)
					}
				})
				counts := 0
				for _, statement := range statements {
					if strings.Contains(strings.ToLower(statement), "count(") {
						counts++
					}
				}
				wantCounts := 1
				if scheduled {
					wantCounts = 0
				}
				if counts != wantCounts {
					t.Fatalf("scheduled=%v executed %d count queries, want %d", scheduled, counts, wantCounts)
				}
			}
		})
	}
}

func TestScheduledExportPostgresIndexPlan(t *testing.T) {
	c := newPostgresLifecycleTestClient(t)
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	for _, table := range []string{"mcp_audit_logs", "llm_audit_logs"} {
		t.Run(table, func(t *testing.T) {
			// Include legacy NULL and zero timestamps among 100,000 historical rows and
			// 20 recent rows. Ordinary timestamp indexes must bound selection before ID sorting.
			id := "g"
			if table == "llm_audit_logs" {
				id = "md5(g::text)"
			}
			insert := "INSERT INTO " + table + " (id, created_at, updated_at) SELECT " + id + ", stamp, CASE WHEN g % 3 = 0 THEN NULL WHEN g % 3 = 1 THEN '0001-01-01 00:00:00+00:00'::timestamptz ELSE stamp END FROM (SELECT g, CASE WHEN g <= 100000 THEN '2025-01-01'::timestamptz + g * interval '1 second' ELSE ?::timestamptz END AS stamp FROM generate_series(1, 100020) AS g) AS history"
			if err := c.db.WithContext(t.Context()).Exec(insert, start).Error; err != nil {
				t.Fatal(err)
			}
			if err := c.db.WithContext(t.Context()).Exec("ANALYZE " + table).Error; err != nil {
				t.Fatal(err)
			}
			var query string
			var args []any
			callbacks := c.db.WithContext(t.Context()).Callback().Query()
			const callback = "test:scheduled_export_plan"
			if err := callbacks.After("gorm:query").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Table == table {
					query = tx.Statement.SQL.String()
					args = append([]any(nil), tx.Statement.Vars...)
				}
			}); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = callbacks.Remove(callback) }()
			cursor := &AuditLogExportQuery{}
			for page := range 2 {
				if table == "mcp_audit_logs" {
					logs, total, err := c.GetMCPAuditLogs(t.Context(), MCPAuditLogOptions{StartTime: start, EndTime: start.Add(time.Hour), Limit: 10, ScheduledExport: cursor})
					if err != nil || len(logs) != 10 || total != 0 {
						t.Fatalf("page=%d logs=%d total=%d error=%v", page, len(logs), total, err)
					}
					last := logs[len(logs)-1]
					cursor.AfterID = strconv.FormatUint(uint64(last.ID), 10)
				} else {
					logs, total, err := c.GetLLMAuditLogs(t.Context(), LLMAuditLogOptions{StartTime: start, EndTime: start.Add(time.Hour), Limit: 10, ScheduledExport: cursor})
					if err != nil || len(logs) != 10 || total != 0 {
						t.Fatalf("page=%d logs=%d total=%d error=%v", page, len(logs), total, err)
					}
					last := logs[len(logs)-1]
					cursor.AfterID = last.ID
				}
				rows, err := c.db.WithContext(t.Context()).Raw("EXPLAIN (ANALYZE, BUFFERS) "+query, args...).Rows()
				if err != nil {
					t.Fatal(err)
				}
				var lines []string
				for rows.Next() {
					var line string
					if err := rows.Scan(&line); err != nil {
						t.Fatal(err)
					}
					lines = append(lines, line)
				}
				if err := rows.Err(); err != nil {
					t.Fatal(err)
				}
				if err := rows.Close(); err != nil {
					t.Fatal(err)
				}
				plan := strings.Join(lines, "\n")
				t.Logf("page %d:\n%s", page, plan)
				if !strings.Contains(plan, "Index") || strings.Contains(plan, "Seq Scan") {
					t.Fatalf("scheduled window did not use indexed selection:\n%s", plan)
				}
				// A small sort over matching rows is fine; scanning old rows by primary key
				// and discarding retained history defeats the bounded timestamp window.
				for _, match := range regexp.MustCompile(`Rows Removed by Filter: (\d+)`).FindAllStringSubmatch(plan, -1) {
					removed, err := strconv.Atoi(match[1])
					if err != nil {
						t.Fatal(err)
					}
					if removed > 100 {
						t.Fatalf("scheduled query scanned retained history:\n%s", plan)
					}
				}
			}
		})
	}
}
