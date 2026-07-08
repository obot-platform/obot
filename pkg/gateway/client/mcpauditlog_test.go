package client

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/datatypes"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/server/options/encryptionconfig"
	"k8s.io/apiserver/pkg/storage/value"
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

func TestGetMCPAuditLogLocalAgentDoesNotRequireMCPFields(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()
	now := time.Now().UTC()

	log := validLocalAgentAuditLog(now, "entry-1", types2.AuditLogOutcomeStatusSuccess)

	if err := c.db.WithContext(ctx).Create(&log).Error; err != nil {
		t.Fatalf("insert local-agent audit log: %v", err)
	}

	got, err := c.GetMCPAuditLog(ctx, log.ID, false)
	if err != nil {
		t.Fatalf("get local-agent audit log: %v", err)
	}
	if got.MCPFields != nil {
		t.Fatalf("expected no MCP fields for local-agent log, got %#v", got.MCPFields)
	}
	local := got.LocalAgentToolCallFields
	if local == nil {
		t.Fatal("expected local-agent fields")
	}
	if local.RequestBody != nil || local.ResponseBody != nil || local.RawEvent != nil || local.TranscriptPath != "" {
		t.Fatalf("expected sensitive local-agent payload fields to be blanked, got %#v", local)
	}
}

func TestInsertLocalAgentAuditLogsCompletedSuccess(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()

	log := validLocalAgentAuditLog(time.Now().UTC(), "entry-1", types2.AuditLogOutcomeStatusSuccess)
	if err := c.InsertLocalAgentAuditLogs(ctx, []types.MCPAuditLog{log}); err != nil {
		t.Fatalf("insert local-agent audit log: %v", err)
	}

	if got := countAuditLogs(t, c); got != 1 {
		t.Fatalf("expected 1 audit log, got %d", got)
	}

	var stored types.MCPAuditLog
	if err := c.db.WithContext(ctx).First(&stored).Error; err != nil {
		t.Fatalf("load stored audit log: %v", err)
	}
	if stored.SourceType != types2.AuditLogSourceTypeLocalAgentToolCall {
		t.Fatalf("expected local-agent source type, got %q", stored.SourceType)
	}
	if stored.MCPFields != nil && (stored.MCPFields.MCPID != "" || len(stored.MCPFields.RequestBody) > 0 || len(stored.MCPFields.ResponseBody) > 0) {
		t.Fatalf("expected no populated MCP fields, got %#v", stored.MCPFields)
	}
}

func TestInsertLocalAgentAuditLogsAcceptsTerminalStatuses(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()
	now := time.Now().UTC()

	statuses := []types2.AuditLogOutcomeStatus{
		types2.AuditLogOutcomeStatusSuccess,
		types2.AuditLogOutcomeStatusFailure,
		types2.AuditLogOutcomeStatusDenied,
		types2.AuditLogOutcomeStatusTimeout,
	}
	for i, status := range statuses {
		log := validLocalAgentAuditLog(now.Add(time.Duration(i)*time.Second), string(status)+"-entry", status)
		if err := c.InsertLocalAgentAuditLogs(ctx, []types.MCPAuditLog{log}); err != nil {
			t.Fatalf("insert %s local-agent audit log: %v", status, err)
		}
	}

	if got := countAuditLogs(t, c); got != int64(len(statuses)) {
		t.Fatalf("expected %d audit logs, got %d", len(statuses), got)
	}
}

func TestInsertLocalAgentAuditLogsDuplicateIdempotencyKeyIsNoop(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()
	now := time.Now().UTC()

	first := validLocalAgentAuditLog(now, "same-entry", types2.AuditLogOutcomeStatusSuccess)
	duplicate := validLocalAgentAuditLog(now.Add(time.Second), "same-entry", types2.AuditLogOutcomeStatusFailure)
	duplicate.LocalAgentToolCallFields.ActionName = "different-tool"

	if err := c.InsertLocalAgentAuditLogs(ctx, []types.MCPAuditLog{first}); err != nil {
		t.Fatalf("insert first local-agent audit log: %v", err)
	}
	if err := c.InsertLocalAgentAuditLogs(ctx, []types.MCPAuditLog{duplicate}); err != nil {
		t.Fatalf("duplicate idempotency key should be a no-op: %v", err)
	}

	if got := countAuditLogs(t, c); got != 1 {
		t.Fatalf("expected duplicate idempotency key to keep 1 audit log, got %d", got)
	}
	var stored types.MCPAuditLog
	if err := c.db.WithContext(ctx).First(&stored).Error; err != nil {
		t.Fatalf("load stored audit log: %v", err)
	}
	if stored.LocalAgentToolCallFields.ActionName != "mcp__server__tool" {
		t.Fatalf("expected original row to remain unchanged, got tool %q", stored.LocalAgentToolCallFields.ActionName)
	}
}

func TestInsertLocalAgentAuditLogsRejectsMissingRequiredFields(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()

	log := validLocalAgentAuditLog(time.Now().UTC(), "", types2.AuditLogOutcomeStatusSuccess)
	if err := c.InsertLocalAgentAuditLogs(ctx, []types.MCPAuditLog{log}); err == nil {
		t.Fatal("expected missing idempotency key to be rejected")
	}
}

func TestInsertLocalAgentAuditLogsRejectsNonTerminalStatus(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()

	log := validLocalAgentAuditLog(time.Now().UTC(), "entry-1", types2.AuditLogOutcomeStatus("pre_tool"))
	if err := c.InsertLocalAgentAuditLogs(ctx, []types.MCPAuditLog{log}); err == nil {
		t.Fatal("expected non-terminal status to be rejected")
	}
}

func TestLocalAgentAuditLogEncryptedFieldsDecryptWhenRequested(t *testing.T) {
	c := newTestClient(t)
	c.encryptionConfig = testEncryptionConfig()
	ctx := t.Context()
	now := time.Now().UTC()

	log := validLocalAgentAuditLog(now, "entry-1", types2.AuditLogOutcomeStatusSuccess)
	if err := c.InsertLocalAgentAuditLogs(ctx, []types.MCPAuditLog{log}); err != nil {
		t.Fatalf("insert local-agent audit log: %v", err)
	}

	var stored types.MCPAuditLog
	if err := c.db.WithContext(ctx).First(&stored).Error; err != nil {
		t.Fatalf("load encrypted local-agent audit log: %v", err)
	}
	if !stored.Encrypted {
		t.Fatal("expected stored audit log to be marked encrypted")
	}
	local := stored.LocalAgentToolCallFields
	if local.DeviceID != "device-1" {
		t.Fatalf("expected device ID to be stored unencrypted, got %q", local.DeviceID)
	}
	if local.OutcomeError == "permission denied for /Users/alice/project/secret.txt" ||
		local.Hostname == "alice-macbook" ||
		local.LocalUsername == "alice" ||
		local.ReportedUserEmail == "alice@example.com" ||
		local.CWD == "/Users/alice/project" ||
		local.GitRoot == "/Users/alice/project" ||
		local.GitRemotes[0] == "git@github.com:acme/private-repo.git" ||
		local.GitBranch == "alice/customer-fix" ||
		local.TranscriptPath == "/tmp/transcript.jsonl" ||
		bytes.Equal(local.RequestBody, []byte(`{"arg":true}`)) ||
		bytes.Equal(local.ResponseBody, []byte(`{"ok":true}`)) ||
		bytes.Equal(local.RawEvent, []byte(`{"native":true}`)) {
		t.Fatalf("expected sensitive local-agent fields to be encrypted at rest: %#v", local)
	}

	got, err := c.GetMCPAuditLog(ctx, stored.ID, true)
	if err != nil {
		t.Fatalf("get decrypted local-agent audit log: %v", err)
	}
	gotLocal := got.LocalAgentToolCallFields
	if gotLocal.OutcomeError != "permission denied for /Users/alice/project/secret.txt" ||
		gotLocal.DeviceID != "device-1" ||
		gotLocal.Hostname != "alice-macbook" ||
		gotLocal.LocalUsername != "alice" ||
		gotLocal.ReportedUserEmail != "alice@example.com" ||
		gotLocal.CWD != "/Users/alice/project" ||
		gotLocal.GitRoot != "/Users/alice/project" ||
		gotLocal.GitRemotes[0] != "git@github.com:acme/private-repo.git" ||
		gotLocal.GitBranch != "alice/customer-fix" ||
		gotLocal.TranscriptPath != "/tmp/transcript.jsonl" ||
		string(gotLocal.RequestBody) != `{"arg":true}` ||
		string(gotLocal.ResponseBody) != `{"ok":true}` ||
		string(gotLocal.RawEvent) != `{"native":true}` {
		t.Fatalf("expected local-agent sensitive fields to decrypt, got %#v", gotLocal)
	}

	blanked, err := c.GetMCPAuditLog(ctx, stored.ID, false)
	if err != nil {
		t.Fatalf("get blanked local-agent audit log: %v", err)
	}
	blankedLocal := blanked.LocalAgentToolCallFields
	if blankedLocal.OutcomeError != "" ||
		blankedLocal.Hostname != "" ||
		blankedLocal.LocalUsername != "" ||
		blankedLocal.ReportedUserEmail != "" ||
		blankedLocal.CWD != "" ||
		blankedLocal.GitRoot != "" ||
		blankedLocal.GitRemotes != nil ||
		blankedLocal.GitBranch != "" ||
		blankedLocal.TranscriptPath != "" ||
		blankedLocal.RequestBody != nil ||
		blankedLocal.ResponseBody != nil ||
		blankedLocal.RawEvent != nil {
		t.Fatalf("expected sensitive fields to be blanked without payload access, got %#v", blankedLocal)
	}
}

func TestMCPAuditLogEncryptionStillDecryptsMCPFields(t *testing.T) {
	c := newTestClient(t)
	c.encryptionConfig = testEncryptionConfig()
	ctx := t.Context()
	now := time.Now().UTC()

	log := types.MCPAuditLog{
		CreatedAt:  now,
		SourceType: types2.AuditLogSourceTypeMCP,
		UserID:     "user-1",
		MCPFields: &types.MCPAuditLogFields{
			MCPID:           "mcp-1",
			RequestBody:     json.RawMessage(`{"name":"tool"}`),
			ResponseBody:    json.RawMessage(`{"ok":true}`),
			RequestHeaders:  json.RawMessage(`{"authorization":"bearer token"}`),
			ResponseHeaders: json.RawMessage(`{"content-type":"application/json"}`),
		},
	}

	if err := c.encryptMCPAuditLog(ctx, &log); err != nil {
		t.Fatalf("encrypt MCP audit log: %v", err)
	}
	if string(log.MCP().RequestBody) == `{"name":"tool"}` {
		t.Fatal("expected MCP request body to be encrypted")
	}
	if err := c.decryptMCPAuditLog(ctx, &log); err != nil {
		t.Fatalf("decrypt MCP audit log: %v", err)
	}
	if string(log.MCP().RequestBody) != `{"name":"tool"}` ||
		string(log.MCP().ResponseBody) != `{"ok":true}` ||
		string(log.MCP().RequestHeaders) != `{"authorization":"bearer token"}` ||
		string(log.MCP().ResponseHeaders) != `{"content-type":"application/json"}` {
		t.Fatalf("expected MCP fields to decrypt, got %#v", log.MCP())
	}
}

func TestGetLocalAgentAuditLogsFiltersBySourceTypeAndBlanksPayloads(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()
	now := time.Now().UTC()

	mcpLog := types.MCPAuditLog{
		CreatedAt:  now,
		SourceType: types2.AuditLogSourceTypeMCP,
		UserID:     "user-1",
		MCPFields: &types.MCPAuditLogFields{
			MCPID:          "mcp-1",
			CallType:       "tools/call",
			CallIdentifier: "tool-1",
			RequestBody:    json.RawMessage(`{"name":"tool-1"}`),
		},
	}
	if err := c.insertMCPAuditLogs(ctx, []types.MCPAuditLog{mcpLog}); err != nil {
		t.Fatalf("insert MCP audit log: %v", err)
	}
	local := validLocalAgentAuditLog(now, "entry-1", types2.AuditLogOutcomeStatusSuccess)
	if err := c.InsertLocalAgentAuditLogs(ctx, []types.MCPAuditLog{local}); err != nil {
		t.Fatalf("insert local-agent audit log: %v", err)
	}

	logs, total, err := c.GetMCPAuditLogs(ctx, MCPAuditLogOptions{
		SourceTypes: []types2.AuditLogSourceType{types2.AuditLogSourceTypeLocalAgentToolCall},
	})
	if err != nil {
		t.Fatalf("list local-agent audit logs: %v", err)
	}
	if total != 1 || len(logs) != 1 {
		t.Fatalf("expected exactly one local-agent row, got total=%d len=%d", total, len(logs))
	}
	got := logs[0]
	if got.SourceType != types2.AuditLogSourceTypeLocalAgentToolCall {
		t.Fatalf("expected local-agent source type, got %q", got.SourceType)
	}
	if got.MCPFields != nil {
		t.Fatalf("expected MCP fields to be nil for local-agent list row, got %#v", got.MCPFields)
	}
	lf := got.LocalAgentToolCallFields
	if lf == nil {
		t.Fatal("expected local-agent fields")
	}
	// DeviceID is non-sensitive metadata and should remain.
	if lf.DeviceID != "device-1" {
		t.Fatalf("expected device ID metadata, got %q", lf.DeviceID)
	}
	// Sensitive payloads must be blanked in list responses.
	if lf.RequestBody != nil || lf.ResponseBody != nil || lf.RawEvent != nil ||
		lf.CWD != "" || lf.Hostname != "" || lf.GitBranch != "" || lf.OutcomeError != "" {
		t.Fatalf("expected sensitive fields to be blanked in list, got %#v", lf)
	}

	// A default (MCP) list must not return the local-agent row.
	mcpLogs, mcpTotal, err := c.GetMCPAuditLogs(ctx, MCPAuditLogOptions{})
	if err != nil {
		t.Fatalf("list MCP audit logs: %v", err)
	}
	if mcpTotal != 1 || len(mcpLogs) != 1 || mcpLogs[0].SourceType != types2.AuditLogSourceTypeMCP {
		t.Fatalf("expected exactly one MCP row by default, got total=%d len=%d", mcpTotal, len(mcpLogs))
	}
}

// TestGetLocalAgentAuditLogsWithRequestAndResponseDecrypts proves the export path
// (WithRequestAndResponse=true, only set for Auditor-role callers) returns decrypted payloads
// through the list query, while the normal list path (false) blanks them.
func TestGetLocalAgentAuditLogsWithRequestAndResponseDecrypts(t *testing.T) {
	c := newTestClient(t)
	c.encryptionConfig = testEncryptionConfig()
	ctx := t.Context()
	now := time.Now().UTC()

	log := validLocalAgentAuditLog(now, "entry-1", types2.AuditLogOutcomeStatusSuccess)
	if err := c.InsertLocalAgentAuditLogs(ctx, []types.MCPAuditLog{log}); err != nil {
		t.Fatalf("insert local-agent audit log: %v", err)
	}

	// Without payload access (normal list), sensitive fields are blanked.
	blanked, _, err := c.GetMCPAuditLogs(ctx, MCPAuditLogOptions{
		SourceTypes: []types2.AuditLogSourceType{types2.AuditLogSourceTypeLocalAgentToolCall},
	})
	if err != nil {
		t.Fatalf("list without payload: %v", err)
	}
	if len(blanked) != 1 {
		t.Fatalf("expected one row, got %d", len(blanked))
	}
	if bl := blanked[0].LocalAgentToolCallFields; bl.RequestBody != nil || bl.CWD != "" || bl.OutcomeError != "" {
		t.Fatalf("expected sensitive fields blanked without payload access, got %#v", bl)
	}

	// With payload access (export for auditors), sensitive fields decrypt.
	withPayload, _, err := c.GetMCPAuditLogs(ctx, MCPAuditLogOptions{
		SourceTypes:            []types2.AuditLogSourceType{types2.AuditLogSourceTypeLocalAgentToolCall},
		WithRequestAndResponse: true,
	})
	if err != nil {
		t.Fatalf("list with payload: %v", err)
	}
	if len(withPayload) != 1 {
		t.Fatalf("expected one row, got %d", len(withPayload))
	}
	wp := withPayload[0].LocalAgentToolCallFields
	if string(wp.RequestBody) != `{"arg":true}` || string(wp.ResponseBody) != `{"ok":true}` ||
		string(wp.RawEvent) != `{"native":true}` {
		t.Fatalf("expected decrypted payloads, got request=%s response=%s raw=%s", wp.RequestBody, wp.ResponseBody, wp.RawEvent)
	}
	if wp.CWD != "/Users/alice/project" || wp.Hostname != "alice-macbook" || wp.OutcomeError == "" {
		t.Fatalf("expected decrypted sensitive metadata, got %#v", wp)
	}
}

func TestGetLocalAgentAuditLogsAppliesFilters(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()
	now := time.Now().UTC()

	logs := []types.MCPAuditLog{
		validLocalAgentAuditLog(now, "codex-success", types2.AuditLogOutcomeStatusSuccess),
		validLocalAgentAuditLog(now.Add(time.Second), "codex-failed", types2.AuditLogOutcomeStatusFailure),
	}
	logs[1].LocalAgentToolCallFields.AgentProvider = types2.LocalAgentProviderClaudeCode
	for i := range logs {
		if err := c.InsertLocalAgentAuditLogs(ctx, []types.MCPAuditLog{logs[i]}); err != nil {
			t.Fatalf("insert local-agent audit log: %v", err)
		}
	}

	filtered, total, err := c.GetMCPAuditLogs(ctx, MCPAuditLogOptions{
		SourceTypes:   []types2.AuditLogSourceType{types2.AuditLogSourceTypeLocalAgentToolCall},
		AgentProvider: []string{string(types2.LocalAgentProviderClaudeCode)},
	})
	if err != nil {
		t.Fatalf("list filtered local-agent audit logs: %v", err)
	}
	if total != 1 || len(filtered) != 1 {
		t.Fatalf("expected one claude_code row, got total=%d len=%d", total, len(filtered))
	}
	if filtered[0].LocalAgentToolCallFields.OutcomeStatus != types2.AuditLogOutcomeStatusFailure {
		t.Fatalf("expected the failed claude_code row, got status %q", filtered[0].LocalAgentToolCallFields.OutcomeStatus)
	}

	byStatus, total, err := c.GetMCPAuditLogs(ctx, MCPAuditLogOptions{
		SourceTypes: []types2.AuditLogSourceType{types2.AuditLogSourceTypeLocalAgentToolCall},
		Status:      []string{string(types2.AuditLogOutcomeStatusSuccess)},
	})
	if err != nil {
		t.Fatalf("list by status: %v", err)
	}
	if total != 1 || len(byStatus) != 1 || byStatus[0].LocalAgentToolCallFields.IdempotencyKey != "codex-success" {
		t.Fatalf("expected the codex-success row, got total=%d len=%d", total, len(byStatus))
	}
}

func TestGetLocalAgentAuditLogsFiltersByOccurredAt(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()
	window := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)

	receivedInWindow := validLocalAgentAuditLog(window.Add(-48*time.Hour), "received-in-window", types2.AuditLogOutcomeStatusSuccess)
	receivedInWindow.CreatedAt = window
	occurredInWindow := validLocalAgentAuditLog(window, "occurred-in-window", types2.AuditLogOutcomeStatusSuccess)
	occurredInWindow.CreatedAt = window.Add(-48 * time.Hour)
	if err := c.InsertLocalAgentAuditLogs(ctx, []types.MCPAuditLog{receivedInWindow, occurredInWindow}); err != nil {
		t.Fatalf("insert local-agent audit logs: %v", err)
	}

	logs, total, err := c.GetMCPAuditLogs(ctx, MCPAuditLogOptions{
		SourceTypes: []types2.AuditLogSourceType{types2.AuditLogSourceTypeLocalAgentToolCall},
		StartTime:   window.Add(-time.Hour),
		EndTime:     window.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("list local-agent audit logs: %v", err)
	}
	if total != 1 || len(logs) != 1 {
		t.Fatalf("expected one log in occurred-time window, got total=%d len=%d", total, len(logs))
	}
	if got := logs[0].LocalAgentToolCallFields.IdempotencyKey; got != "occurred-in-window" {
		t.Fatalf("expected occurred-in-window log, got %q", got)
	}
}

func TestGetLocalAgentAuditLogsDefaultsToOccurredAtOrderWithIDTieBreaker(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()
	base := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)

	older := validLocalAgentAuditLog(base, "older", types2.AuditLogOutcomeStatusSuccess)
	older.CreatedAt = base.Add(3 * time.Hour)
	newerLowerID := validLocalAgentAuditLog(base.Add(time.Hour), "newer-lower-id", types2.AuditLogOutcomeStatusSuccess)
	newerLowerID.CreatedAt = base.Add(2 * time.Hour)
	newerHigherID := validLocalAgentAuditLog(base.Add(time.Hour), "newer-higher-id", types2.AuditLogOutcomeStatusSuccess)
	newerHigherID.CreatedAt = base.Add(time.Hour)
	if err := c.InsertLocalAgentAuditLogs(ctx, []types.MCPAuditLog{older, newerLowerID, newerHigherID}); err != nil {
		t.Fatalf("insert local-agent audit logs: %v", err)
	}

	want := []string{"newer-higher-id", "newer-lower-id", "older"}
	for offset, wantKey := range want {
		logs, total, err := c.GetMCPAuditLogs(ctx, MCPAuditLogOptions{
			SourceTypes: []types2.AuditLogSourceType{types2.AuditLogSourceTypeLocalAgentToolCall},
			Limit:       1,
			Offset:      offset,
		})
		if err != nil {
			t.Fatalf("list local-agent audit logs at offset %d: %v", offset, err)
		}
		if total != int64(len(want)) || len(logs) != 1 {
			t.Fatalf("offset %d: expected total=%d and one log, got total=%d len=%d", offset, len(want), total, len(logs))
		}
		if got := logs[0].LocalAgentToolCallFields.IdempotencyKey; got != wantKey {
			t.Fatalf("offset %d: expected %q, got %q", offset, wantKey, got)
		}
	}
}

func TestGetLocalAgentAuditLogFilterOptions(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()
	now := time.Now().UTC()

	codex := validLocalAgentAuditLog(now, "codex-entry", types2.AuditLogOutcomeStatusSuccess)
	claude := validLocalAgentAuditLog(now.Add(time.Second), "claude-entry", types2.AuditLogOutcomeStatusSuccess)
	claude.LocalAgentToolCallFields.AgentProvider = types2.LocalAgentProviderClaudeCode
	for _, log := range []types.MCPAuditLog{codex, claude} {
		if err := c.InsertLocalAgentAuditLogs(ctx, []types.MCPAuditLog{log}); err != nil {
			t.Fatalf("insert local-agent audit log: %v", err)
		}
	}

	opts := MCPAuditLogOptions{SourceTypes: []types2.AuditLogSourceType{types2.AuditLogSourceTypeLocalAgentToolCall}}
	options, err := c.GetAuditLogFilterOptions(ctx, "agent_provider", opts, "")
	if err != nil {
		t.Fatalf("get agent_provider filter options: %v", err)
	}
	if len(options) != 2 {
		t.Fatalf("expected two agent providers, got %v", options)
	}
	found := map[string]bool{}
	for _, o := range options {
		found[o] = true
	}
	if !found[string(types2.LocalAgentProviderCodex)] || !found[string(types2.LocalAgentProviderClaudeCode)] {
		t.Fatalf("expected codex and claude_code providers, got %v", options)
	}
}

func TestGetLocalAgentAuditLogFilterOptionsUseOccurredAt(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()
	window := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)

	receivedInWindow := validLocalAgentAuditLog(window.Add(-48*time.Hour), "received-in-window", types2.AuditLogOutcomeStatusSuccess)
	receivedInWindow.CreatedAt = window
	receivedInWindow.LocalAgentToolCallFields.AgentProvider = types2.LocalAgentProviderCodex
	occurredInWindow := validLocalAgentAuditLog(window, "occurred-in-window", types2.AuditLogOutcomeStatusSuccess)
	occurredInWindow.CreatedAt = window.Add(-48 * time.Hour)
	occurredInWindow.LocalAgentToolCallFields.AgentProvider = types2.LocalAgentProviderClaudeCode
	if err := c.InsertLocalAgentAuditLogs(ctx, []types.MCPAuditLog{receivedInWindow, occurredInWindow}); err != nil {
		t.Fatalf("insert local-agent audit logs: %v", err)
	}

	options, err := c.GetAuditLogFilterOptions(ctx, "agent_provider", MCPAuditLogOptions{
		SourceTypes: []types2.AuditLogSourceType{types2.AuditLogSourceTypeLocalAgentToolCall},
		StartTime:   window.Add(-time.Hour),
		EndTime:     window.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("get agent_provider filter options: %v", err)
	}
	if len(options) != 1 || options[0] != string(types2.LocalAgentProviderClaudeCode) {
		t.Fatalf("expected only observed-time option %q, got %v", types2.LocalAgentProviderClaudeCode, options)
	}
}

func TestGetMCPAuditLogsMixedOrderingPaginationAndTimeFilter(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()
	base := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)

	insertMCP := func(at time.Time, name string) {
		t.Helper()
		if err := c.insertMCPAuditLogs(ctx, []types.MCPAuditLog{{
			CreatedAt: at, SourceType: types2.AuditLogSourceTypeMCP,
			MCPFields: &types.MCPAuditLogFields{
				MCPID: "mcp-1", CallType: "tools/call", CallIdentifier: name,
				ResponseReceived: true, ResponseStatus: 200,
			},
		}}); err != nil {
			t.Fatalf("insert MCP log: %v", err)
		}
	}
	insertLocal := func(observed, recorded time.Time, key string) {
		t.Helper()
		log := validLocalAgentAuditLog(observed, key, types2.AuditLogOutcomeStatusSuccess)
		log.CreatedAt = recorded
		if err := c.InsertLocalAgentAuditLogs(ctx, []types.MCPAuditLog{log}); err != nil {
			t.Fatalf("insert local log: %v", err)
		}
	}

	insertMCP(base.Add(3*time.Hour), "mcp-newest")
	insertLocal(base.Add(2*time.Hour), base.Add(20*time.Hour), "local-second")
	insertMCP(base.Add(time.Hour), "mcp-third")
	insertLocal(base, base.Add(30*time.Hour), "local-oldest")

	opts := MCPAuditLogOptions{
		SourceTypes: []types2.AuditLogSourceType{
			types2.AuditLogSourceTypeMCP,
			types2.AuditLogSourceTypeLocalAgentToolCall,
		},
		Limit: 2, Offset: 1,
	}
	logs, total, err := c.GetMCPAuditLogs(ctx, opts)
	if err != nil {
		t.Fatalf("mixed list: %v", err)
	}
	if total != 4 || len(logs) != 2 {
		t.Fatalf("expected total 4 and page length 2, got total=%d len=%d", total, len(logs))
	}
	if logs[0].LocalAgentToolCallFields == nil || logs[0].LocalAgentToolCallFields.IdempotencyKey != "local-second" ||
		logs[1].MCP() == nil || logs[1].MCP().CallIdentifier != "mcp-third" {
		t.Fatalf("unexpected mixed page order: %#v", logs)
	}

	opts.Offset, opts.Limit = 0, 0
	opts.StartTime, opts.EndTime = base.Add(90*time.Minute), base.Add(150*time.Minute)
	logs, total, err = c.GetMCPAuditLogs(ctx, opts)
	if err != nil {
		t.Fatalf("mixed time filter: %v", err)
	}
	if total != 1 || len(logs) != 1 || logs[0].LocalAgentToolCallFields == nil || logs[0].LocalAgentToolCallFields.IdempotencyKey != "local-second" {
		t.Fatalf("time filter must use each source's event time, got total=%d logs=%#v", total, logs)
	}
}

func TestValidateAuditLogOptionsRejectsSourceSpecificFiltersWithMultipleSources(t *testing.T) {
	both := []types2.AuditLogSourceType{
		types2.AuditLogSourceTypeMCP,
		types2.AuditLogSourceTypeLocalAgentToolCall,
	}
	mcpOnly := []types2.AuditLogSourceType{types2.AuditLogSourceTypeMCP}
	localOnly := []types2.AuditLogSourceType{types2.AuditLogSourceTypeLocalAgentToolCall}

	tests := []struct {
		name    string
		opts    MCPAuditLogOptions
		wantErr bool
	}{
		{
			name:    "mcp filter with both sources is rejected",
			opts:    MCPAuditLogOptions{SourceTypes: both, MCPID: []string{"mcp-1"}},
			wantErr: true,
		},
		{
			name:    "local filter with both sources is rejected",
			opts:    MCPAuditLogOptions{SourceTypes: both, AgentProvider: []string{string(types2.LocalAgentProviderCodex)}},
			wantErr: true,
		},
		{
			name:    "mcp filter scoped to the mcp source is allowed",
			opts:    MCPAuditLogOptions{SourceTypes: mcpOnly, MCPID: []string{"mcp-1"}},
			wantErr: false,
		},
		{
			name:    "local filter scoped to the local source is allowed",
			opts:    MCPAuditLogOptions{SourceTypes: localOnly, AgentProvider: []string{string(types2.LocalAgentProviderCodex)}},
			wantErr: false,
		},
		{
			name:    "common filters with both sources are allowed",
			opts:    MCPAuditLogOptions{SourceTypes: both, UserID: []string{"user-1"}},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAuditLogOptions(tt.opts, tt.opts.SourceTypes)
			if tt.wantErr && err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected no validation error, got %v", err)
			}
		})
	}
}

func validLocalAgentAuditLog(occurredAt time.Time, idempotencyKey string, status types2.AuditLogOutcomeStatus) types.MCPAuditLog {
	return types.MCPAuditLog{
		CreatedAt:  occurredAt,
		SourceType: types2.AuditLogSourceTypeLocalAgentToolCall,
		UserID:     "user-1",
		ClientIP:   "127.0.0.1",
		LocalAgentToolCallFields: &types.LocalAgentToolCallAuditLogFields{
			OccurredAt:        occurredAt,
			ActorType:         types2.AuditLogActorTypeDevice,
			ActorID:           "device-1",
			ActionName:        "mcp__server__tool",
			ActionKind:        "mcp",
			TargetType:        types2.AuditLogTargetTypeMCPTool,
			TargetName:        "tool",
			TargetParentType:  types2.AuditLogTargetTypeMCPServer,
			TargetParentName:  "server",
			OutcomeStatus:     status,
			OutcomeError:      "permission denied for /Users/alice/project/secret.txt",
			AgentProvider:     types2.LocalAgentProviderCodex,
			CLIVersion:        "1.2.3",
			IdempotencyKey:    idempotencyKey,
			DeviceID:          "device-1",
			Hostname:          "alice-macbook",
			LocalUsername:     "alice",
			ReportedUserEmail: "alice@example.com",
			CWD:               "/Users/alice/project",
			GitRoot:           "/Users/alice/project",
			GitRemotes:        datatypes.JSONSlice[string]{"git@github.com:acme/private-repo.git"},
			GitBranch:         "alice/customer-fix",
			RequestBody:       json.RawMessage(`{"arg":true}`),
			ResponseBody:      json.RawMessage(`{"ok":true}`),
			RawEvent:          json.RawMessage(`{"native":true}`),
			TranscriptPath:    "/tmp/transcript.jsonl",
		},
	}
}

func testEncryptionConfig() *encryptionconfig.EncryptionConfiguration {
	return &encryptionconfig.EncryptionConfiguration{
		Transformers: map[schema.GroupResource]value.Transformer{
			mcpAuditLogGroupResource: testTransformer{},
		},
	}
}

type testTransformer struct{}

func (testTransformer) TransformToStorage(_ context.Context, data []byte, _ value.Context) ([]byte, error) {
	return append([]byte("encrypted:"), data...), nil
}

func (testTransformer) TransformFromStorage(_ context.Context, data []byte, _ value.Context) ([]byte, bool, error) {
	return bytes.TrimPrefix(data, []byte("encrypted:")), false, nil
}
