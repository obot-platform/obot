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
			AgentProvider:  types2.LocalAgentProviderCodex,
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
			AgentProvider:  types2.LocalAgentProviderCodex,
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
	occurredAt := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	valid := LocalAgentToolCallAuditLogFields{
		OccurredAt:     occurredAt,
		ActorType:      types2.AuditLogActorTypeUser,
		ActorID:        "user-1",
		ActionName:     "mcp__server__tool",
		TargetType:     types2.AuditLogTargetTypeMCPTool,
		TargetName:     "tool",
		OutcomeStatus:  types2.AuditLogOutcomeStatusSuccess,
		AgentProvider:  types2.LocalAgentProviderCodex,
		CLIVersion:     "1.2.3",
		IdempotencyKey: "entry-1",
		RequestBody:    json.RawMessage(`{"arg":true}`),
		ResponseBody:   json.RawMessage(`{"ok":true}`),
		RawEvent:       json.RawMessage(`{"native":true}`),
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
	invalid.RequestBody = nil
	log.LocalAgentToolCallFields = &invalid
	if err := log.ValidateSourceFields(); err == nil {
		t.Fatal("expected missing tool input validation error")
	}

	invalid = valid
	invalid.ResponseBody = nil
	log.LocalAgentToolCallFields = &invalid
	if err := log.ValidateSourceFields(); err == nil {
		t.Fatal("expected missing tool output validation error")
	}

	// An explicit JSON null counts as present, not missing. No-output terminal
	// paths (failure/denial/timeout) submit an explicit null rather than
	// omitting the field, so these must validate successfully.
	for _, tt := range []struct {
		name   string
		update func(*LocalAgentToolCallAuditLogFields)
	}{
		{
			name: "null tool input",
			update: func(local *LocalAgentToolCallAuditLogFields) {
				local.RequestBody = json.RawMessage(`null`)
			},
		},
		{
			name: "null tool output",
			update: func(local *LocalAgentToolCallAuditLogFields) {
				local.ResponseBody = json.RawMessage(`null`)
			},
		},
		{
			name: "null raw hook payload",
			update: func(local *LocalAgentToolCallAuditLogFields) {
				local.RawEvent = json.RawMessage(`null`)
			},
		},
		{
			name: "whitespace padded null payload",
			update: func(local *LocalAgentToolCallAuditLogFields) {
				local.RequestBody = json.RawMessage(`  null  `)
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			accepted := valid
			tt.update(&accepted)
			log.LocalAgentToolCallFields = &accepted
			if err := log.ValidateSourceFields(); err != nil {
				t.Fatalf("expected explicit null payload to be accepted as present, got error: %v", err)
			}
		})
	}

	invalid = valid
	invalid.ActorType = ""
	log.LocalAgentToolCallFields = &invalid
	if err := log.ValidateSourceFields(); err == nil {
		t.Fatal("expected missing actor type validation error")
	}

	invalid = valid
	invalid.ActorType = "robot"
	log.LocalAgentToolCallFields = &invalid
	if err := log.ValidateSourceFields(); err == nil {
		t.Fatal("expected invalid actor type validation error")
	}

	invalid = valid
	invalid.TargetType = types2.AuditLogTargetTypeLocalTool
	invalid.TargetParentType = types2.AuditLogTargetTypeMCPServer
	invalid.TargetParentName = "server"
	log.LocalAgentToolCallFields = &invalid
	if err := log.ValidateSourceFields(); err == nil {
		t.Fatal("expected local tool parent validation error")
	}

	invalid = valid
	invalid.TargetParentType = types2.AuditLogTargetTypeMCPServer
	invalid.TargetParentName = ""
	log.LocalAgentToolCallFields = &invalid
	if err := log.ValidateSourceFields(); err == nil {
		t.Fatal("expected incomplete MCP parent validation error")
	}

	invalid = valid
	invalid.OccurredAt = time.Now().Add(2 * time.Hour)
	log.LocalAgentToolCallFields = &invalid
	if err := log.ValidateSourceFields(); err == nil {
		t.Fatal("expected far-future occurredAt validation error")
	}

	// A timestamp slightly in the future (within the allowed clock skew) must still validate.
	accepted := valid
	accepted.OccurredAt = time.Now().Add(time.Minute)
	log.LocalAgentToolCallFields = &accepted
	if err := log.ValidateSourceFields(); err != nil {
		t.Fatalf("expected near-future occurredAt within skew to be accepted, got error: %v", err)
	}
}

func TestNewLocalAgentToolCallAuditLogFromInput(t *testing.T) {
	createdAt := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	occurredAt := createdAt.Add(-time.Second)
	startedAt := occurredAt.Add(-2 * time.Second)

	log := NewLocalAgentToolCallAuditLogFromInput(types2.LocalAgentToolCallAuditLogInput{
		OccurredAt: *types2.NewTime(occurredAt),
		Action:     types2.LocalAgentToolCallAuditLogAction{Name: "mcp__server__tool", Kind: "mcp"},
		Target: types2.LocalAgentToolCallAuditLogTarget{
			TargetType: types2.AuditLogTargetTypeMCPTool,
			Name:       "tool",
			Parent: &types2.LocalAgentToolCallAuditLogTargetRef{
				TargetType: types2.AuditLogTargetTypeMCPServer,
				Name:       "server",
			},
		},
		Outcome: types2.LocalAgentToolCallAuditLogOutcome{
			Status: types2.AuditLogOutcomeStatusSuccess, Reason: "none",
			DurationMs: 2000, Error: "error with /Users/alice/project",
		},
		Details: types2.LocalAgentToolCallAuditLogReportedDetails{
			StartedAt: types2.NewTime(startedAt),
			Trace: types2.LocalAgentToolCallAuditLogTrace{
				IdempotencyKey: "entry-1", ToolUseID: "tool-use-1", SessionID: "session-1", TurnID: "turn-1",
			},
			Agent: types2.LocalAgentToolCallAuditLogAgent{
				Provider: types2.LocalAgentProviderCodex, Version: "0.1.0", CLIName: "obot", CLIVersion: "1.2.3",
				Model: "gpt-5", ModelID: "model-1", PermissionMode: "default",
			},
			Device: types2.LocalAgentToolCallAuditLogDevice{
				Hostname: "alice-macbook", OS: "darwin", Architecture: "arm64", LocalUsername: "alice",
			},
			Environment: types2.LocalAgentToolCallAuditLogEnvironment{
				CWD: "/Users/alice/project", GitRoot: "/Users/alice/project",
				GitRemotes: []string{"git@github.com:acme/private-repo.git"}, GitBranch: "alice/customer-fix",
				GitCommit: "abc123", ReportedUserEmail: "alice@example.com", TranscriptPath: "/tmp/transcript.jsonl",
			},
			Request:  types2.LocalAgentToolCallAuditLogPayload{Body: json.RawMessage(`{"arg":true}`)},
			Response: types2.LocalAgentToolCallAuditLogPayload{Body: json.RawMessage(`{"ok":true}`)},
			RawEvent: json.RawMessage(`{"native":true}`),
		},
	}, types2.AuditLogActorTypeUser, "user-1", "127.0.0.1", 0, createdAt)

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
	if local.ActorType != types2.AuditLogActorTypeUser || local.ActorID != "user-1" {
		t.Fatalf("expected server-owned actor, got type=%q id=%q", local.ActorType, local.ActorID)
	}
	if local.AgentProvider != types2.LocalAgentProviderCodex ||
		local.CLIVersion != "1.2.3" ||
		local.ActionName != "mcp__server__tool" ||
		local.CWD != "/Users/alice/project" ||
		local.GitRemotes[0] != "git@github.com:acme/private-repo.git" ||
		string(local.RequestBody) != `{"arg":true}` ||
		string(local.RawEvent) != `{"native":true}` {
		t.Fatalf("client-supplied fields were not copied correctly: %#v", local)
	}
	if local.StartedAt == nil || !local.StartedAt.Equal(startedAt) || !local.OccurredAt.Equal(occurredAt) {
		t.Fatalf("time fields were not converted correctly: %#v", local)
	}
	if err := log.ValidateSourceFields(); err != nil {
		t.Fatalf("converted input should validate: %v", err)
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
	if got, want := localError.BindNames, []string{"LocalAgentToolCallFields", "OutcomeError"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("expected local_agent_error to bind to local-agent fields, got %#v", got)
	}
}
