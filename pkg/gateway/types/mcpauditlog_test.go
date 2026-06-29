package types

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"gorm.io/gorm/schema"
)

func TestMCPAuditLogNormalizeMCPFields(t *testing.T) {
	log := MCPAuditLog{}

	log.NormalizeMCPFields()
	if log.SourceType != types2.AuditLogSourceTypeMCP {
		t.Fatalf("expected source type %q, got %q", types2.AuditLogSourceTypeMCP, log.SourceType)
	}
	if log.MCPFields == nil {
		t.Fatal("expected MCP fields to be populated")
	}
}

func TestMCPAuditLogMCPInitializesOnlyMCPRows(t *testing.T) {
	mcpLog := MCPAuditLog{
		SourceType: types2.AuditLogSourceTypeMCP,
	}
	if mcp := mcpLog.MCP(); mcp == nil {
		t.Fatal("expected MCP fields to be initialized for MCP log")
	}

	untypedLog := MCPAuditLog{}
	if mcp := untypedLog.MCP(); mcp != nil {
		t.Fatalf("expected no MCP fields for empty source type, got %#v", mcp)
	}

	localLog := MCPAuditLog{
		SourceType: types2.AuditLogSourceTypeLocalAgentToolCall,
		LocalAgentToolCallFields: &LocalAgentToolCallAuditLogFields{
			AgentProvider:  string(types2.LocalAgentProviderCodex),
			IdempotencyKey: "entry-1",
		},
	}
	if mcp := localLog.MCP(); mcp != nil {
		t.Fatalf("expected no MCP fields for local-agent log, got %#v", mcp)
	}
}

func TestMCPAuditLogValidationRejectsBothFieldGroups(t *testing.T) {
	log := MCPAuditLog{
		SourceType: types2.AuditLogSourceTypeMCP,
		MCPFields: &MCPAuditLogFields{
			MCPID: "mcp-1",
		},
		LocalAgentToolCallFields: &LocalAgentToolCallAuditLogFields{
			AgentProvider:  string(types2.LocalAgentProviderCodex),
			IdempotencyKey: "entry-1",
		},
	}

	if err := log.ValidateSourceFields(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestMCPAuditLogValidationRequiresSelectedFieldGroup(t *testing.T) {
	tests := []struct {
		name string
		log  MCPAuditLog
	}{
		{
			name: "mcp without mcp fields",
			log: MCPAuditLog{
				SourceType: types2.AuditLogSourceTypeMCP,
			},
		},
		{
			name: "local-agent without local fields",
			log: MCPAuditLog{
				SourceType: types2.AuditLogSourceTypeLocalAgentToolCall,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.log.ValidateSourceFields(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestMCPAuditLogValidationRequiresLocalAgentFields(t *testing.T) {
	observedAt := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	valid := LocalAgentToolCallAuditLogFields{
		AgentProvider:  string(types2.LocalAgentProviderCodex),
		CLIVersion:     "1.2.3",
		Status:         string(types2.LocalAgentAuditLogStatusSucceeded),
		ObservedAt:     observedAt,
		IdempotencyKey: "entry-1",
		ToolName:       "mcp__server__tool",
		ToolInput:      json.RawMessage(`{"arg":true}`),
		ToolOutput:     json.RawMessage(`{"ok":true}`),
		RawHookPayload: json.RawMessage(`{"native":true}`),
	}

	log := MCPAuditLog{
		SourceType:               types2.AuditLogSourceTypeLocalAgentToolCall,
		LocalAgentToolCallFields: &valid,
	}
	if err := log.ValidateSourceFields(); err != nil {
		t.Fatalf("expected valid local-agent log, got error: %v", err)
	}

	invalid := valid
	invalid.IdempotencyKey = ""
	log.LocalAgentToolCallFields = &invalid
	if err := log.ValidateSourceFields(); err == nil {
		t.Fatal("expected missing idempotency key validation error")
	}

	invalid = valid
	invalid.ToolInput = nil
	log.LocalAgentToolCallFields = &invalid
	if err := log.ValidateSourceFields(); err == nil {
		t.Fatal("expected missing tool input validation error")
	}

	invalid = valid
	invalid.ToolOutput = nil
	log.LocalAgentToolCallFields = &invalid
	if err := log.ValidateSourceFields(); err == nil {
		t.Fatal("expected missing tool output validation error")
	}
}

func TestConvertMCPAuditLogUsesNestedMCPAPIFields(t *testing.T) {
	createdAt := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	log := MCPAuditLog{
		ID:         1,
		CreatedAt:  createdAt,
		SourceType: types2.AuditLogSourceTypeMCP,
		UserID:     "user-1",
		ClientIP:   "127.0.0.1",
		MCPFields: &MCPAuditLogFields{
			APIKey:                    "key",
			MCPID:                     "mcp-1",
			PowerUserWorkspaceID:      "workspace-1",
			MCPServerDisplayName:      "Server",
			MCPServerCatalogEntryName: "catalog",
			ClientName:                "client",
			ClientVersion:             "1.0.0",
			CallType:                  "tools/call",
			CallIdentifier:            "tool",
			RequestBody:               json.RawMessage(`{"input":true}`),
			ResponseBody:              json.RawMessage(`{"ok":true}`),
			ResponseStatus:            200,
			ProcessingTimeMs:          42,
			SessionID:                 "session-1",
			ObotAuditCorrelationID:    "correlation-1",
			ResponseReceived:          true,
			RequestID:                 "request-1",
		},
		LocalAgentToolCallFields: &LocalAgentToolCallAuditLogFields{},
	}

	apiLog := ConvertMCPAuditLog(log)
	if apiLog.SourceType != "mcp" {
		t.Fatalf("expected source type mcp, got %q", apiLog.SourceType)
	}
	if apiLog.UserID != "user-1" || apiLog.ClientIP != "127.0.0.1" {
		t.Fatalf("common API fields were not preserved: %#v", apiLog)
	}
	if apiLog.MCPFields == nil {
		t.Fatal("expected nested MCP fields")
	}
	if apiLog.LocalAgentToolCallFields != nil {
		t.Fatalf("expected no local-agent fields for MCP log: %#v", apiLog.LocalAgentToolCallFields)
	}
	if apiLog.MCPFields.MCPID != "mcp-1" || apiLog.MCPFields.CallIdentifier != "tool" || apiLog.MCPFields.ClientInfo.Name != "client" {
		t.Fatalf("nested MCP API fields were not populated: %#v", apiLog.MCPFields)
	}
	if apiLog.MCPFields.ObotAuditCorrelationID != "correlation-1" {
		t.Fatalf("expected MCP correlation ID to be preserved, got %q", apiLog.MCPFields.ObotAuditCorrelationID)
	}
}

func TestConvertMCPAuditLogLocalAgentToolCallFields(t *testing.T) {
	createdAt := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	startedAt := createdAt.Add(-2 * time.Second)
	log := MCPAuditLog{
		ID:         2,
		CreatedAt:  createdAt,
		SourceType: types2.AuditLogSourceTypeLocalAgentToolCall,
		UserID:     "user-1",
		ClientIP:   "127.0.0.1",
		MCPFields:  &MCPAuditLogFields{},
		LocalAgentToolCallFields: &LocalAgentToolCallAuditLogFields{
			AgentProvider:          string(types2.LocalAgentProviderCodex),
			CLIVersion:             "1.2.3",
			Status:                 string(types2.LocalAgentAuditLogStatusSucceeded),
			ObservedAt:             createdAt,
			StartedAt:              &startedAt,
			DurationMs:             2000,
			IdempotencyKey:         "entry-1",
			ToolUseID:              "tool-use-1",
			SessionID:              "session-1",
			TurnID:                 "turn-1",
			ToolName:               "mcp__server__tool",
			ToolKind:               "mcp",
			MCPServerHint:          "server",
			MCPToolName:            "tool",
			ObotAuditCorrelationID: "correlation-1",
			ToolInput:              json.RawMessage(`{"arg":true}`),
			ToolOutput:             json.RawMessage(`{"ok":true}`),
			RawHookPayload:         json.RawMessage(`{"native":true}`),
		},
	}

	apiLog := ConvertMCPAuditLog(log)
	if apiLog.MCPFields != nil {
		t.Fatalf("expected no MCP fields for local-agent log: %#v", apiLog.MCPFields)
	}
	local := apiLog.LocalAgentToolCallFields
	if local == nil {
		t.Fatal("expected local-agent fields")
	}
	if local.IdempotencyKey != "entry-1" || local.ToolName != "mcp__server__tool" || local.ObotAuditCorrelationID != "correlation-1" {
		t.Fatalf("local-agent fields were not populated: %#v", local)
	}
	if local.StartedAt == nil || local.StartedAt.Time.UTC() != startedAt {
		t.Fatalf("startedAt was not converted: %#v", local.StartedAt)
	}
}

func TestNewLocalAgentToolCallAuditLogFromManifest(t *testing.T) {
	createdAt := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	observedAt := createdAt.Add(-time.Second)
	startedAt := observedAt.Add(-2 * time.Second)

	log := NewLocalAgentToolCallAuditLogFromManifest(types2.LocalAgentToolCallAuditLogManifest{
		AgentProvider:          types2.LocalAgentProviderCodex,
		AgentVersion:           "0.1.0",
		CLIName:                "obot",
		CLIVersion:             "1.2.3",
		Status:                 types2.LocalAgentAuditLogStatusSucceeded,
		FailureType:            "none",
		ObservedAt:             *types2.NewTime(observedAt),
		StartedAt:              types2.NewTime(startedAt),
		DurationMs:             2000,
		Error:                  "error with /Users/alice/project",
		IdempotencyKey:         "entry-1",
		ToolUseID:              "tool-use-1",
		SessionID:              "session-1",
		TurnID:                 "turn-1",
		ToolName:               "mcp__server__tool",
		ToolKind:               "mcp",
		MCPServerHint:          "server",
		MCPToolName:            "tool",
		ObotAuditCorrelationID: "correlation-1",
		Model:                  "gpt-5",
		ModelID:                "model-1",
		PermissionMode:         "default",
		DeviceID:               "device-1",
		Hostname:               "alice-macbook",
		OS:                     "darwin",
		Arch:                   "arm64",
		LocalUsername:          "alice",
		ReportedUserEmail:      "alice@example.com",
		CWD:                    "/Users/alice/project",
		GitRepoRoot:            "/Users/alice/project",
		GitRemoteURLs:          []string{"git@github.com:acme/private-repo.git"},
		GitBranch:              "alice/customer-fix",
		GitCommitSHA:           "abc123",
		TranscriptPath:         "/tmp/transcript.jsonl",
		ToolInput:              json.RawMessage(`{"arg":true}`),
		ToolOutput:             json.RawMessage(`{"ok":true}`),
		RawHookPayload:         json.RawMessage(`{"native":true}`),
	}, "user-1", "127.0.0.1", types2.LocalAgentIdentityStatusAuthenticatedUser, createdAt)

	if log.SourceType != types2.AuditLogSourceTypeLocalAgentToolCall {
		t.Fatalf("expected local-agent source type, got %q", log.SourceType)
	}
	if log.UserID != "user-1" || log.ClientIP != "127.0.0.1" || !log.CreatedAt.Equal(createdAt) {
		t.Fatalf("server-owned fields were not populated correctly: %#v", log)
	}
	local := log.LocalAgentToolCallFields
	if local == nil {
		t.Fatal("expected local-agent fields")
	}
	if local.IdentityStatus != string(types2.LocalAgentIdentityStatusAuthenticatedUser) {
		t.Fatalf("expected server-owned identity status, got %q", local.IdentityStatus)
	}
	if local.AgentProvider != string(types2.LocalAgentProviderCodex) ||
		local.CLIVersion != "1.2.3" ||
		local.ToolName != "mcp__server__tool" ||
		local.CWD != "/Users/alice/project" ||
		local.GitRemoteURLs[0] != "git@github.com:acme/private-repo.git" ||
		string(local.ToolInput) != `{"arg":true}` ||
		string(local.RawHookPayload) != `{"native":true}` {
		t.Fatalf("client-supplied fields were not copied correctly: %#v", local)
	}
	if local.StartedAt == nil || !local.StartedAt.Equal(startedAt) || !local.ObservedAt.Equal(observedAt) {
		t.Fatalf("time fields were not converted correctly: %#v", local)
	}
	if err := log.ValidateSourceFields(); err != nil {
		t.Fatalf("converted input should validate: %v", err)
	}
}

func TestObotAuditCorrelationIDUsesSharedDatabaseColumn(t *testing.T) {
	parsed, err := schema.Parse(&MCPAuditLog{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatal(err)
	}

	var fieldPaths [][]string
	for _, field := range parsed.Fields {
		if field.DBName == "obot_audit_correlation_id" {
			fieldPaths = append(fieldPaths, field.BindNames)
		}
	}
	if len(fieldPaths) != 2 {
		t.Fatalf("expected MCP and local-agent correlation fields to share one DB column name, got %#v", fieldPaths)
	}

	var dbNameCount int
	for _, dbName := range parsed.DBNames {
		if dbName == "obot_audit_correlation_id" {
			dbNameCount++
		}
	}
	if dbNameCount != 1 {
		t.Fatalf("expected one database column for correlation ID, got %d in %#v", dbNameCount, parsed.DBNames)
	}
}

func TestResponseReceivedUsesMCPDatabaseColumn(t *testing.T) {
	parsed, err := schema.Parse(&MCPAuditLog{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatal(err)
	}

	field := parsed.LookUpField("response_received")
	if field == nil {
		t.Fatal("expected response_received database column")
	}
	if got, want := field.BindNames, []string{"MCPFields", "ResponseReceived"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("expected response_received to bind to MCP fields, got %#v", got)
	}
}

func TestLocalAgentErrorUsesSeparateDatabaseColumn(t *testing.T) {
	parsed, err := schema.Parse(&MCPAuditLog{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatal(err)
	}

	mcpError := parsed.LookUpField("error")
	if mcpError == nil {
		t.Fatal("expected MCP error database column")
	}
	if got, want := mcpError.BindNames, []string{"MCPFields", "Error"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("expected error to bind to MCP fields, got %#v", got)
	}

	localError := parsed.LookUpField("local_agent_error")
	if localError == nil {
		t.Fatal("expected local_agent_error database column")
	}
	if got, want := localError.BindNames, []string{"LocalAgentToolCallFields", "Error"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("expected local_agent_error to bind to local-agent fields, got %#v", got)
	}
}
