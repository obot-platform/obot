package auditlog

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	api "github.com/obot-platform/obot/apiclient/types"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
)

func TestClassifyMCPOutcome(t *testing.T) {
	tests := []struct {
		name   string
		status int
		err    string
		want   api.AuditLogOutcomeStatus
	}{
		{"denied takes precedence", 403, "server error", api.AuditLogOutcomeStatusDenied},
		{"timeout", 504, "", api.AuditLogOutcomeStatusTimeout},
		{"error", 200, "MCP error", api.AuditLogOutcomeStatusFailure},
		{"http failure", 500, "", api.AuditLogOutcomeStatusFailure},
		{"success", 200, "", api.AuditLogOutcomeStatusSuccess},
		{"unmatched request", 0, "", api.AuditLogOutcomeStatusUnknown},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ClassifyMCPOutcome(test.status, test.err); got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestPresentMCPNormalizesSummaryAndDetails(t *testing.T) {
	created := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	log := gatewaytypes.MCPAuditLog{
		ID: 7, CreatedAt: created, SourceType: api.AuditLogSourceTypeMCP,
		UserID: "user-1", ClientIP: "127.0.0.1",
		MCPFields: &gatewaytypes.MCPAuditLogFields{
			APIKey: "credential-1", MCPID: "mcp-1", MCPServerDisplayName: "GitHub",
			CallType: "tools/call", CallIdentifier: "search", ResponseStatus: 200,
			ResponseReceived: true, ProcessingTimeMs: 42, RequestBody: json.RawMessage(`{"q":"obot"}`),
			SessionID: "session-1", ClientName: "client", ClientVersion: "1.0.0",
		},
	}

	summary := Present(log, PresentOptions{})
	if summary.Details != nil || summary.EventType != api.AuditLogEventTypeMCPCall {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	if summary.Actor.ActorType != api.AuditLogActorTypeUser || summary.Actor.ID != "user-1" || summary.Actor.CredentialID != "credential-1" {
		t.Fatalf("unexpected actor: %#v", summary.Actor)
	}
	if summary.Target.TargetType != api.AuditLogTargetTypeMCPTool || summary.Target.Parent == nil {
		t.Fatalf("unexpected target: %#v", summary.Target)
	}
	if summary.Outcome.Status != api.AuditLogOutcomeStatusSuccess || summary.Timestamp.Source != api.AuditLogTimestampSourceServer {
		t.Fatalf("unexpected outcome/timestamp: %#v %#v", summary.Outcome, summary.Timestamp)
	}

	detail := Present(log, PresentOptions{IncludeDetails: true, PayloadRedacted: true})
	if detail.Details == nil || !detail.Details.PayloadRedacted || detail.Details.Request == nil || string(detail.Details.Request.Body) != `{"q":"obot"}` {
		t.Fatalf("unexpected details: %#v", detail.Details)
	}
	data, err := json.Marshal(detail)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "mcpFields") || strings.Contains(string(data), "localAgentToolCallFields") {
		t.Fatalf("legacy subtype key leaked into normalized JSON: %s", data)
	}
}

func TestPresentLocalAgentIdentityTargetAndTimestamp(t *testing.T) {
	recorded := time.Date(2026, 7, 14, 10, 0, 1, 0, time.UTC)
	observed := recorded.Add(-time.Second)
	log := gatewaytypes.MCPAuditLog{
		ID: 8, CreatedAt: recorded, SourceType: api.AuditLogSourceTypeLocalAgentToolCall,
		UserID: "untrusted-user-must-not-win",
		LocalAgentToolCallFields: &gatewaytypes.LocalAgentToolCallAuditLogFields{
			OccurredAt: observed, ActorType: api.AuditLogActorTypeDevice, ActorID: "device-1", DeviceID: "device-1",
			ActionName: "mcp__github__search", ActionKind: "mcp",
			TargetType: api.AuditLogTargetTypeMCPTool, TargetName: "search",
			TargetParentType: api.AuditLogTargetTypeMCPServer, TargetParentName: "github",
			OutcomeStatus: api.AuditLogOutcomeStatusDenied, OutcomeReason: "policy",
		},
	}
	event := Present(log, PresentOptions{IncludeDetails: true})
	if event.Timestamp.OccurredAt.GetTime() != observed || event.Timestamp.RecordedAt.GetTime() != recorded || event.Timestamp.Source != api.AuditLogTimestampSourceClientReported {
		t.Fatalf("unexpected timestamps: %#v", event.Timestamp)
	}
	if event.Actor.ActorType != api.AuditLogActorTypeDevice || event.Actor.ID != "device-1" {
		t.Fatalf("unexpected actor: %#v", event.Actor)
	}
	if event.Target.TargetType != api.AuditLogTargetTypeMCPTool || event.Target.Parent == nil || event.Target.Parent.Name != "github" {
		t.Fatalf("unexpected target: %#v", event.Target)
	}
	if event.Outcome.Status != api.AuditLogOutcomeStatusDenied || event.Outcome.Reason != "policy" {
		t.Fatalf("unexpected outcome: %#v", event.Outcome)
	}
}

func TestPresentUnknownActors(t *testing.T) {
	mcp := gatewaytypes.MCPAuditLog{SourceType: api.AuditLogSourceTypeMCP, MCPFields: &gatewaytypes.MCPAuditLogFields{APIKey: "credential"}}
	if actor := Present(mcp, PresentOptions{}).Actor; actor.ActorType != api.AuditLogActorTypeCredential || actor.ID != "credential" {
		t.Fatalf("unexpected credential actor: %#v", actor)
	}
	local := gatewaytypes.MCPAuditLog{SourceType: api.AuditLogSourceTypeLocalAgentToolCall, UserID: "reported", LocalAgentToolCallFields: &gatewaytypes.LocalAgentToolCallAuditLogFields{ActorType: api.AuditLogActorTypeUnknown}}
	if actor := Present(local, PresentOptions{}).Actor; actor.ActorType != api.AuditLogActorTypeUnknown || actor.ID != "" {
		t.Fatalf("unexpected unresolved actor: %#v", actor)
	}
}
