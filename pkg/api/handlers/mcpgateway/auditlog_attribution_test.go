package mcpgateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mcp/auditlogs"
)

func TestCollectMCPAuditEntryPersistsWhenAPIKeyAttributionFails(t *testing.T) {
	gatewayClient := newLocalAgentAuditLogTestGatewayClient(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	NewAuditLogHandler(gatewayClient).collectMCPAuditEntry(ctx, auditlogs.MCPAuditLog{
		Metadata:       map[string]string{"mcpID": "mcp-1"},
		CreatedAt:      time.Now().UTC(),
		APIKey:         auditlogs.RedactAPIKey("ok1-7-42-abcdefghijklmnopqrstuvwxyz"),
		CallType:       "tools/call",
		CallIdentifier: "search",
		RequestBody:    json.RawMessage(`{}`),
		ResponseBody:   json.RawMessage(`{}`),
		ResponseStatus: 200,
	})

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		logs, total, err := gatewayClient.GetMCPAuditLogs(t.Context(), gatewayclient.MCPAuditLogOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if total == 1 {
			if logs[0].APIKeyID != nil || logs[0].APIKeyName != "" {
				t.Fatalf("persisted event gained attribution: ID %v, name %q", logs[0].APIKeyID, logs[0].APIKeyName)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("audit event was dropped after API-key attribution failure")
}

func TestAttributeMCPAuditLogAPIKeyFallsBackOnlyWhenKeyIsMissing(t *testing.T) {
	gatewayClient := newLocalAgentAuditLogTestGatewayClient(t)
	input := auditLogInput{MCPFields: &gatewaytypes.MCPAuditLogFields{
		APIKey: auditlogs.RedactAPIKey("ok1-7-42-abcdefghijklmnopqrstuvwxyz"),
	}}

	if err := NewAuditLogHandler(gatewayClient).attributeMCPAuditLogAPIKey(t.Context(), &input); err != nil {
		t.Fatal(err)
	}

	if input.APIKeyID == nil || *input.APIKeyID != 42 || input.APIKeyName != "ok1-7-42-*****" {
		t.Fatalf("missing-key attribution = ID %v, name %q", input.APIKeyID, input.APIKeyName)
	}
}

func TestAttributeMCPAuditLogAPIKeyReturnsTransientLookupError(t *testing.T) {
	gatewayClient := newLocalAgentAuditLogTestGatewayClient(t)
	input := auditLogInput{MCPFields: &gatewaytypes.MCPAuditLogFields{
		APIKey: auditlogs.RedactAPIKey("ok1-7-42-abcdefghijklmnopqrstuvwxyz"),
	}}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := NewAuditLogHandler(gatewayClient).attributeMCPAuditLogAPIKey(ctx, &input)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("lookup error = %v, want context canceled", err)
	}
	if input.APIKeyID != nil || input.APIKeyName != "" {
		t.Fatalf("transient lookup error produced attribution: ID %v, name %q", input.APIKeyID, input.APIKeyName)
	}
}

func TestAttributeMCPAuditLogAPIKeyLooksUpTheOwnerThePrefixNames(t *testing.T) {
	gatewayClient := newLocalAgentAuditLogTestGatewayClient(t)
	ctx := t.Context()
	agent, err := gatewayClient.CreateAgent(ctx, gatewaytypes.Agent{DisplayName: "nightly build"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := gatewayClient.CreateAgentAPIKey(ctx, agent.ID, gatewaytypes.AgentAPIKeyCreateRequest{Name: "nightly", MCPServerIDs: []string{"*"}})
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, secret, err := gatewayclient.ParseAPIKey(created.Key)
	if err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name     string
		key      string
		wantID   uint
		wantName string
	}{
		{
			name:     "agent key",
			key:      created.Key,
			wantID:   created.ID,
			wantName: "nightly",
		},
		{
			// The user with the agent's ID has no such key, so the agent key's name is not used.
			name:     "agent key with a user key's prefix",
			key:      fmt.Sprintf("ok1-%d-%d-%s", agent.ID, created.ID, secret),
			wantID:   created.ID,
			wantName: fmt.Sprintf("ok1-%d-%d-*****", agent.ID, created.ID),
		},
		{
			name:     "missing agent key",
			key:      fmt.Sprintf("oak1-%d-%d-%s", agent.ID, created.ID+100, secret),
			wantID:   created.ID + 100,
			wantName: fmt.Sprintf("oak1-%d-%d-*****", agent.ID, created.ID+100),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			input := auditLogInput{MCPFields: &gatewaytypes.MCPAuditLogFields{
				APIKey: auditlogs.RedactAPIKey(tt.key),
			}}
			if err := NewAuditLogHandler(gatewayClient).attributeMCPAuditLogAPIKey(ctx, &input); err != nil {
				t.Fatal(err)
			}
			if input.APIKeyID == nil || *input.APIKeyID != tt.wantID || input.APIKeyName != tt.wantName {
				t.Fatalf("attribution = ID %v, name %q; want ID %d, name %q", input.APIKeyID, input.APIKeyName, tt.wantID, tt.wantName)
			}
		})
	}
}
