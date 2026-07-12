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

	log := types.MCPAuditLog{
		CreatedAt:  now,
		SourceType: types2.AuditLogSourceTypeLocalAgentToolCall,
		UserID:     "user-1",
		ClientIP:   "127.0.0.1",
		LocalAgentToolCallFields: &types.LocalAgentToolCallAuditLogFields{
			AgentProvider:  string(types2.LocalAgentProviderCodex),
			CLIVersion:     "1.2.3",
			Status:         string(types2.LocalAgentAuditLogStatusSucceeded),
			ObservedAt:     now,
			IdempotencyKey: "entry-1",
			ToolName:       "mcp__server__tool",
			IdentityStatus: string(types2.LocalAgentIdentityStatusAuthenticatedUser),
			ToolInput:      json.RawMessage(`{"arg":true}`),
			ToolOutput:     json.RawMessage(`{"ok":true}`),
			RawHookPayload: json.RawMessage(`{"native":true}`),
			TranscriptPath: "/tmp/transcript.jsonl",
		},
	}

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
	if local.ToolInput != nil || local.ToolOutput != nil || local.RawHookPayload != nil || local.TranscriptPath != "" {
		t.Fatalf("expected sensitive local-agent payload fields to be blanked, got %#v", local)
	}
}

func TestInsertLocalAgentAuditLogsCompletedSuccess(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()

	log := validLocalAgentAuditLog(time.Now().UTC(), "entry-1", string(types2.LocalAgentAuditLogStatusSucceeded))
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

	statuses := []types2.LocalAgentAuditLogStatus{
		types2.LocalAgentAuditLogStatusSucceeded,
		types2.LocalAgentAuditLogStatusFailed,
		types2.LocalAgentAuditLogStatusDenied,
		types2.LocalAgentAuditLogStatusTimeout,
	}
	for i, status := range statuses {
		log := validLocalAgentAuditLog(now.Add(time.Duration(i)*time.Second), string(status)+"-entry", string(status))
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

	first := validLocalAgentAuditLog(now, "same-entry", string(types2.LocalAgentAuditLogStatusSucceeded))
	duplicate := validLocalAgentAuditLog(now.Add(time.Second), "same-entry", string(types2.LocalAgentAuditLogStatusFailed))
	duplicate.LocalAgentToolCallFields.ToolName = "different-tool"

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
	if stored.LocalAgentToolCallFields.ToolName != "mcp__server__tool" {
		t.Fatalf("expected original row to remain unchanged, got tool %q", stored.LocalAgentToolCallFields.ToolName)
	}
}

func TestInsertLocalAgentAuditLogsRejectsMissingRequiredFields(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()

	log := validLocalAgentAuditLog(time.Now().UTC(), "", string(types2.LocalAgentAuditLogStatusSucceeded))
	if err := c.InsertLocalAgentAuditLogs(ctx, []types.MCPAuditLog{log}); err == nil {
		t.Fatal("expected missing idempotency key to be rejected")
	}
}

func TestInsertLocalAgentAuditLogsRejectsNonTerminalStatus(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()

	log := validLocalAgentAuditLog(time.Now().UTC(), "entry-1", "pre_tool")
	if err := c.InsertLocalAgentAuditLogs(ctx, []types.MCPAuditLog{log}); err == nil {
		t.Fatal("expected non-terminal status to be rejected")
	}
}

func TestLocalAgentAuditLogEncryptedFieldsDecryptWhenRequested(t *testing.T) {
	c := newTestClient(t)
	c.encryptionConfig = testEncryptionConfig()
	ctx := t.Context()
	now := time.Now().UTC()

	log := validLocalAgentAuditLog(now, "entry-1", string(types2.LocalAgentAuditLogStatusSucceeded))
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
	if local.Error == "permission denied for /Users/alice/project/secret.txt" ||
		local.Hostname == "alice-macbook" ||
		local.LocalUsername == "alice" ||
		local.ReportedUserEmail == "alice@example.com" ||
		local.CWD == "/Users/alice/project" ||
		local.GitRepoRoot == "/Users/alice/project" ||
		local.GitRemoteURLs[0] == "git@github.com:acme/private-repo.git" ||
		local.GitBranch == "alice/customer-fix" ||
		local.TranscriptPath == "/tmp/transcript.jsonl" ||
		bytes.Equal(local.ToolInput, []byte(`{"arg":true}`)) ||
		bytes.Equal(local.ToolOutput, []byte(`{"ok":true}`)) ||
		bytes.Equal(local.RawHookPayload, []byte(`{"native":true}`)) {
		t.Fatalf("expected sensitive local-agent fields to be encrypted at rest: %#v", local)
	}

	got, err := c.GetMCPAuditLog(ctx, stored.ID, true)
	if err != nil {
		t.Fatalf("get decrypted local-agent audit log: %v", err)
	}
	gotLocal := got.LocalAgentToolCallFields
	if gotLocal.Error != "permission denied for /Users/alice/project/secret.txt" ||
		gotLocal.DeviceID != "device-1" ||
		gotLocal.Hostname != "alice-macbook" ||
		gotLocal.LocalUsername != "alice" ||
		gotLocal.ReportedUserEmail != "alice@example.com" ||
		gotLocal.CWD != "/Users/alice/project" ||
		gotLocal.GitRepoRoot != "/Users/alice/project" ||
		gotLocal.GitRemoteURLs[0] != "git@github.com:acme/private-repo.git" ||
		gotLocal.GitBranch != "alice/customer-fix" ||
		gotLocal.TranscriptPath != "/tmp/transcript.jsonl" ||
		string(gotLocal.ToolInput) != `{"arg":true}` ||
		string(gotLocal.ToolOutput) != `{"ok":true}` ||
		string(gotLocal.RawHookPayload) != `{"native":true}` {
		t.Fatalf("expected local-agent sensitive fields to decrypt, got %#v", gotLocal)
	}

	blanked, err := c.GetMCPAuditLog(ctx, stored.ID, false)
	if err != nil {
		t.Fatalf("get blanked local-agent audit log: %v", err)
	}
	blankedLocal := blanked.LocalAgentToolCallFields
	if blankedLocal.Error != "" ||
		blankedLocal.DeviceID != "" ||
		blankedLocal.Hostname != "" ||
		blankedLocal.LocalUsername != "" ||
		blankedLocal.ReportedUserEmail != "" ||
		blankedLocal.CWD != "" ||
		blankedLocal.GitRepoRoot != "" ||
		blankedLocal.GitRemoteURLs != nil ||
		blankedLocal.GitBranch != "" ||
		blankedLocal.TranscriptPath != "" ||
		blankedLocal.ToolInput != nil ||
		blankedLocal.ToolOutput != nil ||
		blankedLocal.RawHookPayload != nil {
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

func validLocalAgentAuditLog(observedAt time.Time, idempotencyKey, status string) types.MCPAuditLog {
	return types.MCPAuditLog{
		CreatedAt:  observedAt,
		SourceType: types2.AuditLogSourceTypeLocalAgentToolCall,
		UserID:     "user-1",
		ClientIP:   "127.0.0.1",
		LocalAgentToolCallFields: &types.LocalAgentToolCallAuditLogFields{
			AgentProvider:          string(types2.LocalAgentProviderCodex),
			CLIVersion:             "1.2.3",
			Status:                 status,
			Error:                  "permission denied for /Users/alice/project/secret.txt",
			ObservedAt:             observedAt,
			IdempotencyKey:         idempotencyKey,
			ToolName:               "mcp__server__tool",
			ToolKind:               "mcp",
			ObotAuditCorrelationID: "correlation-1",
			DeviceID:               "device-1",
			Hostname:               "alice-macbook",
			LocalUsername:          "alice",
			ReportedUserEmail:      "alice@example.com",
			IdentityStatus:         string(types2.LocalAgentIdentityStatusAuthenticatedUser),
			CWD:                    "/Users/alice/project",
			GitRepoRoot:            "/Users/alice/project",
			GitRemoteURLs:          datatypes.JSONSlice[string]{"git@github.com:acme/private-repo.git"},
			GitBranch:              "alice/customer-fix",
			ToolInput:              json.RawMessage(`{"arg":true}`),
			ToolOutput:             json.RawMessage(`{"ok":true}`),
			RawHookPayload:         json.RawMessage(`{"native":true}`),
			TranscriptPath:         "/tmp/transcript.jsonl",
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
